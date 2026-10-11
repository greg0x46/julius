package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/settings"
)

func TestQuoteSettingsPostgresRoundTrip(t *testing.T) {
	base := os.Getenv("JULIUS_TEST_POSTGRES_DSN")
	if base == "" {
		t.Skip("JULIUS_TEST_POSTGRES_DSN not set")
	}
	raw, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	schema := "quote_settings_test_" + fmt.Sprintf("%x", uuid.New())
	if _, err := raw.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	defer raw.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	conn, err := db.Open(u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	master := bytes.Repeat([]byte{42}, 32)
	store := auth.NewStore(conn)
	if err := store.Initialize(context.Background(), "owner@example.com", testPassword, master, nil); err != nil {
		t.Fatal(err)
	}
	token, err := store.Login(context.Background(), "owner@example.com", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(httpapi.NewServer(conn, fstest.MapFS{"index.html": {Data: []byte("spa")}}, settings.NewSecrets(master), auth.Config{PublicURL: testOrigin}))
	defer srv.Close()
	endpoint, _ := url.Parse(srv.URL)
	fixtureTokens.Store(endpoint.Host, token)
	defer fixtureTokens.Delete(endpoint.Host)
	resp := doJSON(t, "PUT", srv.URL+"/api/settings/quotes", map[string]any{"enabled": false, "time": "09:20", "timezone": "UTC", "brapi_token": "secret"})
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("PUT: %d", resp.StatusCode)
	}
	resp = doJSON(t, "PUT", srv.URL+"/api/settings/quotes", map[string]any{"time": "10:25"})
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("partial PUT: %d", resp.StatusCode)
	}
	resp = doJSON(t, "GET", srv.URL+"/api/settings/quotes", nil)
	defer resp.Body.Close()
	var got settings.QuoteRefreshSettings
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	expected := settings.QuoteRefreshSettings{Enabled: false, Time: "10:25", Timezone: "UTC"}
	if resp.StatusCode != 200 || got != expected {
		t.Fatalf("GET: %d %+v", resp.StatusCode, got)
	}
	saved, found, err := settings.GetBrapiToken(context.Background(), conn, master)
	if err != nil || !found || saved != "secret" {
		t.Fatalf("token: %v %v", found, err)
	}
}
