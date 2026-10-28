package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/mcpaccess"
	"github.com/greg0x46/julius/internal/settings"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func management(t *testing.T, store *auth.Store) func(string, string, string, bool) *httptest.ResponseRecorder {
	t.Helper()
	config := testConfig
	config.MCPEnabled = true
	api := httpapi.NewServer(store.DB, nil, nil, config, nil)
	token, err := store.Login(context.Background(), "owner@example.com", "synthetic-password")
	if err != nil {
		t.Fatal(err)
	}
	return func(method, path, body string, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.AddCookie(&http.Cookie{Name: "__Host-julius_session", Value: token})
		if csrf {
			r.Header.Set("Origin", testConfig.PublicURL)
			r.Header.Set("X-Julius-Request", "1")
		}
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		return w
	}
}
func authorizationID(t *testing.T, conn *sql.DB, token string) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(`SELECT authorization_id FROM oauth_grants WHERE token_hash=?`, tokenHash(token)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
func TestManagedAuthorizations(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, firstToken := setup(t, conn)
		secondToken := oauthToken(t, store)
		first := authorizationID(t, conn, firstToken)
		second := authorizationID(t, conn, secondToken)
		if first == second {
			t.Fatal("shared connection identity")
		}
		api := management(t, store)
		w := api("GET", "/api/settings/mcp", "", false)
		var list struct {
			Available, Enabled bool
			Connections        []mcpaccess.Authorization
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list.Connections) != 2 || !list.Available || !list.Enabled {
			t.Fatalf("list: %d %s", w.Code, w.Body)
		}
		for _, secret := range []string{firstToken, secondToken, tokenHash(firstToken), "token_hash", "csrf_hash"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("credential leaked")
			}
		}
		if w := api("DELETE", "/api/settings/mcp/connections/"+first, "", false); w.Code != 403 {
			t.Fatalf("CSRF: %d", w.Code)
		}
		for _, tc := range []struct {
			method, path, body string
			status             int
		}{
			{"PATCH", "/api/settings/mcp/connections/" + first, `{"scope":"unknown"}`, 422},
			{"PATCH", "/api/settings/mcp/connections/" + first, `{"scope":"mcp:read mcp:write"}`, 409},
			{"PATCH", "/api/settings/mcp/connections/invalid", `{"scope":"mcp:read"}`, 422},
			{"PUT", "/api/settings/mcp", `{}`, 422},
			{"PUT", "/api/settings/mcp", `{"enabled":false}`, 200},
		} {
			w := api(tc.method, tc.path, tc.body, true)
			if w.Code != tc.status {
				t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body)
			}
		}
		handler, _ := NewHandler(true, testConfig, store)
		for _, token := range []string{firstToken, secondToken} {
			if w := request(t, handler, token, "", `{}`); w.Code != 403 {
				t.Fatalf("disabled: %d", w.Code)
			}
		}
		if w := oauthRequest(handler, "POST", "/oauth/token", "grant_type=authorization_code", nil, ""); w.Code != 403 {
			t.Fatalf("disabled OAuth: %d", w.Code)
		}
		// Revocation remains available while MCP is switched off.
		if w := api("DELETE", "/api/settings/mcp/connections/"+first, "", true); w.Code != 200 {
			t.Fatalf("revoke: %d %s", w.Code, w.Body)
		}
		if w := api("PUT", "/api/settings/mcp", `{"enabled":true}`, true); w.Code != 200 {
			t.Fatal(w.Body)
		}
		if w := request(t, handler, firstToken, "", `{}`); w.Code != 401 {
			t.Fatalf("revoked: %d", w.Code)
		}
		if w := request(t, handler, secondToken, "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`); w.Code != 200 {
			t.Fatalf("other client: %d %s", w.Code, w.Body)
		}
		if err := settings.SetAuthenticationEnabled(context.Background(), conn, false); err != nil {
			t.Fatal(err)
		}
		for _, method := range []string{"GET", "PUT", "DELETE"} {
			path := "/api/settings/mcp"
			if method == "DELETE" {
				path += "/connections/" + second
			}
			if w := api(method, path, `{"enabled":true}`, true); w.Code != 403 {
				t.Fatalf("open management %s: %d", method, w.Code)
			}
		}
		if err := settings.SetAuthenticationEnabled(context.Background(), conn, true); err != nil {
			t.Fatal(err)
		}
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}
		if w := api("GET", "/api/settings/mcp", "", false); w.Code != 503 || strings.Contains(w.Body.String(), "sql") {
			t.Fatalf("failure: %d %s", w.Code, w.Body)
		}
	})
}
func TestToolPermissionsAtExecution(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, readToken := setup(t, conn)
		flow := newFlow(t, store)
		flow.params.Set("scope", mcpaccess.ReadWrite)
		writeToken := flow.token(t, flow.approve(t))
		id := authorizationID(t, conn, writeToken)
		oauth := &oauthServer{store: store, config: testConfig}
		server := sdk.NewServer(&sdk.Implementation{Name: "synthetic-tools", Version: "1"}, nil)
		server.AddReceivingMiddleware(oauth.permissions(map[string]string{"read": mcpaccess.Read, "write": mcpaccess.ReadWrite}))
		calls := 0
		for _, name := range []string{"read", "write", "unclassified"} {
			server.AddTool(&sdk.Tool{Name: name, InputSchema: map[string]any{"type": "object"}}, func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
				calls++
				return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "synthetic"}}}, nil
			})
		}
		handler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, &sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
		call := func(token, name string, want bool) {
			t.Helper()
			before := calls
			w := request(t, handler, token, "", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"`+name+`","arguments":{}}}`)
			if (calls == before+1) != want {
				t.Fatalf("%s permitted=%t calls=%d %s", name, want, calls, w.Body)
			}
		}
		call(readToken, "read", true)
		call(readToken, "write", false)
		call(writeToken, "write", true)
		call(writeToken, "unclassified", false)
		access := mcpaccess.NewStore(conn)
		if err := access.SetScope(context.Background(), id, mcpaccess.Read); err != nil {
			t.Fatal(err)
		}
		call(writeToken, "write", false)
		call(writeToken, "read", true)
		if err := access.SetScope(context.Background(), id, mcpaccess.ReadWrite); err != nil {
			t.Fatal(err)
		}
		call(writeToken, "write", true)
		if err := access.Revoke(context.Background(), id); err != nil {
			t.Fatal(err)
		}
		call(writeToken, "read", false)
		call(readToken, "read", true)
		if err := mcpaccess.SetEnabled(context.Background(), conn, false); err != nil {
			t.Fatal(err)
		}
		call(readToken, "read", false)
	})
}

func TestRevocationBeforeCodeExchange(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, _ := setup(t, conn)
		flow := newFlow(t, store)
		code := flow.approve(t)
		id := authorizationID(t, conn, code)
		if err := mcpaccess.NewStore(conn).Revoke(context.Background(), id); err != nil {
			t.Fatal(err)
		}
		w := oauthRequest(flow.handler, "POST", "/oauth/token", flow.exchange(code).Encode(), nil, "")
		if w.Code != 400 {
			t.Fatalf("revoked code: %d %s", w.Code, w.Body)
		}
	})
}

func TestConsentLimitsRequestedScopes(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, _ := setup(t, conn)
		for _, requested := range []string{"mcp:access", mcpaccess.Read, mcpaccess.ReadWrite} {
			f := newFlow(t, store)
			f.params.Set("scope", requested)
			page := f.begin(t)
			if strings.Contains(page.Body.String(), `value="mcp:read mcp:write"`) != (requested == mcpaccess.ReadWrite) {
				t.Fatal("incorrect consent options")
			}
			p := url.Values{"request": {f.pending}, "decision": {"allow"}, "email": {"owner@example.com"}, "password": {"synthetic-password"}, "scope": {mcpaccess.ReadWrite}}
			if requested != mcpaccess.ReadWrite {
				w := oauthRequest(f.handler, "POST", "/oauth/authorize", p.Encode(), f.cookie, testConfig.PublicURL)
				if w.Code != 400 {
					t.Fatalf("consent escalation: %d", w.Code)
				}
			}
			p.Del("scope")
			w := oauthRequest(f.handler, "POST", "/oauth/authorize", p.Encode(), f.cookie, testConfig.PublicURL)
			u, _ := url.Parse(w.Header().Get("Location"))
			if w.Code != 303 || u.Query().Get("code") == "" {
				t.Fatalf("consent: %d %s", w.Code, w.Body)
			}
			w = oauthRequest(f.handler, "POST", "/oauth/token", f.exchange(u.Query().Get("code")).Encode(), nil, "")
			var result struct {
				Scope string `json:"scope"`
			}
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Scope != mcpaccess.Read {
				t.Fatalf("default read: %d %s", w.Code, w.Body)
			}
		}
	})
}

func TestDemoCannotManageMCP(t *testing.T) {
	store, _ := setup(t, dbtest.OpenSQLite(t))
	config := testConfig
	config.Demo = true
	config.MCPEnabled = true
	api := httpapi.NewServer(store.DB, nil, nil, config, nil)
	for _, method := range []string{"GET", "PUT", "PATCH", "DELETE"} {
		path := "/api/settings/mcp"
		if method == "PATCH" || method == "DELETE" {
			path += "/connections/00000000-0000-0000-0000-000000000001"
		}
		r := httptest.NewRequest(method, path, strings.NewReader(`{"enabled":true}`))
		r.Header.Set("Origin", config.PublicURL)
		r.Header.Set("X-Julius-Request", "1")
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("demo %s: %d", method, w.Code)
		}
	}
}
