package httpapi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/marketdata"
	"github.com/greg0x46/julius/internal/quotes"
	"github.com/greg0x46/julius/internal/settings"
)

func TestQuoteSettingsPartialUpdatesAndSecrets(t *testing.T) {
	srv, conn := newTestServer(t)
	ctx := context.Background()
	put := func(body any, status int) {
		t.Helper()
		resp := doJSON(t, "PUT", srv.URL+"/api/settings/quotes", body)
		defer resp.Body.Close()
		if resp.StatusCode != status {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("PUT: %d %s", resp.StatusCode, body)
		}
	}
	get := func() settings.QuoteRefreshSettings {
		t.Helper()
		resp := doJSON(t, "GET", srv.URL+"/api/settings/quotes", nil)
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("GET: %d", resp.StatusCode)
		}
		var fields map[string]json.RawMessage
		if err := json.NewDecoder(resp.Body).Decode(&fields); err != nil {
			t.Fatal(err)
		}
		if len(fields) != 4 || fields["brapi_token"] != nil || string(fields["last_refresh"]) != "null" {
			t.Fatalf("unexpected GET fields: %v", fields)
		}
		delete(fields, "last_refresh")
		body, _ := json.Marshal(fields)
		var config settings.QuoteRefreshSettings
		if err := json.Unmarshal(body, &config); err != nil {
			t.Fatal(err)
		}
		return config
	}
	if got := get(); got != settings.DefaultQuoteRefreshSettings() {
		t.Fatalf("default: %+v", got)
	}
	put(map[string]any{"brapi_token": "secret-value"}, 200)
	key := make([]byte, 32)
	for i := range key {
		key[i] = 42
	}
	token := func() string {
		t.Helper()
		value, found, err := settings.GetBrapiToken(ctx, conn, key)
		if err != nil || !found {
			t.Fatalf("token: %v %v", found, err)
		}
		return value
	}
	put(map[string]any{"enabled": false, "time": "08:15", "timezone": "Asia/Tokyo"}, 200)
	expected := settings.QuoteRefreshSettings{Enabled: false, Time: "08:15", Timezone: "Asia/Tokyo"}
	if get() != expected || token() != "secret-value" {
		t.Fatal("schedule update changed token")
	}
	put(map[string]any{"brapi_token": "replacement"}, 200)
	if get() != expected || token() != "replacement" {
		t.Fatal("token-only request changed schedule")
	}
	for _, invalid := range []map[string]any{
		{"time": "8:15", "brapi_token": "lost"}, {"time": "24:00"},
		{"timezone": ""}, {"timezone": "Unknown/Zone"},
	} {
		put(invalid, 422)
		if get() != expected || token() != "replacement" {
			t.Fatal("invalid request changed settings")
		}
	}
	// Simulate a token write failure after the valid configuration write.
	if _, err := conn.Exec(`CREATE TRIGGER reject_quote_token BEFORE UPDATE ON settings WHEN NEW.key = 'quotes.brapi_token' BEGIN SELECT RAISE(ABORT, 'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	put(map[string]any{"enabled": true, "brapi_token": "rejected"}, 503)
	if get() != expected || token() != "replacement" {
		t.Fatal("failed token write did not roll back configuration")
	}
	if _, err := conn.Exec(`DROP TRIGGER reject_quote_token`); err != nil {
		t.Fatal(err)
	}
	put(map[string]any{"brapi_token": ""}, 200)
	if token() != "" || get() != expected {
		t.Fatal("clear token failed")
	}
	put(map[string]any{"enabled": true}, 200)
	expected.Enabled = true
	if get() != expected {
		t.Fatal("partial update reset omitted fields")
	}
}

func TestQuoteSettingsAuthenticationAndCSRF(t *testing.T) {
	locked, _ := newLockedTestServer(t)
	for _, route := range []string{"GET /api/settings/quotes", "PUT /api/settings/quotes", "POST /api/settings/quotes/refresh"} {
		method, path, _ := strings.Cut(route, " ")
		resp := doJSON(t, method, locked.URL+path, map[string]any{"enabled": false})
		resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Fatalf("%s without session: %d", route, resp.StatusCode)
		}
	}
	srv, _ := newTestServer(t)
	req, err := http.NewRequest("PUT", srv.URL+"/api/settings/quotes", nil)
	if err != nil {
		t.Fatal(err)
	}
	token, _ := fixtureTokens.Load(req.URL.Host)
	req.AddCookie(&http.Cookie{Name: "julius_session", Value: token.(string)})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("missing CSRF: %d", resp.StatusCode)
	}
}

type refreshProvider struct{ err error }

func (p refreshProvider) Name() string                    { return marketdata.ProviderYahoo }
func (p refreshProvider) Supports(marketdata.Market) bool { return true }
func (p refreshProvider) Quote(context.Context, marketdata.Instrument) (marketdata.Quote, error) {
	if p.err != nil {
		return marketdata.Quote{}, p.err
	}
	return marketdata.Quote{Price: decimal.NewFromInt(100), Currency: "BRL"}, nil
}
func (p refreshProvider) History(context.Context, marketdata.Instrument, time.Time, time.Time) (marketdata.History, error) {
	return marketdata.History{}, marketdata.ErrNotFound
}

func TestQuoteRefreshNowReportsOutcome(t *testing.T) {
	provider := &refreshProvider{}
	srv, conn := loggedIn(t, func(t *testing.T) (*httptest.Server, *sql.DB, *settings.Secrets) {
		return newTestServerWithMarket(t, marketdata.New(provider))
	})
	ctx := context.Background()
	source, symbol := "crypto", "BTC"
	asset, err := investments.CreateAsset(ctx, conn, investments.AssetInput{
		Name: "Bitcoin", AssetType: "crypto", CurrencyCode: "BRL", QuoteSource: &source, QuoteSymbol: &symbol,
	})
	if err != nil {
		t.Fatal(err)
	}
	account, err := investments.CreateAccount(ctx, conn, investments.AccountInput{Name: "wallet"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := investments.CreatePosition(ctx, conn, investments.PositionInput{
		AccountID: account.ID, AssetID: &asset.ID, Name: asset.Name, AssetType: asset.AssetType,
		InitialQuantity: decimal.NewFromInt(1), InitialUnitCost: decimal.NewFromInt(1),
		OccurredOn: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	refresh := func() quotes.LastRefresh {
		t.Helper()
		resp := doJSON(t, "POST", srv.URL+"/api/settings/quotes/refresh", nil)
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("POST refresh: %d %s", resp.StatusCode, body)
		}
		var last quotes.LastRefresh
		if err := json.NewDecoder(resp.Body).Decode(&last); err != nil {
			t.Fatal(err)
		}
		return last
	}
	// The service caches successful quotes, so failures run first.
	if err := investments.UpsertConnectorQuotes(ctx, conn, []investments.AssetQuote{{
		AssetID: asset.ID, QuotedOn: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Price: decimal.NewFromInt(90),
		Source: marketdata.ProviderYahoo, Origin: investments.QuoteOriginMarket,
	}}); err != nil {
		t.Fatal(err)
	}
	provider.err = &marketdata.RateLimitedError{Provider: marketdata.ProviderYahoo, RetryAfter: time.Minute}
	if last := refresh(); last.Status != quotes.RefreshDeferred || last.Deferred != 1 {
		t.Fatalf("rate limited: %+v", last)
	}
	provider.err = marketdata.ErrUnavailable
	if last := refresh(); last.Status != quotes.RefreshFailed || last.Failures != 1 {
		t.Fatalf("failure: %+v", last)
	}
	stored, err := investments.ListAssetQuotes(ctx, conn, asset.ID, nil)
	if err != nil || len(stored) != 1 || stored[0].Price.String() != "90" {
		t.Fatalf("failure removed stored price: %+v %v", stored, err)
	}
	provider.err = nil
	if last := refresh(); last.Status != quotes.RefreshSucceeded || last.Trigger != "manual" || last.Fetched != 1 {
		t.Fatalf("success: %+v", last)
	}

	resp := doJSON(t, "GET", srv.URL+"/api/settings/quotes", nil)
	defer resp.Body.Close()
	var got struct {
		LastRefresh *quotes.LastRefresh `json:"last_refresh"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil || got.LastRefresh == nil || got.LastRefresh.Status != quotes.RefreshSucceeded {
		t.Fatalf("GET last_refresh: %+v %v", got.LastRefresh, err)
	}
}

func TestQuoteRefreshNowWithoutMarketData(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := doJSON(t, "POST", srv.URL+"/api/settings/quotes/refresh", nil)
	resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
}
