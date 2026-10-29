package mcp

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/marketdata"
	"github.com/greg0x46/julius/internal/quotes"
	"github.com/greg0x46/julius/internal/settings"
	"github.com/greg0x46/julius/internal/syncsvc"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shopspring/decimal"
)

func operationsToken(t *testing.T, store *auth.Store, scope string) string {
	t.Helper()
	f := newFlow(t, store)
	f.params.Set("scope", scope)
	code := f.approve(t)
	w := oauthRequest(f.handler, "POST", "/oauth/token", f.exchange(code).Encode(), nil, "")
	var out struct {
		Token string `json:"access_token"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Token == "" {
		t.Fatalf("token: %d %s", w.Code, w.Body)
	}
	return out.Token
}
func callOperation(t *testing.T, h http.Handler, token, name string, args any, out any) bool {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
	w := request(t, h, token, "", string(body))
	var envelope struct {
		Result *sdk.CallToolResult `json:"result"`
		Error  any                 `json:"error"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &envelope) != nil || envelope.Error != nil || envelope.Result == nil {
		t.Fatalf("call %s: %d %s", name, w.Code, w.Body)
	}
	if envelope.Result.IsError {
		return false
	}
	raw, _ := json.Marshal(envelope.Result.StructuredContent)
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatal(err)
		}
	}
	return true
}
func mustOperation(t *testing.T, h http.Handler, token, name string, args any) operationOutput {
	t.Helper()
	var out operationOutput
	if !callOperation(t, h, token, name, args, &out) {
		t.Fatalf("%s failed", name)
	}
	return out
}
func pollOperation(t *testing.T, h http.Handler, token, id string) operationOutput {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		out := mustOperation(t, h, token, "get_operation_result", map[string]any{"request_id": id})
		if out.State != "running" {
			return out
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("operation did not finish")
	return operationOutput{}
}
func configureOperations(t *testing.T, conn *sql.DB) *settings.Secrets {
	t.Helper()
	key := bytes.Repeat([]byte{42}, 32)
	for k, v := range map[string]string{"pluggy.client_id": "synthetic-client", "pluggy.client_secret": "NEVER-EXPOSE-THIS"} {
		if err := settings.Set(t.Context(), conn, k, v, true, key); err != nil {
			t.Fatal(err)
		}
	}
	return settings.NewSecrets(key)
}
func operationSource(t *testing.T, conn *sql.DB, provider string, active bool) string {
	t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	if _, err := conn.Exec(`INSERT INTO data_sources(id,provider,external_item_id,is_active,created_at,updated_at) VALUES(?,?,?,?,?,?)`, id, provider, "PRIVATE-PROVIDER-ITEM-"+id, active, now, now); err != nil {
		t.Fatal(err)
	}
	return id
}
func TestMCPOperationsScopesAndConsent(t *testing.T) {
	all := mcpScope + " " + writeScope + " " + operationsScope
	for _, tc := range []struct{ in, want string }{
		{operationsScope + " " + mcpScope + " " + writeScope, all}, {mcpScope + " " + operationsScope, ""}, {all + " extra", ""}, {all + " " + writeScope, ""},
	} {
		if got := normalizeScope(tc.in); got != tc.want {
			t.Fatalf("normalize %q = %q", tc.in, got)
		}
	}
	conn := dbtest.OpenSQLite(t)
	store, readToken := setup(t, conn)
	opsToken := operationsToken(t, store, all)
	writeToken := operationsToken(t, store, mcpScope+" "+writeScope)
	h, err := NewHandler(true, testConfig, store, Operations{Context: t.Context(), Market: marketdata.New(), Secrets: configureOperations(t, conn)})
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{readToken, writeToken} {
		for _, tool := range []string{"request_sync", "request_quote_refresh", "get_operation_result"} {
			args := map[string]any{"idempotency_key": "scope-test"}
			if tool == "get_operation_result" {
				args = map[string]any{"request_id": uuid.NewString()}
			}
			if callOperation(t, h, token, tool, args, nil) {
				t.Fatalf("%s authorized without operations scope", tool)
			}
		}
	}
	var status httpapi.MCPWriteResult
	if !callOperation(t, h, readToken, "get_operations_status", map[string]any{}, &status) || strings.Contains(string(status.Data), "NEVER-EXPOSE") {
		t.Fatalf("status: %+v", status)
	}
	var value map[string]any
	_ = json.Unmarshal(status.Data, &value)
	if value["sync_ready"] != true {
		t.Fatalf("readiness: %s", status.Data)
	}
	// Real consent rendering retains both financial approval and operation permissions.
	o := &oauthServer{config: testConfig}
	w := httptest.NewRecorder()
	o.renderConsent(w, 200, grant{Scope: all}, "synthetic", "request", "", "", all)
	for _, text := range []string{"Cada alteração financeira exige sua aprovação no Julius.", "sem uma nova aprovação por operação"} {
		if !strings.Contains(w.Body.String(), text) {
			t.Fatalf("missing consent: %s", text)
		}
	}
	if strings.Contains(w.Body.String(), "Acesso somente para leitura.") {
		t.Fatal("operations consent claims read-only")
	}
	// The stronger grant still permits ordinary financial-write preparation.
	var preview writeToolOutput
	if !callOperation(t, h, opsToken, "create_category", map[string]any{"data": map[string]any{"name": "Synthetic", "kind": "expense", "icon": "coffee", "color": "#2a78d6"}}, &preview) || preview.State != "pending" {
		t.Fatalf("financial write scope regression: %+v", preview)
	}
}
func TestMCPSyncOperations(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, readToken := setup(t, conn)
		token := operationsToken(t, store, mcpScope+" "+writeScope+" "+operationsScope)
		secrets := configureOperations(t, conn)
		h, err := NewHandler(true, testConfig, store, Operations{Context: t.Context(), Secrets: secrets})
		if err != nil {
			t.Fatal(err)
		}
		source := operationSource(t, conn, "pluggy", true)
		args := map[string]any{"idempotency_key": "same-sync-request", "source_id": source}
		outputs := make(chan operationOutput, 8)
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() { outputs <- mustOperation(t, h, token, "request_sync", args) })
		}
		wg.Wait()
		close(outputs)
		var first operationOutput
		for out := range outputs {
			if first.RequestID == "" {
				first = out
			}
			if out.RequestID != first.RequestID || out.State != "accepted" || out.Result.Status != 202 {
				t.Fatalf("duplicate: %+v", out)
			}
		}
		var created struct {
			Runs []struct{ ID, Status string } `json:"runs"`
		}
		if json.Unmarshal(first.Result.Data, &created) != nil || len(created.Runs) != 1 || created.Runs[0].Status != "in_progress" {
			t.Fatalf("not queued: %+v", first)
		}
		run := created.Runs[0].ID
		busy := mustOperation(t, h, token, "request_sync", map[string]any{"idempotency_key": "another-request", "source_id": source})
		if busy.State != "rejected" || busy.Result.Status != 409 || !strings.Contains(string(busy.Result.Data), run) {
			t.Fatalf("busy: %+v", busy)
		}
		// Completion must not make a retry of the original request enqueue again.
		if _, err := conn.Exec(`UPDATE sync_runs SET status='completed',finished_at=? WHERE id=?`, db.FormatTime(time.Now()), run); err != nil {
			t.Fatal(err)
		}
		retried := mustOperation(t, h, token, "request_sync", args)
		if retried.RequestID != first.RequestID {
			t.Fatal("retry created a new receipt")
		}
		var count int
		_ = conn.QueryRow(`SELECT COUNT(*) FROM sync_runs`).Scan(&count)
		if count != 1 {
			t.Fatalf("duplicate runs: %d", count)
		}
		now := db.FormatTime(time.Now())
		for range 2 {
			if _, err := conn.Exec(`INSERT INTO sync_failures(id,sync_run_id,stage,error_code,safe_message,created_at) VALUES(?,?,'item','item_unavailable','NEVER-EXPOSE-THIS',?)`, uuid.NewString(), run, now); err != nil {
				t.Fatal(err)
			}
		}
		for _, tool := range []string{"list_sync_runs", "get_sync_run"} {
			input := map[string]any{"limit": 1, "offset": 0}
			if tool == "get_sync_run" {
				input["id"] = run
			}
			var result httpapi.MCPWriteResult
			if !callOperation(t, h, readToken, tool, input, &result) || strings.Contains(string(result.Data), "NEVER-EXPOSE") || strings.Contains(string(result.Data), "PRIVATE-PROVIDER") {
				t.Fatalf("history: %+v", result)
			}
			if tool == "get_sync_run" {
				var detail struct{ Failures []struct{ Message string } }
				_ = json.Unmarshal(result.Data, &detail)
				if len(detail.Failures) != 1 || detail.Failures[0].Message != syncsvc.SafeMessage("item_unavailable") {
					t.Fatalf("failures: %s", result.Data)
				}
			}
		}
		for _, source := range []string{operationSource(t, conn, "file", true), operationSource(t, conn, "pluggy", false)} {
			rejected := mustOperation(t, h, token, "request_sync", map[string]any{"idempotency_key": uuid.NewString(), "source_id": source})
			if rejected.Result.Status != 409 {
				t.Fatalf("ineligible: %+v", rejected)
			}
		}
		otherToken := operationsToken(t, store, mcpScope+" "+writeScope+" "+operationsScope)
		if callOperation(t, h, otherToken, "get_operation_result", map[string]any{"request_id": first.RequestID}, nil) {
			t.Fatal("receipt shared across tokens")
		}
		for _, tc := range []struct {
			name string
			args map[string]any
		}{
			{"request_sync", map[string]any{"idempotency_key": "bad"}},
			{"request_sync", map[string]any{"idempotency_key": "bad key value"}},
			{"request_sync", map[string]any{"idempotency_key": "same-sync-request"}},
			{"request_sync", map[string]any{"idempotency_key": "valid-key", "source_id": "not-a-uuid"}},
			{"request_quote_refresh", map[string]any{"idempotency_key": "valid-key", "source_id": source}},
			{"request_sync", map[string]any{"idempotency_key": "valid-key", "force": true}},
			{"list_sync_runs", map[string]any{"limit": 101}},
			{"list_sync_runs", map[string]any{"offset": -1}},
			{"get_sync_run", map[string]any{}},
			{"get_sync_run", map[string]any{"id": uuid.NewString()}},
			{"get_operation_result", map[string]any{"request_id": "invalid"}},
		} {
			if callOperation(t, h, token, tc.name, tc.args, nil) {
				t.Fatalf("accepted invalid %s %+v", tc.name, tc.args)
			}
		}
		unavailable, _ := NewHandler(true, testConfig, store, Operations{Context: t.Context()})
		if callOperation(t, unavailable, token, "request_sync", args, nil) || callOperation(t, unavailable, token, "request_quote_refresh", map[string]any{"idempotency_key": "valid-key"}, nil) {
			t.Fatal("accepted unavailable services")
		}
	})
}

type operationsProvider struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
	err     error
}

func (*operationsProvider) Name() string                    { return marketdata.ProviderYahoo }
func (*operationsProvider) Supports(marketdata.Market) bool { return true }
func (p *operationsProvider) Quote(ctx context.Context, _ marketdata.Instrument) (marketdata.Quote, error) {
	p.calls.Add(1)
	select {
	case p.entered <- struct{}{}:
	default:
	}
	select {
	case <-p.release:
		return marketdata.Quote{Price: decimal.NewFromInt(100), Currency: "BRL"}, p.err
	case <-ctx.Done():
		return marketdata.Quote{}, ctx.Err()
	}
}
func (*operationsProvider) History(context.Context, marketdata.Instrument, time.Time, time.Time) (marketdata.History, error) {
	return marketdata.History{}, errors.New("unused")
}
func seedOperationAsset(t *testing.T, conn *sql.DB) {
	t.Helper()
	source, symbol := "crypto", "BTC"
	asset, err := investments.CreateAsset(t.Context(), conn, investments.AssetInput{Name: "Synthetic BTC", AssetType: "crypto", CurrencyCode: "BRL", QuoteSource: &source, QuoteSymbol: &symbol})
	if err != nil {
		t.Fatal(err)
	}
	account, err := investments.CreateAccount(t.Context(), conn, investments.AccountInput{Name: "Synthetic wallet"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = investments.CreatePosition(t.Context(), conn, investments.PositionInput{AccountID: account.ID, AssetID: &asset.ID, Name: asset.Name, AssetType: asset.AssetType, InitialQuantity: decimal.NewFromInt(1), InitialUnitCost: decimal.NewFromInt(1), OccurredOn: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
}
func TestMCPQuoteOperations(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want quotes.RefreshStatus
	}{
		{"success", nil, quotes.RefreshSucceeded}, {"outage", errors.New("NEVER-EXPOSE upstream credential"), quotes.RefreshFailed}, {"limited", &marketdata.RateLimitedError{Provider: "yahoo", RetryAfter: time.Minute}, quotes.RefreshDeferred},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := dbtest.OpenSQLite(t)
			store, _ := setup(t, conn)
			token := operationsToken(t, store, mcpScope+" "+writeScope+" "+operationsScope)
			seedOperationAsset(t, conn)
			// Disabling the daily job preserves the existing explicit manual refresh policy.
			config := settings.DefaultQuoteRefreshSettings()
			config.Enabled = false
			if err := settings.SetQuoteRefresh(t.Context(), conn, config); err != nil {
				t.Fatal(err)
			}
			p := &operationsProvider{entered: make(chan struct{}, 1), release: make(chan struct{}), err: tc.err}
			market := marketdata.New(p)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			h, err := NewHandler(true, testConfig, store, Operations{Context: ctx, Market: market})
			if err != nil {
				t.Fatal(err)
			}
			args := map[string]any{"idempotency_key": "refresh-request"}
			out := mustOperation(t, h, token, "request_quote_refresh", args)
			if out.State != "running" || out.Result != nil {
				t.Fatalf("claimed synchronous success: %+v", out)
			}
			select {
			case <-p.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("refresh did not start")
			}
			same := mustOperation(t, h, token, "request_quote_refresh", args)
			if same.RequestID != out.RequestID {
				t.Fatal("duplicate refresh")
			}
			if callOperation(t, h, token, "request_quote_refresh", map[string]any{"idempotency_key": "different-request"}, nil) {
				t.Fatal("concurrent refresh accepted")
			}
			if _, err := quotes.RefreshNow(t.Context(), conn, market); !errors.Is(err, quotes.ErrRefreshRunning) {
				t.Fatalf("shared guard: %v", err)
			}
			close(p.release)
			finished := pollOperation(t, h, token, out.RequestID)
			var last quotes.LastRefresh
			if finished.State != "finished" || finished.Result == nil || json.Unmarshal(finished.Result.Data, &last) != nil || last.Status != tc.want || p.calls.Load() != 1 || strings.Contains(string(finished.Result.Data), "NEVER-EXPOSE") {
				t.Fatalf("outcome: %+v last=%+v calls=%d", finished, last, p.calls.Load())
			}
			// Even a terminal failed refresh is replayed; retrying work requires a new key.
			replay := mustOperation(t, h, token, "request_quote_refresh", args)
			if replay.RequestID != out.RequestID || replay.State != "finished" || p.calls.Load() != 1 {
				t.Fatal("terminal replay repeated network work")
			}
		})
	}
}

func TestMCPQuoteSharedGuardAndShutdown(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	store, _ := setup(t, conn)
	token := operationsToken(t, store, mcpScope+" "+writeScope+" "+operationsScope)
	seedOperationAsset(t, conn)
	p := &operationsProvider{entered: make(chan struct{}, 1), release: make(chan struct{})}
	market := marketdata.New(p)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	h, err := NewHandler(true, testConfig, store, Operations{Context: ctx, Market: market})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); _, _ = quotes.RefreshNow(ctx, conn, market) }()
	select {
	case <-p.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("external refresh did not start")
	}
	out := mustOperation(t, h, token, "request_quote_refresh", map[string]any{"idempotency_key": "shared-guard-test"})
	rejected := pollOperation(t, h, token, out.RequestID)
	if rejected.State != "rejected" || p.calls.Load() != 1 {
		t.Fatalf("duplicate external work: %+v", rejected)
	}
	close(p.release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("external refresh did not finish")
	}
	// A fresh service avoids the provider cache; shutdown must cancel accepted work.
	p = &operationsProvider{entered: make(chan struct{}, 1), release: make(chan struct{})}
	h, err = NewHandler(true, testConfig, store, Operations{Context: ctx, Market: marketdata.New(p)})
	if err != nil {
		t.Fatal(err)
	}
	out = mustOperation(t, h, token, "request_quote_refresh", map[string]any{"idempotency_key": "shutdown-test"})
	select {
	case <-p.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not start")
	}
	cancel()
	finished := pollOperation(t, h, token, out.RequestID)
	var last quotes.LastRefresh
	if finished.State != "finished" || finished.Result == nil || json.Unmarshal(finished.Result.Data, &last) != nil || last.Status != quotes.RefreshFailed {
		t.Fatalf("cancelled refresh: %+v", finished)
	}
	if callOperation(t, h, token, "request_quote_refresh", map[string]any{"idempotency_key": "after-shutdown"}, nil) {
		t.Fatal("accepted work after shutdown")
	}
}
