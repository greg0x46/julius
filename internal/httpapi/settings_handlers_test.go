package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

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
		if len(fields) != 3 || fields["brapi_token"] != nil {
			t.Fatalf("unexpected GET fields: %v", fields)
		}
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
	for _, method := range []string{"GET", "PUT"} {
		resp := doJSON(t, method, locked.URL+"/api/settings/quotes", map[string]any{"enabled": false})
		resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Fatalf("%s without session: %d", method, resp.StatusCode)
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
