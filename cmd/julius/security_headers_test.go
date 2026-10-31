package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/mcp"
	"github.com/greg0x46/julius/internal/settings"
)

// oauthCSP is the stricter policy of the /oauth/* and /mcp routes; the
// app-wide policy must never replace it.
const oauthCSP = "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'"

// headerServer serves the production handler stack on a fresh database.
func headerServer(t *testing.T, publicURL string, mcpEnabled bool) *httptest.Server {
	t.Helper()
	conn := dbtest.OpenSQLite(t)
	master := bytes.Repeat([]byte{7}, 32)
	store := auth.NewStore(conn)
	if err := store.Initialize(context.Background(), "owner@example.test", "synthetic-header-password", master, nil); err != nil {
		t.Fatal(err)
	}
	cfg := auth.Config{PublicURL: publicURL}
	frontend := fstest.MapFS{"index.html": {Data: []byte("<html>spa</html>")}, "assets/app.js": {Data: []byte("console.log(1)")}}
	mcpHandler, err := mcp.NewHandler(mcpEnabled, cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	api := httpapi.NewServer(conn, frontend, settings.NewSecrets(master), cfg, nil)
	ts := httptest.NewServer(publicHandler(slog.New(slog.NewJSONHandler(io.Discard, nil)), cfg, api, mcpHandler))
	t.Cleanup(ts.Close)
	return ts
}

func headerGet(t *testing.T, ts *httptest.Server, method, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp
}

func TestSecurityHeadersOnEveryAppRoute(t *testing.T) {
	ts := headerServer(t, "http://localhost:4200", true)
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/", 200},
		{"GET", "/transacoes", 200},
		{"GET", "/assets/app.js", 200},
		{"GET", "/assets/missing.js", 404},
		{"GET", "/health", 200},
		{"GET", "/api/auth/session", 200},
		{"GET", "/api/categories", 401},
		{"GET", "/api/unknown", 401},
		{"POST", "/api/auth/login", 403},
	} {
		resp := headerGet(t, ts, tc.method, tc.path)
		if resp.StatusCode != tc.status {
			t.Errorf("%s %s: status %d, want %d", tc.method, tc.path, resp.StatusCode, tc.status)
		}
		h := resp.Header
		if got := h.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s %s: X-Content-Type-Options = %q", tc.method, tc.path, got)
		}
		if got := h.Get("X-Frame-Options"); got != "DENY" {
			t.Errorf("%s %s: X-Frame-Options = %q", tc.method, tc.path, got)
		}
		if got := h.Get("Referrer-Policy"); got != "same-origin" {
			t.Errorf("%s %s: Referrer-Policy = %q", tc.method, tc.path, got)
		}
		csp := h.Get("Content-Security-Policy")
		for _, directive := range []string{"default-src 'self'", "frame-ancestors 'none'", "base-uri 'none'", "form-action 'self'", "object-src 'none'"} {
			if !strings.Contains(csp, directive) {
				t.Errorf("%s %s: CSP %q lacks %q", tc.method, tc.path, csp, directive)
			}
		}
		for _, weak := range []string{"unsafe-inline", "unsafe-eval", "*"} {
			if strings.Contains(csp, weak) {
				t.Errorf("%s %s: CSP %q must not contain %q", tc.method, tc.path, csp, weak)
			}
		}
		if got := h.Get("Strict-Transport-Security"); got != "" {
			t.Errorf("%s %s: HSTS %q on an HTTP public origin", tc.method, tc.path, got)
		}
	}
}

func TestSecurityHeadersKeepStricterOAuthAndMCPPolicy(t *testing.T) {
	ts := headerServer(t, "http://localhost:4200", true)
	for _, tc := range []struct{ method, path string }{
		{"GET", "/oauth/authorize"},
		{"POST", "/oauth/token"},
		{"GET", "/oauth/write"},
		{"GET", "/.well-known/oauth-authorization-server"},
		{"GET", "/.well-known/oauth-protected-resource/mcp"},
		{"POST", "/mcp"},
	} {
		resp := headerGet(t, ts, tc.method, tc.path)
		if got := resp.Header.Get("Content-Security-Policy"); got != oauthCSP {
			t.Errorf("%s %s: CSP = %q, want the OAuth policy", tc.method, tc.path, got)
		}
		if got := resp.Header.Get("Referrer-Policy"); got != "same-origin" {
			t.Errorf("%s %s: Referrer-Policy = %q", tc.method, tc.path, got)
		}
		if got := resp.Header.Get("X-Frame-Options"); got != "DENY" {
			t.Errorf("%s %s: X-Frame-Options = %q", tc.method, tc.path, got)
		}
		if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s %s: X-Content-Type-Options = %q", tc.method, tc.path, got)
		}
		if got := resp.Header.Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s %s: Cache-Control = %q", tc.method, tc.path, got)
		}
	}
}

func TestSecurityHeadersWhenMCPIsDisabled(t *testing.T) {
	ts := headerServer(t, "http://localhost:4200", false)
	for _, path := range []string{"/mcp", "/mcp/", "/oauth/authorize"} {
		resp := headerGet(t, ts, "GET", path)
		if resp.StatusCode != 404 {
			t.Errorf("%s: status %d, want 404", path, resp.StatusCode)
		}
		if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s: X-Content-Type-Options = %q", path, got)
		}
		if got := resp.Header.Get("X-Frame-Options"); got != "DENY" {
			t.Errorf("%s: X-Frame-Options = %q", path, got)
		}
		if got := resp.Header.Get("Content-Security-Policy"); !strings.Contains(got, "frame-ancestors 'none'") {
			t.Errorf("%s: CSP = %q", path, got)
		}
	}
}

func TestSecurityHeadersStrictTransportSecurityOnlyForHTTPSOrigin(t *testing.T) {
	secure := headerServer(t, "https://julius.example", true)
	for _, tc := range []struct{ method, path string }{
		{"GET", "/"},
		{"GET", "/health"},
		{"GET", "/api/categories"},
		{"GET", "/oauth/authorize"},
		{"POST", "/mcp"},
		{"GET", "/mcp/"},
	} {
		got := headerGet(t, secure, tc.method, tc.path).Header.Get("Strict-Transport-Security")
		if got != "max-age=31536000" {
			t.Errorf("https %s %s: Strict-Transport-Security = %q, want max-age=31536000 only", tc.method, tc.path, got)
		}
	}
	plain := headerServer(t, "http://127.0.0.1:4200", true)
	for _, path := range []string{"/", "/health", "/api/categories", "/oauth/authorize"} {
		if got := headerGet(t, plain, "GET", path).Header.Get("Strict-Transport-Security"); got != "" {
			t.Errorf("http %s: Strict-Transport-Security = %q, want none", path, got)
		}
	}
}
