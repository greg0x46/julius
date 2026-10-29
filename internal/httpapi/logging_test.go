package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/httplog"
	"github.com/greg0x46/julius/internal/settings"
)

const (
	logOwnerEmail = "owner@example.com"
	logSessionKey = "julius_session"
)

// logSink collects the JSON log output.
type logSink struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *logSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *logSink) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func (s *logSink) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf.Reset()
}

func (s *logSink) entries(t *testing.T) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(s.String(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line is not JSON: %v\n%s", err, line)
		}
		out = append(out, m)
	}
	return out
}

// logRig serves the real API behind httplog.Middleware, with a second copy of
// the same server (same database) without logging for comparisons.
type logRig struct {
	conn   *sql.DB
	logged http.Handler
	plain  http.Handler
	sink   *logSink
}

func newLogRig(t *testing.T, cfg auth.Config) *logRig {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "log.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	master := bytes.Repeat([]byte{42}, 32)
	if err := auth.NewStore(conn).Initialize(context.Background(), logOwnerEmail, testPassword, master, nil); err != nil {
		t.Fatal(err)
	}
	if cfg.PublicURL == "" {
		cfg.PublicURL = testOrigin
	}
	frontend := fstest.MapFS{
		"index.html":    {Data: []byte("<html>spa</html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	build := func() http.Handler {
		return httpapi.NewServer(conn, frontend, settings.NewSecrets(master), cfg, nil)
	}
	sink := &logSink{}
	logger := slog.New(slog.NewJSONHandler(sink, nil))
	return &logRig{conn: conn, logged: httplog.Middleware(logger)(build()), plain: build(), sink: sink}
}

func logBrowser(r *http.Request) {
	r.Header.Set("Origin", testOrigin)
	r.Header.Set("X-Julius-Request", "1")
}

func logCookie(token string) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: logSessionKey, Value: token}) }
}

func logHeader(key, value string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set(key, value) }
}

func serveVia(h http.Handler, method, target, body string, mods ...func(*http.Request)) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	for _, mod := range mods {
		mod(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func (g *logRig) do(method, target, body string, mods ...func(*http.Request)) *httptest.ResponseRecorder {
	return serveVia(g.logged, method, target, body, mods...)
}

func (g *logRig) login(password string, mods ...func(*http.Request)) *httptest.ResponseRecorder {
	body := `{"email":"` + logOwnerEmail + `","password":"` + password + `"}`
	return g.do("POST", "/api/auth/login", body, append([]func(*http.Request){logBrowser}, mods...)...)
}

// token opens a session straight through the store, without a request.
func (g *logRig) token(t *testing.T) string {
	t.Helper()
	token, err := auth.NewStore(g.conn).Login(context.Background(), logOwnerEmail, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func (g *logRig) accessLines(t *testing.T) []map[string]any {
	t.Helper()
	return g.byEvent(t, "http_request")
}

func (g *logRig) byEvent(t *testing.T, name string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, e := range g.sink.entries(t) {
		if e["event"] == name {
			out = append(out, e)
		}
	}
	return out
}

// onlyAccess is the single access line of the request that just ran.
func (g *logRig) onlyAccess(t *testing.T) map[string]any {
	t.Helper()
	lines := g.accessLines(t)
	if len(lines) != 1 {
		t.Fatalf("want exactly one http_request line, got %d:\n%s", len(lines), g.sink.String())
	}
	return lines[0]
}

// wantFields checks key/value pairs of a decoded log line; a nil value means
// the key must be absent. Numbers are float64, as decoded from JSON.
func wantFields(t *testing.T, line map[string]any, kv ...any) {
	t.Helper()
	for i := 0; i < len(kv); i += 2 {
		key := kv[i].(string)
		got, present := line[key]
		if kv[i+1] == nil {
			if present {
				t.Errorf("%s = %v, want it absent (line: %v)", key, got, line)
			}
			continue
		}
		if got != kv[i+1] {
			t.Errorf("%s = %v, want %v (line: %v)", key, got, kv[i+1], line)
		}
	}
}

func problemKind(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var p struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("not a problem body: %q", rec.Body.String())
	}
	return strings.TrimPrefix(p.Type, "/problems/")
}

// The access line names the internal route pattern, never the raw path or
// query, even for requests rejected before the handler runs.
func TestAccessLogRouteIsThePatternNotThePath(t *testing.T) {
	g := newLogRig(t, auth.Config{})
	token := g.token(t)
	const secretID, secretQuery = "SECRET-ID-123", "SECRET-QUERY"

	for _, c := range []struct {
		name, method, target string
		mods                 []func(*http.Request)
		status               float64
		route                string
	}{
		{"handler with a path parameter", "GET", "/api/accounts/" + secretID + "?token=" + secretQuery, []func(*http.Request){logCookie(token)}, 404, "GET /api/accounts/{id}"},
		{"rejected by the gate", "GET", "/api/accounts/" + secretID + "?token=" + secretQuery, nil, 401, "GET /api/accounts/{id}"},
		{"rejected by the origin check", "PUT", "/api/transactions/" + secretID + "/category?token=" + secretQuery, nil, 403, "PUT /api/transactions/{id}/category"},
		{"unknown API path", "GET", "/api/" + secretID + "?token=" + secretQuery, []func(*http.Request){logCookie(token)}, 404, "/"},
	} {
		t.Run(c.name, func(t *testing.T) {
			g.sink.Reset()
			g.do(c.method, c.target, "", c.mods...)
			line := g.onlyAccess(t)
			wantFields(t, line, "route", c.route, "status", c.status)
			if out := g.sink.String(); strings.Contains(out, secretID) || strings.Contains(out, secretQuery) {
				t.Fatalf("log leaks the path or query:\n%s", out)
			}
		})
	}
}

func TestAccessLogErrorCodeComesFromWriteProblem(t *testing.T) {
	g := newLogRig(t, auth.Config{})
	token := g.token(t)

	big := strings.Repeat("a", 1<<20+1)
	for _, c := range []struct {
		name, method, target, body string
		mods                       []func(*http.Request)
		status                     float64
		code                       string
	}{
		{"too large", "POST", "/api/categories", big, []func(*http.Request){logBrowser, logCookie(token)}, 413, "body-too-large"},
		{"unauthenticated", "GET", "/api/categories", "", nil, 401, "unauthorized"},
		{"invalid origin", "POST", "/api/categories", `{}`, []func(*http.Request){logCookie(token)}, 403, "invalid-origin"},
		{"handler problem", "POST", "/api/auth/login", `{"email":1}`, []func(*http.Request){logBrowser}, 422, "invalid-login"},
		{"unknown path", "GET", "/api/nothing-here", "", []func(*http.Request){logCookie(token)}, 404, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			g.sink.Reset()
			rec := g.do(c.method, c.target, c.body, c.mods...)
			line := g.onlyAccess(t)
			if c.code == "" {
				wantFields(t, line, "status", c.status, "error_code", nil)
				return
			}
			wantFields(t, line, "status", c.status, "error_code", c.code)
			// The body is the one the client always got.
			if got := problemKind(t, rec); got != c.code {
				t.Fatalf("problem type = %q, want %q", got, c.code)
			}
		})
	}
}

func TestAccessLogSkipsRoutineNoise(t *testing.T) {
	g := newLogRig(t, auth.Config{})
	token := g.token(t)

	for _, c := range []struct {
		name, target string
		mods         []func(*http.Request)
		status       int
	}{
		{"health", "/health", nil, 200},
		{"static asset", "/assets/app.js", nil, 200},
		{"session probe without a cookie", "/api/auth/session", nil, 200},
		{"session probe with an expired cookie", "/api/auth/session", []func(*http.Request){logCookie(strings.Repeat("A", 43))}, 200},
		{"session probe with a valid cookie", "/api/auth/session", []func(*http.Request){logCookie(token)}, 200},
	} {
		t.Run(c.name, func(t *testing.T) {
			g.sink.Reset()
			if rec := g.do("GET", c.target, "", c.mods...); rec.Code != c.status {
				t.Fatalf("status = %d, want %d", rec.Code, c.status)
			}
			if out := g.sink.String(); out != "" {
				t.Fatalf("routine request was logged:\n%s", out)
			}
		})
	}

	t.Run("SPA navigation is logged once", func(t *testing.T) {
		g.sink.Reset()
		g.do("GET", "/transactions", "")
		wantFields(t, g.onlyAccess(t), "route", "/", "status", float64(200), "auth_type", "none", "auth_result", "not_required")
	})
	t.Run("a failing asset is logged", func(t *testing.T) {
		g.sink.Reset()
		g.do("GET", "/assets/missing.js", "")
		wantFields(t, g.onlyAccess(t), "route", "/", "status", float64(404))
	})
	t.Run("a rejected poll is logged", func(t *testing.T) {
		g.sink.Reset()
		g.do("GET", "/api/sync-runs/some-run", "")
		wantFields(t, g.onlyAccess(t), "route", "GET /api/sync-runs/{id}", "status", float64(401), "auth_result", "rejected")
	})
}
