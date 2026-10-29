package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/mcpaccess"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/transactions"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type writeFixture struct {
	conn                      *sql.DB
	store                     *auth.Store
	handler                   http.Handler
	token, readToken, account string
	session                   *sdk.ClientSession
}

func newWriteFixture(t *testing.T, conn *sql.DB) *writeFixture {
	t.Helper()
	store, readToken := setup(t, conn)
	f := newFlow(t, store)
	f.params.Set("scope", writeScope+" "+mcpScope)
	code := f.approve(t)
	w := oauthRequest(f.handler, "POST", "/oauth/token", f.exchange(code).Encode(), nil, "")
	var grant struct {
		Token string `json:"access_token"`
		Scope string `json:"scope"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &grant) != nil || grant.Scope != mcpScope+" "+writeScope {
		t.Fatalf("write grant: %d %s", w.Code, w.Body)
	}
	handler, err := NewHandler(true, testConfig, store)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &writeFixture{conn: conn, store: store, handler: handler, token: grant.Token, readToken: readToken, account: uuid.NewString()}
	now := db.FormatTime(time.Now())
	if _, err = conn.Exec(`INSERT INTO financial_accounts(id,origin,name,account_type,currency_code,opening_balance,created_at,updated_at) VALUES(?,'manual','Synthetic cash','BANK','BRL','100',?,?)`, fixture.account, now, now); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client := sdk.NewClient(&sdk.Implementation{Name: "synthetic-writer", Version: "1"}, nil)
	fixture.session, err = client.Connect(context.Background(), &sdk.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: &http.Client{Transport: bearerTransport{grant.Token}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fixture.session.Close() })
	return fixture
}
func (f *writeFixture) call(t *testing.T, tool string, args any) writeToolOutput {
	t.Helper()
	result, err := f.session.CallTool(context.Background(), &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil || result.IsError {
		content, _ := json.Marshal(result)
		t.Fatalf("%s failed: %s %v", tool, content, err)
	}
	raw, _ := json.Marshal(result.StructuredContent)
	var out writeToolOutput
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func (f *writeFixture) manual() map[string]any {
	return map[string]any{"account_id": f.account, "description": "Synthetic feira <script>", "amount": "-12.34567890123456789", "occurred_at": "2026-10-01"}
}
func (f *writeFixture) approve(t *testing.T, out writeToolOutput) *httptest.ResponseRecorder {
	t.Helper()
	u, err := url.Parse(out.ApprovalURL)
	if err != nil {
		t.Fatal(err)
	}
	w := oauthRequest(f.handler, "GET", u.RequestURI(), "", nil, "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "<script>") {
		t.Fatalf("preview: %d %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "Aprovar e aplicar alteração") {
		t.Fatalf("missing approval form: %s", w.Body)
	}
	cookie := w.Result().Cookies()[0]
	form := url.Values{"request": {out.RequestID}, "decision": {"allow"}, "email": {"owner@example.com"}, "password": {"synthetic-password"}}
	w = oauthRequest(f.handler, "POST", "/oauth/write", form.Encode(), cookie, testConfig.PublicURL)
	if w.Code != 200 {
		t.Fatalf("approval: %d %s", w.Code, w.Body)
	}
	// Retrying the exact approval POST must return the terminal state.
	again := oauthRequest(f.handler, "POST", "/oauth/write", form.Encode(), cookie, testConfig.PublicURL)
	if again.Code != 200 || again.Body.String() != w.Body.String() {
		t.Fatalf("approval replay changed result: %d", again.Code)
	}
	return w
}
func (f *writeFixture) result(t *testing.T, id string) writeToolOutput {
	return f.call(t, "get_write_result", map[string]any{"request_id": id})
}
func TestMCPWriteManualLifecycle(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		f := newWriteFixture(t, conn)
		cat, err := categories.Create(context.Background(), conn, "Synthetic food", money.Expense, "coffee", "#2a78d6")
		if err != nil {
			t.Fatal(err)
		}
		args := map[string]any{"data": f.manual(), "idempotency_key": "synthetic-create-1"}
		out := f.call(t, "create_manual_transaction", args)
		var count int
		_ = conn.QueryRow(`SELECT COUNT(*) FROM financial_transactions`).Scan(&count)
		if count != 0 {
			t.Fatal("prepare wrote data")
		}
		f.approve(t, out)
		result := f.result(t, out.RequestID)
		if result.State != "applied" || result.Result.Status != 201 {
			t.Fatalf("create: %+v", result)
		}
		var item struct{ ID, Amount string }
		_ = json.Unmarshal(result.Result.Data, &item)
		if item.Amount != "-12.34567890123456789" {
			t.Fatalf("amount rounded: %s", item.Amount)
		}
		out = f.call(t, "create_manual_transaction", args)
		f.approve(t, out)
		repeated := f.result(t, out.RequestID)
		if string(repeated.Result.Data) != string(result.Result.Data) {
			t.Fatalf("idempotent retry differs: %s", repeated.Result.Data)
		}
		out = f.call(t, "assign_transaction_category", map[string]any{"id": item.ID, "data": map[string]any{"category_id": cat.ID}})
		f.approve(t, out)
		current, found, err := transactions.GetItem(context.Background(), conn, item.ID)
		if err != nil || !found || current.InternalCategory == nil || current.InternalCategory.ID != cat.ID {
			t.Fatalf("category not applied: %+v %v", current, err)
		}
		data := f.manual()
		data["amount"] = "-15.000000001"
		out = f.call(t, "update_manual_transaction", map[string]any{"id": item.ID, "data": data})
		f.approve(t, out)
		balance, err := transactions.AccountBalance(context.Background(), conn, f.account)
		if err != nil || balance == nil || *balance != "84.999999999" {
			t.Fatalf("balance: %v %v", balance, err)
		}
		out = f.call(t, "delete_manual_transaction", map[string]any{"id": item.ID, "data": map[string]any{}})
		f.approve(t, out)
		_, found, err = transactions.GetItem(context.Background(), conn, item.ID)
		if err != nil || found {
			t.Fatal("delete not applied")
		}
		var deleted sql.NullString
		if err = conn.QueryRow(`SELECT deleted_at FROM financial_transactions WHERE id=?`, item.ID).Scan(&deleted); err != nil || !deleted.Valid {
			t.Fatalf("not soft deleted: %v", err)
		}
	})
}
func TestMCPWriteApprovalBoundaries(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		f := newWriteFixture(t, conn)
		args := map[string]any{"data": f.manual(), "idempotency_key": "synthetic-key"}
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "create_manual_transaction", "arguments": args}})
		for _, token := range []string{"", f.readToken} {
			w := request(t, f.handler, token, "", string(raw))
			if token == "" && w.Code != 401 {
				t.Fatal("anonymous write accepted")
			}
			if token != "" && !strings.Contains(w.Body.String(), "mcp:write scope required") {
				t.Fatalf("read scope accepted: %s", w.Body)
			}
		}
		out := f.call(t, "create_manual_transaction", args)
		u, _ := url.Parse(out.ApprovalURL)
		w := oauthRequest(f.handler, "GET", u.RequestURI(), "", nil, "")
		cookie := w.Result().Cookies()[0]
		form := url.Values{"request": {out.RequestID}, "decision": {"allow"}, "email": {"owner@example.com"}, "password": {"synthetic-password"}}
		for _, origin := range []string{"", "https://attacker.example"} {
			if w = oauthRequest(f.handler, "POST", "/oauth/write", form.Encode(), cookie, origin); w.Code != 403 {
				t.Fatal("wrong origin accepted")
			}
		}
		if w = oauthRequest(f.handler, "POST", "/oauth/write", form.Encode(), nil, testConfig.PublicURL); w.Code != 403 {
			t.Fatal("missing CSRF accepted")
		}
		form.Set("password", "wrong")
		w = oauthRequest(f.handler, "POST", "/oauth/write", form.Encode(), cookie, testConfig.PublicURL)
		if w.Code != 401 || !strings.Contains(w.Body.String(), "E-mail ou senha inválidos") || strings.Contains(w.Body.String(), `value="wrong"`) {
			t.Fatal("invalid credentials accepted or echoed")
		}
		form.Set("decision", "deny")
		w = oauthRequest(f.handler, "POST", "/oauth/write", form.Encode(), cookie, testConfig.PublicURL)
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		form.Set("decision", "allow")
		form.Set("password", "synthetic-password")
		_ = oauthRequest(f.handler, "POST", "/oauth/write", form.Encode(), cookie, testConfig.PublicURL)
		if f.result(t, out.RequestID).State != "denied" {
			t.Fatal("denial replay applied")
		}
		out = f.call(t, "create_manual_transaction", args)
		// Another valid write token cannot claim this request.
		if _, err := conn.Exec(`UPDATE oauth_grants SET scope=? WHERE token_hash=?`, mcpScope+" "+writeScope, tokenHash(f.readToken)); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(`UPDATE mcp_authorizations SET scope=?, approved_scope=? WHERE id=(SELECT authorization_id FROM oauth_grants WHERE token_hash=?)`, mcpScope+" "+writeScope, mcpScope+" "+writeScope, tokenHash(f.readToken)); err != nil {
			t.Fatal(err)
		}
		raw, _ = json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "get_write_result", "arguments": map[string]any{"request_id": out.RequestID}}})
		w = request(t, f.handler, f.readToken, "", string(raw))
		if !strings.Contains(w.Body.String(), "unavailable or expired") {
			t.Fatalf("cross-token result: %s", w.Body)
		}
		if _, err := conn.Exec(`DELETE FROM oauth_grants WHERE token_hash=?`, tokenHash(f.token)); err != nil {
			t.Fatal(err)
		}
		u, _ = url.Parse(out.ApprovalURL)
		w = oauthRequest(f.handler, "GET", u.RequestURI(), "", nil, "")
		if w.Code != 403 {
			t.Fatal("revoked token approval accepted")
		}
		var count int
		_ = conn.QueryRow(`SELECT COUNT(*) FROM financial_transactions`).Scan(&count)
		if count != 0 {
			t.Fatal("boundary failure modified financial data")
		}
	})
}
func TestMCPWritePendingAuthorizationChanges(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		global      bool
	}{
		{"revoked", `UPDATE mcp_authorizations SET revoked_at=created_at WHERE id=?`, false},
		{"read-only connection", `UPDATE mcp_authorizations SET scope='mcp:access' WHERE id=?`, false},
		{"expired connection", `UPDATE mcp_authorizations SET expires_at=created_at WHERE id=?`, false},
		{"stale connection revision", `UPDATE mcp_authorizations SET revision=revision+1 WHERE id=?`, false},
		{"read-only token", `UPDATE oauth_grants SET scope='mcp:access' WHERE authorization_id=?`, false},
		{"expired token", `UPDATE oauth_grants SET expires_at='2000-01-01T00:00:00Z' WHERE authorization_id=?`, false},
		{"owner revision", `UPDATE auth_owner SET revision=revision+1 WHERE id=1 AND CAST(? AS TEXT) IS NOT NULL`, true},
		{"MCP disabled", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
				f := newWriteFixture(t, conn)
				out := f.call(t, "create_manual_transaction", map[string]any{"data": f.manual(), "idempotency_key": "synthetic-revoked"})
				u, _ := url.Parse(out.ApprovalURL)
				w := oauthRequest(f.handler, "GET", u.RequestURI(), "", nil, "")
				if w.Code != 200 {
					t.Fatalf("initial preview: %d", w.Code)
				}
				cookie := w.Result().Cookies()[0]
				id := authorizationID(t, conn, f.token)
				if tc.name == "MCP disabled" {
					if err := mcpaccess.SetEnabled(context.Background(), conn, false); err != nil {
						t.Fatal(err)
					}
				} else if _, err := conn.Exec(tc.query, id); err != nil {
					t.Fatal(err)
				}
				if w = oauthRequest(f.handler, "GET", u.RequestURI(), "", nil, ""); w.Code != 403 {
					t.Fatalf("changed authorization preview: %d", w.Code)
				}
				form := url.Values{"request": {out.RequestID}, "decision": {"allow"}, "email": {"owner@example.com"}, "password": {"synthetic-password"}}
				if w = oauthRequest(f.handler, "POST", "/oauth/write", form.Encode(), cookie, testConfig.PublicURL); w.Code != 403 {
					t.Fatalf("changed authorization approval: %d", w.Code)
				}
				var count int
				if err := conn.QueryRow(`SELECT COUNT(*) FROM financial_transactions`).Scan(&count); err != nil || count != 0 {
					t.Fatalf("unauthorized mutation: count=%d err=%v", count, err)
				}
				if !tc.global {
					flow := newFlow(t, f.store)
					flow.params.Set("scope", mcpaccess.ReadWrite)
					token := flow.token(t, flow.approve(t))
					srv := httptest.NewServer(f.handler)
					defer srv.Close()
					client := sdk.NewClient(&sdk.Implementation{Name: "other-writer", Version: "1"}, nil)
					session, err := client.Connect(context.Background(), &sdk.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: &http.Client{Transport: bearerTransport{token}}}, nil)
					if err != nil {
						t.Fatal(err)
					}
					defer session.Close()
					other := *f
					other.session = session
					otherOut := other.call(t, "create_manual_transaction", map[string]any{"data": f.manual(), "idempotency_key": "synthetic-other"})
					other.approve(t, otherOut)
					if other.result(t, otherOut.RequestID).State != "applied" {
						t.Fatal("independent authorization could not apply write")
					}
				}
			})
		})
	}
}

func TestMCPWriteConflictFailureAndConcurrency(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		f := newWriteFixture(t, conn)
		out := f.call(t, "create_manual_transaction", map[string]any{"data": f.manual(), "idempotency_key": "synthetic-conflict"})
		if _, err := conn.Exec(`UPDATE financial_accounts SET name='Changed' WHERE id=?`, f.account); err != nil {
			t.Fatal(err)
		}
		w := f.approve(t, out)
		if !strings.Contains(html.UnescapeString(w.Body.String()), "Os dados mudaram") {
			t.Fatalf("stale preview accepted: %s", w.Body)
		}
		if f.result(t, out.RequestID).State != "failed" {
			t.Fatal("stale preview applied")
		}
		drop := dbtest.FailInserts(t, conn, "transaction_category_events", "")
		cat, err := categories.Create(context.Background(), conn, "Synthetic", money.Expense, "coffee", "#2a78d6")
		if err != nil {
			t.Fatal(err)
		}
		data := f.manual()
		data["category_id"] = cat.ID
		out = f.call(t, "create_manual_transaction", map[string]any{"data": data, "idempotency_key": "synthetic-rollback"})
		f.approve(t, out)
		failed := f.result(t, out.RequestID)
		if failed.State != "failed" || failed.Result.Status != 503 {
			t.Fatalf("failure: %+v", failed)
		}
		var count int
		_ = conn.QueryRow(`SELECT COUNT(*) FROM financial_transactions`).Scan(&count)
		if count != 0 {
			t.Fatal("partial transaction survived")
		}
		_ = conn.QueryRow(`SELECT COUNT(*) FROM manual_transaction_idempotency`).Scan(&count)
		if count != 0 {
			t.Fatal("failed idempotency key survived")
		}
		drop()
		out = f.call(t, "create_manual_transaction", map[string]any{"data": data, "idempotency_key": "synthetic-rollback"})
		u, _ := url.Parse(out.ApprovalURL)
		w = oauthRequest(f.handler, "GET", u.RequestURI(), "", nil, "")
		cookie := w.Result().Cookies()[0]
		form := url.Values{"request": {out.RequestID}, "decision": {"allow"}, "email": {"owner@example.com"}, "password": {"synthetic-password"}}
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				oauthRequest(f.handler, "POST", "/oauth/write", form.Encode(), cookie, testConfig.PublicURL)
			}()
		}
		wg.Wait()
		if f.result(t, out.RequestID).State != "applied" {
			t.Fatal("concurrent approval failed")
		}
		_ = conn.QueryRow(`SELECT COUNT(*) FROM financial_transactions`).Scan(&count)
		if count != 1 {
			t.Fatalf("concurrent approvals inserted %d", count)
		}
	})
}

func TestMCPWriteIdempotencyConflictAndExpiry(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		f := newWriteFixture(t, conn)
		args := map[string]any{"data": f.manual(), "idempotency_key": "synthetic-same-key"}
		out := f.call(t, "create_manual_transaction", args)
		f.approve(t, out)
		changed := f.manual()
		changed["amount"] = "-99"
		args["data"] = changed
		out = f.call(t, "create_manual_transaction", args)
		f.approve(t, out)
		result := f.result(t, out.RequestID)
		if result.State != "failed" || result.Result.Status != 409 || !strings.Contains(string(result.Result.Data), "idempotency-key-conflict") {
			t.Fatalf("key reuse: %+v", result)
		}
		out = f.call(t, "create_manual_transaction", map[string]any{"data": f.manual(), "idempotency_key": "synthetic-expiry"})
		f.store.Now = func() time.Time { return time.Now().Add(6 * time.Minute) }
		u, _ := url.Parse(out.ApprovalURL)
		w := oauthRequest(f.handler, "GET", u.RequestURI(), "", nil, "")
		if w.Code != 410 || strings.Contains(w.Body.String(), "<form") {
			t.Fatal("expired approval still active")
		}
	})
}
func TestMCPWriteRuleToolSchemas(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	f := newWriteFixture(t, conn)
	out := f.call(t, "create_category", map[string]any{"data": map[string]any{"name": "Synthetic category", "kind": "expense", "icon": "coffee", "color": "#2a78d6"}})
	f.approve(t, out)
	result := f.result(t, out.RequestID)
	var cat struct{ ID string }
	_ = json.Unmarshal(result.Result.Data, &cat)
	out = f.call(t, "create_automation_rule", map[string]any{"data": map[string]any{"name": "Synthetic rule", "is_active": true, "logic_operator": "and", "conditions": []any{map[string]any{"field": "description", "operator": "contains", "value": "Synthetic"}}, "actions": []any{map[string]any{"type": "set_category", "category_id": cat.ID}}}})
	f.approve(t, out)
	if f.result(t, out.RequestID).State != "applied" {
		t.Fatal("rule not applied")
	}
}
