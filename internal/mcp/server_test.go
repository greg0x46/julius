package mcp

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/settings"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

var testConfig = auth.Config{PublicURL: "https://julius.example"}

func setup(t *testing.T, conn *sql.DB) (*auth.Store, string) {
	t.Helper()
	store := auth.NewStore(conn)
	ctx := context.Background()
	if err := store.Initialize(ctx, "owner@example.com", "synthetic-password", bytes.Repeat([]byte{42}, 32), nil); err != nil {
		t.Fatal(err)
	}
	return store, oauthToken(t, store)
}

func request(t *testing.T, handler http.Handler, token, origin, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestConfiguration(t *testing.T) {
	for _, value := range []string{"", "false", "0", " true "} {
		got, err := ParseEnabled(value)
		if err != nil || got != (strings.TrimSpace(value) == "true") {
			t.Fatalf("ParseEnabled(%q) = %v, %v", value, got, err)
		}
	}
	if _, err := ParseEnabled("invalid"); err == nil {
		t.Fatal("accepted invalid configuration")
	}
	for _, demo := range []bool{false, true} {
		handler, err := NewHandler(false, auth.Config{Demo: demo}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := request(t, handler, "", "", `{}`).Code; got != 404 {
			t.Fatalf("disabled: %d", got)
		}
	}
	if _, err := NewHandler(true, auth.Config{PublicURL: testConfig.PublicURL, Demo: true}, nil); err == nil {
		t.Fatal("enabled in demo")
	}
	if _, err := NewHandler(true, auth.Config{}, nil); err == nil {
		t.Fatal("accepted invalid origin config")
	}
}

func TestAccessBoundary(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, token := setup(t, conn)
		handler, err := NewHandler(true, testConfig, store)
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			token, origin string
			status        int
		}{
			{"", "", 401}, {"invalid", "", 401}, {token, "https://evil.example", 403},
		} {
			w := request(t, handler, tc.token, tc.origin, `{}`)
			if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("access: %d %s", w.Code, w.Body)
			}
			if w.Code == 401 && w.Header().Get("WWW-Authenticate") == "" {
				t.Fatal("missing bearer challenge")
			}
		}
		r := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{}`))
		r.AddCookie(&http.Cookie{Name: "__Host-julius_session", Value: token})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatalf("cookie authorized: %d", w.Code)
		}
		if err := settings.SetAuthenticationEnabled(context.Background(), conn, false); err != nil {
			t.Fatal(err)
		}
		if w := request(t, handler, token, "", `{}`); w.Code != 403 {
			t.Fatalf("open mode: %d", w.Code)
		}
		if err := settings.SetAuthenticationEnabled(context.Background(), conn, true); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(`DELETE FROM oauth_grants WHERE token_hash=?`, tokenHash(token)); err != nil {
			t.Fatal(err)
		}
		if w := request(t, handler, token, "", `{}`); w.Code != 401 {
			t.Fatalf("revoked: %d", w.Code)
		}
		token = oauthToken(t, store)
		store.Now = func() time.Time { return time.Now().Add(auth.Lifetime + time.Second) }
		if w := request(t, handler, token, "", `{}`); w.Code != 401 {
			t.Fatalf("expired: %d", w.Code)
		}
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}
		w = request(t, handler, token, "", `{}`)
		if w.Code != 503 || strings.Contains(w.Body.String(), "sql") {
			t.Fatalf("database error: %d %s", w.Code, w.Body)
		}
	})
}

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func TestProtocolLifecycle(t *testing.T) {
	store, token := setup(t, dbtest.OpenSQLite(t))
	handler, err := NewHandler(true, testConfig, store)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := sdk.NewClient(&sdk.Implementation{Name: "generic-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: &http.Client{Transport: bearerTransport{token}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.ListTools(ctx, &sdk.ListToolsParams{})
	if err != nil || len(result.Tools) != 31 {
		t.Fatalf("tool discovery: %v %v", result, err)
	}
	readOnly := 0
	for _, tool := range result.Tools {
		if tool.Annotations != nil && tool.Annotations.ReadOnlyHint {
			readOnly++
		}
		if tool.OutputSchema == nil || tool.Annotations == nil {
			t.Fatalf("untyped tool: %s", tool.Name)
		}
	}
	if readOnly != 9 {
		t.Fatalf("read-only tool annotations: got %d, want 9", readOnly)
	}
	read, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "list_goals", Arguments: map[string]any{}})
	if err != nil || read.IsError || read.StructuredContent == nil {
		t.Fatalf("financial read: %+v %v", read, err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	// Exercise the initialize handshake used by pre-2026 clients too.
	w := request(t, handler, token, testConfig.PublicURL, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","clientInfo":{"name":"generic","version":"1"},"capabilities":{}}}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"protocolVersion":"2025-11-25"`) || w.Header().Get("Mcp-Session-Id") != "" {
		t.Fatalf("initialize: %d %s", w.Code, w.Body)
	}
	if w := request(t, handler, token, "", `{`); w.Code != 400 {
		t.Fatalf("malformed JSON: %d %s", w.Code, w.Body)
	}
	if w := request(t, handler, token, "", `{"jsonrpc":"2.0","id":2,"method":"unknown"}`); w.Code != 400 {
		t.Fatalf("unsupported method: %d %s", w.Code, w.Body)
	}
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"missing","arguments":{}}}`, -32602},
	} {
		w := request(t, handler, token, "", tc.body)
		var response struct {
			Error struct {
				Code int `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Error.Code != tc.code {
			t.Fatalf("protocol error: %d %s (%v)", w.Code, w.Body, err)
		}
	}
	if w := request(t, handler, token, "", strings.Repeat("x", (1<<20)+1)); w.Code != 413 {
		t.Fatalf("oversize: %d %s", w.Code, w.Body)
	}
	for _, method := range []string{"GET", "DELETE"} {
		r, _ := http.NewRequestWithContext(ctx, method, srv.URL+"/mcp", nil)
		resp, err := (&http.Client{Transport: bearerTransport{token}}).Do(r)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 405 {
			t.Fatalf("%s: %d", method, resp.StatusCode)
		}
	}
	shutdownCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := srv.Config.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentRequestsAreBounded(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	store, token := setup(t, conn)
	handler, err := NewHandler(true, testConfig, store)
	if err != nil {
		t.Fatal(err)
	}
	// Hold SQLite's only connection to keep authentication requests in flight.
	held, err := conn.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{}, maxConcurrentRequests)
	for i := 0; i < maxConcurrentRequests; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			r := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{}`)).WithContext(ctx)
			r.Header.Set("Authorization", "Bearer "+token)
			handler.ServeHTTP(httptest.NewRecorder(), r)
		}()
	}
	deadline := time.Now().Add(5 * time.Second)
	for conn.Stats().WaitCount < int64(maxConcurrentRequests) {
		if time.Now().After(deadline) {
			t.Fatal("authentication requests did not start")
		}
		time.Sleep(time.Millisecond)
	}
	if w := request(t, handler, token, "", `{}`); w.Code != 429 {
		t.Fatalf("saturation: %d %s", w.Code, w.Body)
	}
	cancel()
	for i := 0; i < maxConcurrentRequests; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("request did not stop on cancellation")
		}
	}
}

func TestSlowRequestBodyIsBounded(t *testing.T) {
	store, token := setup(t, dbtest.OpenSQLite(t))
	handler, err := NewHandler(true, testConfig, store)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(requestTimeout + 5*time.Second)); err != nil {
		t.Fatal(err)
	}
	// Send valid headers, but never finish the declared body.
	_, err = fmt.Fprintf(conn, "POST /mcp HTTP/1.1\r\nHost: example.com\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\nAccept: application/json, text/event-stream\r\nContent-Length: 100\r\n\r\n{", token)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	// The SDK's body-read deadline and HTTP handler timeout can race.
	if response.StatusCode != 400 && response.StatusCode != 503 {
		t.Fatalf("slow body: %d", response.StatusCode)
	}
	shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := srv.Config.Shutdown(shutdown); err != nil {
		t.Fatal(err)
	}
}

func TestPublicHostThroughLoopbackProxy(t *testing.T) {
	store, token := setup(t, dbtest.OpenSQLite(t))
	handler, err := NewHandler(true, testConfig, store)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	defer srv.Close()
	r, err := http.NewRequest("POST", srv.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	r.Host = "julius.example"
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	response, err := srv.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("proxy host: %d %s", response.StatusCode, body)
	}
}
