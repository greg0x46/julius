package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/settings"
)

func newDemoHandler(t *testing.T, authenticationStored bool) http.Handler {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "demo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	master := bytes.Repeat([]byte{7}, 32)
	if err := auth.NewStore(conn).Initialize(context.Background(), "demo@example.test", testPassword, master, nil); err != nil {
		t.Fatal(err)
	}
	if err := settings.SetAuthenticationEnabled(context.Background(), conn, authenticationStored); err != nil {
		t.Fatal(err)
	}
	return httpapi.NewServer(conn, fstest.MapFS{"index.html": {Data: []byte("spa")}}, settings.NewSecrets(master), auth.Config{PublicURL: testOrigin, Demo: true}, nil)
}

func demoRequest(h http.Handler, method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if method != http.MethodGet {
		req.Header.Set("Origin", testOrigin)
		req.Header.Set("X-Julius-Request", "1")
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func problemType(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var problem struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &problem)
	return problem.Type
}

// A demo is open whatever the stored switch says, and says so in the session.
func TestDemoIsAlwaysOpen(t *testing.T) {
	for _, stored := range []bool{false, true} {
		h := newDemoHandler(t, stored)
		rec := demoRequest(h, http.MethodGet, "/api/auth/session", "", nil)
		var session map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &session); err != nil || rec.Code != 200 {
			t.Fatalf("session = %d %s", rec.Code, rec.Body.String())
		}
		if session["authentication_enabled"] != false || session["demo"] != true {
			t.Fatalf("stored=%v: session = %v, want open demo", stored, session)
		}
		if rec := demoRequest(h, http.MethodGet, "/api/categories", "", nil); rec.Code != 200 {
			t.Fatalf("stored=%v: GET /api/categories = %d, want open access", stored, rec.Code)
		}
	}
}

func TestDemoBlocksAccessAndIntegrationWrites(t *testing.T) {
	h := newDemoHandler(t, false)
	for _, c := range []struct{ method, target, body string }{
		{"PUT", "/api/auth/config", `{"enabled":true}`},
		{"PUT", "/api/auth/password", `{"current_password":"a","password":"b"}`},
		{"POST", "/api/auth/login", `{"email":"a@b.c","password":"x"}`},
		{"POST", "/api/auth/logout", ``},
		{"PUT", "/api/settings/pluggy", `{"pluggy_client_id":"id","pluggy_client_secret":"secret"}`},
		{"PUT", "/api/settings/quotes", `{"enabled":true}`},
		{"POST", "/api/settings/quotes/refresh", ``},
		{"POST", "/api/sync-runs", `{}`},
		{"POST", "/api/data-sources", `{}`},
		{"PATCH", "/api/data-sources/abc", `{}`},
		// Not clean paths: the guard must not be skipped by dot segments.
		{"PUT", "/api/x/../auth/config", `{"enabled":true}`},
		{"PUT", "/api/settings/./pluggy", `{}`},
	} {
		rec := demoRequest(h, c.method, c.target, c.body, nil)
		if rec.Code != http.StatusForbidden || problemType(t, rec) != "/problems/demo-mode" {
			t.Errorf("%s %s = %d %s, want 403 demo-mode", c.method, c.target, rec.Code, rec.Body.String())
		}
	}
}

func TestDemoKeepsOriginCheckAndOrdinaryWrites(t *testing.T) {
	h := newDemoHandler(t, false)
	rec := demoRequest(h, "PUT", "/api/auth/config", `{"enabled":true}`, map[string]string{"Origin": "https://evil.example"})
	if rec.Code != 403 || problemType(t, rec) != "/problems/invalid-origin" {
		t.Fatalf("foreign origin = %d %s, want invalid-origin", rec.Code, rec.Body.String())
	}
	if rec := demoRequest(h, "GET", "/api/settings/quotes", "", nil); rec.Code != 200 {
		t.Fatalf("GET /api/settings/quotes = %d, want 200 (reads stay open)", rec.Code)
	}
	if rec := demoRequest(h, "POST", "/api/categories", `{"name":"Churrasco","kind":"expense","icon":"coffee","color":"#eb6834"}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/categories = %d %s, want 201 (the demo is editable)", rec.Code, rec.Body.String())
	}
}
