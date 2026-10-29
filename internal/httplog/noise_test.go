package httplog

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// noiseMux mimics the real layout: a few specific routes plus the SPA
// catch-all "/". The status and auth outcome come from test headers.
func noiseMux() *http.ServeMux {
	h := func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test-Reject") != "" {
			SetAuth(r.Context(), AuthSession, AuthRejected)
		}
		status, _ := strconv.Atoi(r.Header.Get("X-Test-Status"))
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
	}
	mux := http.NewServeMux()
	for _, p := range []string{
		"GET /health", "GET /api/auth/session", "GET /api/sync-runs/{id}", "GET /api/sync-runs",
		"POST /api/things", "OPTIONS /api/things", "GET /api/files/{name}", "/",
	} {
		mux.HandleFunc(p, h)
	}
	return mux
}

func TestNoiseControl(t *testing.T) {
	const acrm = "Access-Control-Request-Method"
	tests := []struct {
		name    string
		method  string
		target  string
		status  int
		reject  bool
		headers map[string]string
		logged  bool
	}{
		// static frontend assets served by the SPA catch-all
		{"js asset", "GET", "/assets/index-abc123.js", 200, false, nil, false},
		{"css asset with query", "GET", "/assets/app.css?v=1", 200, false, nil, false},
		{"asset 304", "GET", "/assets/app.js", 304, false, nil, false},
		{"root level favicon", "GET", "/favicon.ico", 200, false, nil, false},
		{"svg", "GET", "/logo.svg", 200, false, nil, false},
		{"woff2", "GET", "/fonts/inter.woff2", 200, false, nil, false},
		{"webmanifest", "GET", "/manifest.webmanifest", 200, false, nil, false},
		{"source map", "GET", "/assets/app.js.map", 200, false, nil, false},
		{"uppercase extension", "GET", "/ASSETS/APP.JS", 200, false, nil, false},
		{"extensionless under /assets/", "GET", "/assets/chunk", 200, false, nil, false},
		{"head asset", "HEAD", "/assets/app.js", 200, false, nil, false},
		{"asset missing", "GET", "/assets/gone.js", 404, false, nil, true},
		{"asset server error", "GET", "/assets/app.js", 500, false, nil, true},
		{"asset with rejected auth", "GET", "/assets/app.js", 200, true, nil, true},
		{"post to an asset path is not an asset fetch", "POST", "/assets/app.js", 200, false, nil, true},
		{"extension outside the allowlist", "GET", "/data/export.json", 200, false, nil, true},
		{"asset extension on a non-SPA route", "GET", "/api/files/chart.js", 200, false, nil, true},
		// SPA navigations
		{"navigation root", "GET", "/", 200, false, nil, true},
		{"navigation route", "GET", "/transactions", 200, false, nil, true},
		{"navigation nested", "GET", "/accounts/12/edit", 200, false, nil, true},
		// CORS preflight
		{"preflight ok", "OPTIONS", "/api/things", 204, false, map[string]string{acrm: "POST"}, false},
		{"preflight on catch-all", "OPTIONS", "/anything", 204, false, map[string]string{acrm: "GET"}, false},
		{"preflight rejected", "OPTIONS", "/api/things", 403, false, map[string]string{acrm: "POST"}, true},
		{"options without preflight header", "OPTIONS", "/api/things", 204, false, nil, true},
		// health
		{"health ok", "GET", "/health", 200, false, nil, false},
		{"health head", "HEAD", "/health", 200, false, nil, false},
		{"health failing", "GET", "/health", 503, false, nil, true},
		// polling
		{"session probe ok", "GET", "/api/auth/session", 200, false, nil, false},
		{"session probe unauthenticated", "GET", "/api/auth/session", 401, false, nil, true},
		{"session probe rejected but 200", "GET", "/api/auth/session", 200, true, nil, true},
		{"sync run poll", "GET", "/api/sync-runs/42", 200, false, nil, false},
		{"sync run poll failing", "GET", "/api/sync-runs/42", 500, false, nil, true},
		{"sync run list is not polling", "GET", "/api/sync-runs", 200, false, nil, true},
		// ordinary API traffic
		{"api write", "POST", "/api/things", 201, false, nil, true},
		{"api write failing", "POST", "/api/things", 422, false, nil, true},
	}
	variants := []struct {
		name  string
		chain func(*http.ServeMux) http.Handler
	}{
		{"outer pattern", func(m *http.ServeMux) http.Handler { return m }},
		{"WithRoute", func(m *http.ServeMux) http.Handler { return WithRoute(m, copyingMux{m}) }},
	}
	for _, v := range variants {
		for _, tc := range tests {
			t.Run(v.name+"/"+tc.name, func(t *testing.T) {
				l, s := newTestLogger()
				r := httptest.NewRequest(tc.method, tc.target, nil)
				r.Header.Set("X-Test-Status", strconv.Itoa(tc.status))
				if tc.reject {
					r.Header.Set("X-Test-Reject", "1")
				}
				for k, val := range tc.headers {
					r.Header.Set(k, val)
				}
				serve(Middleware(l)(v.chain(noiseMux())), r)
				n := len(s.lines())
				if tc.logged && n != 1 {
					t.Fatalf("expected a log line, got %d", n)
				}
				if !tc.logged && n != 0 {
					t.Fatalf("expected no log line, got: %s", s.String())
				}
			})
		}
	}
}

// copyingMux hands the mux a copy of the request, as real
// middleware does, so only the WithRoute record can supply the route.
type copyingMux struct{ mux *http.ServeMux }

func (h copyingMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r.Clone(r.Context()))
}
