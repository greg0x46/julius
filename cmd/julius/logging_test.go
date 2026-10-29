package main

// End-to-end checks of the access log on the assembled server: the real
// httpapi and MCP handlers behind loggedHandler, served over real TCP
// connections, with a temporary SQLite database and synthetic data only.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
	"unicode"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/mcp"
	"github.com/greg0x46/julius/internal/settings"
)

const (
	lgOrigin   = "https://julius.example"
	lgEmail    = "owner-sentinel-q7x@leak.test"
	lgPassword = "owner-pw-sentinel-m4k"
	lgResource = lgOrigin + "/mcp"
	lgSession  = "__Host-julius_session"
	lgUA       = "lg-test-agent/1"
	lgForgedID = "client-supplied-request-id-sentinel"
	lgRedirect = "http://127.0.0.1:45678/callback"
	lgInitRPC  = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","clientInfo":{"name":"generic","version":"1"},"capabilities":{}}}`
	lgListRPC  = `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
)

var lgMaster = bytes.Repeat([]byte{42}, 32)

// logSink collects the JSON log output; drain returns what is new.
type logSink struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	mark int
}

func (s *logSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *logSink) all() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func (s *logSink) drain() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.buf.String()[s.mark:]
	s.mark = s.buf.Len()
	return out
}

// stack is one assembled server. served counts requests whose logging
// finished, so a test can read the log right after the client has the answer.
type stack struct {
	t      *testing.T
	url    string
	sink   *logSink
	client *http.Client
	conn   *sql.DB
	cfg    auth.Config
	served atomic.Int64
	issued atomic.Int64
	ids    map[string]bool
}

func lgFrontend() fs.FS {
	return fstest.MapFS{
		"index.html":    {Data: []byte("<html>synthetic spa</html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
}

func lgHandlers(t *testing.T, conn *sql.DB, cfg auth.Config) (api, mcpHandler http.Handler) {
	t.Helper()
	mcpHandler, err := mcp.NewHandler(true, cfg, auth.NewStore(conn))
	if err != nil {
		t.Fatal(err)
	}
	return httpapi.NewServer(conn, lgFrontend(), settings.NewSecrets(lgMaster), cfg, nil), mcpHandler
}

func lgClient(t *testing.T) *http.Client {
	t.Helper()
	transport := &http.Transport{}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{
		Transport:     transport,
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// startLogged serves loggedHandler(api, mcpHandler) on a loopback listener.
func startLogged(t *testing.T, api, mcpHandler http.Handler) *stack {
	t.Helper()
	s := &stack{t: t, sink: &logSink{}, ids: map[string]bool{}}
	logged := loggedHandler(slog.New(slog.NewJSONHandler(s.sink, nil)), api, mcpHandler)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logged.ServeHTTP(w, r)
		s.served.Add(1)
	}))
	t.Cleanup(ts.Close)
	s.url = ts.URL
	s.client = lgClient(t)
	return s
}

// newStack is the production wiring on a fresh database with one owner.
func newStack(t *testing.T) *stack {
	t.Helper()
	conn := dbtest.OpenSQLite(t)
	if err := auth.NewStore(conn).Initialize(context.Background(), lgEmail, lgPassword, lgMaster, nil); err != nil {
		t.Fatal(err)
	}
	cfg := auth.Config{PublicURL: lgOrigin}
	api, mcpHandler := lgHandlers(t, conn, cfg)
	s := startLogged(t, api, mcpHandler)
	s.conn, s.cfg = conn, cfg
	return s
}

// startPlain serves the same routes without the middleware.
func startPlain(t *testing.T, conn *sql.DB, cfg auth.Config) *httptest.Server {
	t.Helper()
	api, mcpHandler := lgHandlers(t, conn, cfg)
	ts := httptest.NewServer(serverHandler(api, mcpHandler))
	t.Cleanup(ts.Close)
	return ts
}

func (s *stack) settle() {
	s.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for s.served.Load() < s.issued.Load() {
		if time.Now().After(deadline) {
			s.t.Fatalf("%d request(s) were not finished and logged", s.issued.Load()-s.served.Load())
		}
		time.Sleep(time.Millisecond)
	}
}

type reply struct {
	status  int
	header  http.Header
	body    string
	cookies []*http.Cookie
}

type mod func(*http.Request)

func header(key, value string) mod { return func(r *http.Request) { r.Header.Set(key, value) } }
func addHeader(key, value string) mod {
	return func(r *http.Request) { r.Header.Add(key, value) }
}
func cookie(name, value string) mod {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: name, Value: value}) }
}
func browser(r *http.Request) {
	r.Header.Set("Origin", lgOrigin)
	r.Header.Set("X-Julius-Request", "1")
}
func asJSON(r *http.Request) { r.Header.Set("Content-Type", "application/json") }
func asForm(r *http.Request) { r.Header.Set("Content-Type", "application/x-www-form-urlencoded") }
func bearer(token string) mod {
	return func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
	}
}

// send performs one request. Every request carries a forged X-Request-Id; no
// response may carry a request id header.
func send(t *testing.T, client *http.Client, base, method, target, body string, mods ...mod) reply {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, base+target, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("User-Agent", lgUA)
	req.Header.Set("X-Request-Id", lgForgedID)
	for _, m := range mods {
		m(req)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s %s: reading the body: %v", method, target, err)
	}
	for name := range resp.Header {
		if lower := strings.ToLower(name); strings.Contains(lower, "request-id") || strings.Contains(lower, "requestid") || strings.Contains(lower, "correlation") {
			t.Fatalf("%s %s: response carries a request id header %q", method, target, name)
		}
	}
	return reply{status: resp.StatusCode, header: resp.Header, body: string(raw), cookies: resp.Cookies()}
}

func (s *stack) do(method, target, body string, mods ...mod) reply {
	s.t.Helper()
	s.issued.Add(1)
	r := send(s.t, s.client, s.url, method, target, body, mods...)
	s.settle()
	return r
}

func (s *stack) login(email, password string, mods ...mod) reply {
	s.t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	return s.do("POST", "/api/auth/login", string(body), append([]mod{browser, asJSON}, mods...)...)
}

// sessionToken opens a session straight through the store, without a request.
func (s *stack) sessionToken() string {
	s.t.Helper()
	token, err := auth.NewStore(s.conn).Login(context.Background(), lgEmail, lgPassword)
	if err != nil {
		s.t.Fatal(err)
	}
	return token
}

func str(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func setOf(keys ...string) map[string]bool {
	out := map[string]bool{}
	for _, k := range keys {
		out[k] = true
	}
	return out
}

var (
	lgAccessKeys = setOf("time", "level", "msg", "event", "request_id", "method", "route", "status", "duration_ms", "response_bytes",
		"peer_ip", "forwarded_ip", "user_agent", "auth_type", "auth_result", "client_name", "client_id_prefix",
		"error_code", "error_reason", "invalid_field", "invalid_values")
	lgEventKeys   = setOf("time", "level", "msg", "event", "request_id", "reason", "limiter", "scope", "client_name", "client_id_prefix")
	lgEventLevels = map[string]string{
		"auth.login.success": "INFO", "auth.login.failure": "WARN", "auth.logout": "INFO",
		"oauth.client.registered": "INFO", "oauth.consent.granted": "INFO", "oauth.consent.denied": "INFO",
		"oauth.credentials.rejected": "WARN", "oauth.token.revoked": "INFO",
		"security.rate_limited": "WARN", "security.invalid_origin": "WARN",
	}
	lgRequestID = regexp.MustCompile(`^[0-9a-f]{32}$`)
	lgRoute     = regexp.MustCompile(`^((GET|POST|PUT|PATCH|DELETE) )?/[A-Za-z0-9._/{}-]*$`)
	lgToken     = regexp.MustCompile(`^[a-z0-9_.:-]{1,64}$`)
)

func checkAccessShape(t *testing.T, m map[string]any) {
	t.Helper()
	for k := range m {
		if !lgAccessKeys[k] {
			t.Errorf("unexpected field %q in %v", k, m)
		}
	}
	for _, k := range []string{"event", "request_id", "method", "route", "status", "duration_ms", "response_bytes", "peer_ip", "auth_type", "auth_result"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing field %q in %v", k, m)
		}
	}
	status, _ := m["status"].(float64)
	level := "INFO"
	switch {
	case status >= 500:
		level = "ERROR"
	case status >= 400:
		level = "WARN"
	}
	if m["msg"] != "http_request" || m["level"] != level {
		t.Errorf("msg/level = %v/%v, want http_request/%s: %v", m["msg"], m["level"], level, m)
	}
	if !lgRequestID.MatchString(str(m, "request_id")) {
		t.Errorf("request_id %q is not 32 hex characters", m["request_id"])
	}
	if route := str(m, "route"); !lgRoute.MatchString(route) {
		t.Errorf("route %q is not a static pattern", route)
	}
	if d, _ := m["duration_ms"].(float64); d < 0 {
		t.Errorf("duration_ms = %v", m["duration_ms"])
	}
	if n, _ := m["response_bytes"].(float64); n < 0 {
		t.Errorf("response_bytes = %v", m["response_bytes"])
	}
	if _, err := netip.ParseAddr(str(m, "peer_ip")); err != nil {
		t.Errorf("peer_ip %q: %v", m["peer_ip"], err)
	}
	for _, k := range []string{"error_code", "error_reason", "invalid_field"} {
		if v, ok := m[k]; ok && (!lgToken.MatchString(v.(string)) || v == "invalid_value") {
			t.Errorf("%s = %q is not a controlled value", k, v)
		}
	}
	if values, ok := m["invalid_values"]; ok {
		for _, v := range values.([]any) {
			if !lgToken.MatchString(v.(string)) || v == "invalid_value" {
				t.Errorf("invalid_values holds %q", v)
			}
		}
	}
}

func checkEventShape(t *testing.T, m map[string]any) {
	t.Helper()
	level, ok := lgEventLevels[str(m, "event")]
	if !ok {
		t.Errorf("event %q is not in the closed list: %v", m["event"], m)
		return
	}
	for k := range m {
		if !lgEventKeys[k] {
			t.Errorf("unexpected attribute %q in event %v", k, m)
		}
	}
	if m["msg"] != m["event"] || m["level"] != level {
		t.Errorf("msg/level = %v/%v, want %v/%s", m["msg"], m["level"], m["event"], level)
	}
}

// capture is what the log gained since the last take.
type capture struct {
	raw    string
	access []map[string]any
	events []map[string]any
}

// take drains the log, requiring valid JSON lines of the documented shape,
// request ids never seen before, and events that belong to an access line of
// the same capture.
func (s *stack) take() capture {
	t := s.t
	t.Helper()
	raw := s.sink.drain()
	c := capture{raw: raw}
	if raw == "" {
		return c
	}
	if !strings.HasSuffix(raw, "\n") {
		t.Fatalf("log does not end with a newline: %q", raw)
	}
	ids := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSuffix(raw, "\n"), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("log line is not JSON: %v\n%q", err, line)
		}
		if m["event"] == "http_request" {
			checkAccessShape(t, m)
			id := str(m, "request_id")
			if ids[id] || s.ids[id] {
				t.Fatalf("request id %s was used by two requests", id)
			}
			ids[id], s.ids[id] = true, true
			c.access = append(c.access, m)
		} else {
			checkEventShape(t, m)
			c.events = append(c.events, m)
		}
	}
	for _, e := range c.events {
		if !ids[str(e, "request_id")] {
			t.Fatalf("event %v shares no request_id with an access line:\n%s", e["event"], raw)
		}
	}
	return c
}

// logged is the single request that just ran.
type logged struct {
	capture
	line map[string]any
}

func (s *stack) one() logged {
	s.t.Helper()
	c := s.take()
	if len(c.access) != 1 {
		s.t.Fatalf("want exactly one http_request line, got %d:\n%s", len(c.access), c.raw)
	}
	return logged{c, c.access[0]}
}

func (s *stack) none() {
	s.t.Helper()
	if out := s.sink.drain(); out != "" {
		s.t.Fatalf("expected no log output, got:\n%s", out)
	}
}

func (l logged) eventNames() []string {
	var names []string
	for _, e := range l.events {
		names = append(names, str(e, "event"))
	}
	return names
}

func (l logged) event(t *testing.T, name string) map[string]any {
	t.Helper()
	for _, e := range l.events {
		if e["event"] == name {
			if e["request_id"] != l.line["request_id"] {
				t.Fatalf("event %s has another request_id", name)
			}
			return e
		}
	}
	t.Fatalf("missing event %s; got %v\n%s", name, l.eventNames(), l.raw)
	return nil
}

func (l logged) noEvents(t *testing.T) {
	t.Helper()
	if len(l.events) != 0 {
		t.Fatalf("unexpected events %v\n%s", l.eventNames(), l.raw)
	}
}

// has checks key/value pairs of a decoded line; a nil value means absent.
func has(t *testing.T, m map[string]any, kv ...any) {
	t.Helper()
	for i := 0; i < len(kv); i += 2 {
		key, want := kv[i].(string), kv[i+1]
		if n, ok := want.(int); ok {
			want = float64(n)
		}
		got, present := m[key]
		switch {
		case want == nil && present:
			t.Errorf("%s = %v, want it absent (line: %v)", key, got, m)
		case want != nil && got != want:
			t.Errorf("%s = %v, want %v (line: %v)", key, got, want, m)
		}
	}
}

func absent(t *testing.T, raw string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if s == "" {
			t.Fatal("empty sentinel")
		}
		if strings.Contains(raw, s) {
			t.Fatalf("%q reached the log:\n%s", s, raw)
		}
	}
}

// --- OAuth helpers -------------------------------------------------------

type oauthApp struct {
	s        *stack
	name     string
	redirect string
	state    string
	verifier string
	id       string
	pending  string
	csrf     *http.Cookie
	extra    []mod
}

func newOAuthApp(s *stack, name, redirect string) *oauthApp {
	return &oauthApp{s: s, name: name, redirect: redirect, state: "opaque-state", verifier: strings.Repeat("v", 43)}
}

func (a *oauthApp) challenge() string {
	sum := sha256.Sum256([]byte(a.verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (a *oauthApp) register() reply {
	a.s.t.Helper()
	body, _ := json.Marshal(map[string]any{"client_name": a.name, "redirect_uris": []string{a.redirect}, "token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code"}, "response_types": []string{"code"}})
	r := a.s.do("POST", "/oauth/register", string(body), append([]mod{asJSON}, a.extra...)...)
	var created struct {
		ID string `json:"client_id"`
	}
	if r.status != 201 || json.Unmarshal([]byte(r.body), &created) != nil || created.ID == "" {
		a.s.t.Fatalf("register: %d %s", r.status, r.body)
	}
	a.id = created.ID
	return r
}

func (a *oauthApp) authorizeQuery() url.Values {
	return url.Values{"client_id": {a.id}, "redirect_uri": {a.redirect}, "response_type": {"code"}, "resource": {lgResource}, "scope": {"mcp:access"}, "code_challenge": {a.challenge()}, "code_challenge_method": {"S256"}, "state": {a.state}}
}

func (a *oauthApp) begin(extraQuery string) reply {
	a.s.t.Helper()
	r := a.s.do("GET", "/oauth/authorize?"+a.authorizeQuery().Encode()+extraQuery, "", a.extra...)
	found := regexp.MustCompile(`name="request" value="([^"]+)"`).FindStringSubmatch(r.body)
	if r.status != 200 || len(found) != 2 || len(r.cookies) != 1 {
		a.s.t.Fatalf("authorize: %d %s", r.status, r.body)
	}
	a.pending, a.csrf = found[1], r.cookies[0]
	return r
}

func (a *oauthApp) consent(decision, email, password string, mods ...mod) reply {
	a.s.t.Helper()
	form := url.Values{"request": {a.pending}, "decision": {decision}, "email": {email}, "password": {password}}
	all := append([]mod{asForm, header("Origin", lgOrigin), cookie(a.csrf.Name, a.csrf.Value)}, a.extra...)
	return a.s.do("POST", "/oauth/authorize", form.Encode(), append(all, mods...)...)
}

// callbackCode extracts the authorization code from the consent callback page.
func callbackCode(t *testing.T, r reply) string {
	t.Helper()
	link := regexp.MustCompile(`<a href="([^"]+)">`).FindStringSubmatch(r.body)
	if r.status != 200 || len(link) != 2 {
		t.Fatalf("consent callback: %d %s", r.status, r.body)
	}
	u, err := url.Parse(html.UnescapeString(link[1]))
	if err != nil || u.Query().Get("code") == "" {
		t.Fatalf("callback without a code: %s", link[1])
	}
	return u.Query().Get("code")
}

func (a *oauthApp) tokenForm(code string) url.Values {
	return url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {a.id}, "redirect_uri": {a.redirect}, "resource": {lgResource}, "code_verifier": {a.verifier}}
}

func (a *oauthApp) token(code string, mods ...mod) (string, reply) {
	a.s.t.Helper()
	all := append([]mod{asForm}, a.extra...)
	r := a.s.do("POST", "/oauth/token", a.tokenForm(code).Encode(), append(all, mods...)...)
	var out struct {
		Token string `json:"access_token"`
	}
	if r.status == 200 && json.Unmarshal([]byte(r.body), &out) != nil {
		a.s.t.Fatalf("token body: %s", r.body)
	}
	return out.Token, r
}

// accessToken runs the whole authorization on s and returns the bearer.
func (a *oauthApp) accessToken() string {
	a.s.t.Helper()
	a.register()
	a.begin("")
	token, _ := a.token(callbackCode(a.s.t, a.consent("allow", lgEmail, lgPassword)))
	if token == "" {
		a.s.t.Fatal("no access token")
	}
	return token
}

// ---------------------------------------------------------------------------

// Every area of the server produces exactly one access line with a static
// route pattern, and routine noise produces none.
func TestEveryRouteAreaIsLoggedOnceWithAStaticRoute(t *testing.T) {
	s := newStack(t)
	session := cookie(lgSession, s.sessionToken())
	const idSentinel, querySentinel = "path-id-sentinel-81c", "query-sentinel-5d2"

	tests := []struct {
		name, method, target, body string
		mods                       []mod
		route                      string
		status                     int
		authType, authResult       string
		errorCode                  string
		events                     []string
		unlogged                   bool
	}{
		{name: "authenticated API", method: "GET", target: "/api/categories", mods: []mod{session}, route: "GET /api/categories", status: 200, authType: "session", authResult: "ok"},
		{name: "API path parameter and query", method: "GET", target: "/api/accounts/" + idSentinel + "?token=" + querySentinel, mods: []mod{session}, route: "GET /api/accounts/{id}", status: 404, authType: "session", authResult: "ok", errorCode: "account-not-found"},
		{name: "rejected API", method: "GET", target: "/api/categories?token=" + querySentinel, route: "GET /api/categories", status: 401, authType: "none", authResult: "rejected", errorCode: "unauthorized"},
		{name: "API write without the browser headers", method: "PUT", target: "/api/transactions/" + idSentinel + "/category", mods: []mod{session}, route: "PUT /api/transactions/{id}/category", status: 403, authType: "none", authResult: "not_required", errorCode: "invalid-origin", events: []string{"security.invalid_origin"}},
		{name: "unknown API path", method: "GET", target: "/api/unknown-" + idSentinel, mods: []mod{session}, route: "/", status: 404, authType: "session", authResult: "ok"},
		{name: "MCP without a bearer", method: "POST", target: "/mcp", body: "{}", route: "/mcp", status: 401, authType: "none", authResult: "rejected", errorCode: "unauthorized"},
		{name: "reserved MCP subpath", method: "GET", target: "/mcp/" + idSentinel, route: "/mcp/", status: 404, authType: "none", authResult: "not_required"},
		{name: "OAuth registration", method: "POST", target: "/oauth/register?x=" + querySentinel, body: "{}", mods: []mod{asJSON}, route: "POST /oauth/register", status: 400, authType: "none", authResult: "not_required", errorCode: "invalid_client_metadata"},
		{name: "OAuth wrong method", method: "GET", target: "/oauth/register", route: "/oauth/", status: 405, authType: "none", authResult: "not_required"},
		{name: "OAuth unknown path", method: "GET", target: "/oauth/" + idSentinel, route: "/oauth/", status: 404, authType: "none", authResult: "not_required"},
		{name: "authorization server metadata", method: "GET", target: "/.well-known/oauth-authorization-server", route: "GET /.well-known/oauth-authorization-server", status: 200, authType: "none", authResult: "not_required"},
		{name: "protected resource metadata", method: "GET", target: "/.well-known/oauth-protected-resource", route: "GET /.well-known/oauth-protected-resource", status: 200, authType: "none", authResult: "not_required"},
		{name: "protected resource metadata for /mcp", method: "GET", target: "/.well-known/oauth-protected-resource/mcp", route: "GET /.well-known/oauth-protected-resource/mcp", status: 200, authType: "none", authResult: "not_required"},
		{name: "frontend root", method: "GET", target: "/", route: "/", status: 200, authType: "none", authResult: "not_required"},
		{name: "frontend navigation", method: "GET", target: "/transactions", route: "/", status: 200, authType: "none", authResult: "not_required"},
		{name: "frontend deep navigation", method: "GET", target: "/transactions/" + idSentinel + "?q=" + querySentinel, route: "/", status: 200, authType: "none", authResult: "not_required"},
		{name: "missing asset", method: "GET", target: "/missing.js", route: "/", status: 404, authType: "none", authResult: "not_required"},
		{name: "unknown method", method: "SENTINELMETHOD", target: "/api/categories", mods: []mod{session}, route: "/", status: 403, authType: "none", authResult: "not_required", errorCode: "invalid-origin", events: []string{"security.invalid_origin"}},
		{name: "asset served", method: "GET", target: "/assets/app.js", unlogged: true},
		{name: "health", method: "GET", target: "/health", unlogged: true},
		{name: "session probe", method: "GET", target: "/api/auth/session", mods: []mod{session}, unlogged: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := s.do(tc.method, tc.target, tc.body, tc.mods...)
			if tc.unlogged {
				if r.status != 200 {
					t.Fatalf("status %d", r.status)
				}
				s.none()
				return
			}
			if r.status != tc.status {
				t.Fatalf("status %d, want %d: %s", r.status, tc.status, r.body)
			}
			l := s.one()
			has(t, l.line, "route", tc.route, "status", tc.status, "auth_type", tc.authType, "auth_result", tc.authResult, "peer_ip", "127.0.0.1", "user_agent", lgUA)
			if tc.errorCode == "" {
				has(t, l.line, "error_code", nil)
			} else {
				has(t, l.line, "error_code", tc.errorCode)
			}
			if tc.method == "SENTINELMETHOD" {
				has(t, l.line, "method", "OTHER")
			} else {
				has(t, l.line, "method", tc.method)
			}
			if !slices.Equal(l.eventNames(), tc.events) {
				t.Errorf("events = %v, want %v", l.eventNames(), tc.events)
			}
		})
	}
	absent(t, s.sink.all(), idSentinel, querySentinel, "SENTINELMETHOD", lgForgedID)
}

func TestHealthFailureIsLogged(t *testing.T) {
	s := newStack(t)
	if r := s.do("GET", "/health", ""); r.status != 200 {
		t.Fatalf("health: %d", r.status)
	}
	s.none()
	if err := s.conn.Close(); err != nil {
		t.Fatal(err)
	}
	r := s.do("GET", "/health", "")
	if r.status != 503 || !strings.Contains(r.body, "unavailable") {
		t.Fatalf("health with a closed database: %d %s", r.status, r.body)
	}
	l := s.one()
	has(t, l.line, "route", "GET /health", "status", 503, "level", "ERROR", "auth_type", "none", "auth_result", "not_required")
}

func TestDisabledMCPStillLogsItsReservedRoutes(t *testing.T) {
	disabled, err := mcp.NewHandler(false, auth.Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := startLogged(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), disabled)
	for target, route := range map[string]string{"/mcp": "/mcp", "/oauth/token": "/oauth/", "/.well-known/oauth-authorization-server": "/.well-known/oauth-authorization-server"} {
		if r := s.do("POST", target, "{}"); r.status != 404 {
			t.Fatalf("%s: %d", target, r.status)
		}
		has(t, s.one().line, "route", route, "status", 404)
	}
}

// ---------------------------------------------------------------------------

// POST /oauth/register answers every invalid registration with the same
// public error; the access line says which validation failed.
func TestRegisterRejectionsAreDiagnosedFromTheLog(t *testing.T) {
	s := newStack(t)
	const validRedirect = `"redirect_uris":["https://client.example/cb"]`
	elevenURIs := `["` + strings.Repeat(`https://client.example/cb","`, 10) + `https://client.example/cb"]`

	tests := []struct {
		name, body, public string
		code, reason       string
		field              string
		values             []string
		secret             string // client text that must not reach the log
	}{
		{name: "invalid JSON", body: `not json bad-json-sentinel`, reason: "invalid_json", secret: "bad-json-sentinel"},
		{name: "two JSON values", body: `{` + validRedirect + `} {"second-value-sentinel":1}`, reason: "multiple_json_values", secret: "second-value-sentinel"},
		{name: "client_id provided", body: `{` + validRedirect + `,"client_id":"chosen-id-sentinel"}`, reason: "client_id_provided", field: "client_id", secret: "chosen-id-sentinel"},
		{name: "name over 200 bytes", body: `{` + validRedirect + `,"client_name":"` + strings.Repeat("n", 201) + `"}`, reason: "name_too_long", field: "client_name", secret: strings.Repeat("n", 60)},
		{name: "no redirect_uris", body: `{"client_name":"No redirects"}`, reason: "redirect_uris_missing", field: "redirect_uris"},
		{name: "more than 10 redirect_uris", body: `{"redirect_uris":` + elevenURIs + `}`, reason: "too_many_redirect_uris", field: "redirect_uris", secret: "client.example"},
		{name: "client_secret_post", body: `{` + validRedirect + `,"token_endpoint_auth_method":"client_secret_post"}`, reason: "unsupported_auth_method", field: "token_endpoint_auth_method", values: []string{"client_secret_post"}},
		{name: "implicit grant", body: `{` + validRedirect + `,"grant_types":["authorization_code","implicit"]}`, reason: "unsupported_grant_types", field: "grant_types", values: []string{"authorization_code", "implicit"}},
		{name: "id_token response type", body: `{` + validRedirect + `,"response_types":["code","id_token"]}`, reason: "unsupported_response_types", field: "response_types", values: []string{"code", "id_token"}},
		{name: "http redirect to a public host", body: `{"redirect_uris":["http://example.com/cb"]}`, public: "invalid_redirect_uri", code: "invalid_redirect_uri", reason: "http_non_loopback", field: "redirect_uris", secret: "example.com"},
		{name: "redirect with userinfo", body: `{"redirect_uris":["https://user@example.com/cb"]}`, public: "invalid_redirect_uri", code: "invalid_redirect_uri", reason: "userinfo", field: "redirect_uris", secret: "user@example.com"},
	}
	seen := map[string]string{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			public := orDefault(tc.public, "invalid_client_metadata")
			r := s.do("POST", "/oauth/register", tc.body, asJSON)
			if r.status != 400 || strings.TrimSpace(r.body) != fmt.Sprintf(`{"error":%q}`, public) {
				t.Fatalf("public response changed: %d %s", r.status, r.body)
			}
			l := s.one()
			l.noEvents(t)
			has(t, l.line, "route", "POST /oauth/register", "status", 400, "auth_type", "none", "auth_result", "not_required",
				"error_code", orDefault(tc.code, "invalid_client_metadata"), "error_reason", tc.reason, "client_name", nil, "client_id_prefix", nil)
			if tc.field == "" {
				has(t, l.line, "invalid_field", nil)
			} else {
				has(t, l.line, "invalid_field", tc.field)
			}
			var values []string
			if raw, ok := l.line["invalid_values"].([]any); ok {
				for _, v := range raw {
					values = append(values, v.(string))
				}
			}
			if !slices.Equal(values, tc.values) {
				t.Errorf("invalid_values = %v, want %v", values, tc.values)
			}
			diagnosis := fmt.Sprintf("%s/%s/%s/%v", l.line["error_code"], tc.reason, tc.field, values)
			if previous, dup := seen[diagnosis]; dup {
				t.Errorf("%s and %s are indistinguishable in the log: %s", previous, tc.name, diagnosis)
			}
			seen[diagnosis] = tc.name
			if tc.secret != "" {
				absent(t, l.raw, tc.secret)
			}
		})
	}
	if strings.Contains(s.sink.all(), "oauth.client.registered") {
		t.Fatal("a rejected registration emitted oauth.client.registered")
	}
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// From the log alone: which validation failed, and that the registration
// event exists only for the request that really created a client.
func TestRegisterDiagnosisAndSuccessEventShareTheRequestID(t *testing.T) {
	s := newStack(t)

	s.do("POST", "/oauth/register", `{"redirect_uris":["https://client.example/cb"],"token_endpoint_auth_method":"client_secret_basic"}`, asJSON)
	failed := s.one()
	failed.noEvents(t)
	diagnosis := fmt.Sprintf("%s: %s on %s %v", failed.line["error_code"], failed.line["error_reason"], failed.line["invalid_field"], failed.line["invalid_values"])
	if diagnosis != "invalid_client_metadata: unsupported_auth_method on token_endpoint_auth_method [client_secret_basic]" {
		t.Fatalf("diagnosis from the log = %q", diagnosis)
	}

	r := s.do("POST", "/oauth/register", `{"client_name":"Synthetic assistant","redirect_uris":["https://client.example/cb"]}`, asJSON)
	var created struct {
		ID string `json:"client_id"`
	}
	if r.status != 201 || json.Unmarshal([]byte(r.body), &created) != nil {
		t.Fatalf("register: %d %s", r.status, r.body)
	}
	ok := s.one()
	has(t, ok.line, "status", 201, "error_code", nil, "error_reason", nil, "invalid_field", nil, "invalid_values", nil)
	event := ok.event(t, "oauth.client.registered")
	has(t, event, "client_name", "Synthetic assistant", "client_id_prefix", created.ID[:8], "request_id", ok.line["request_id"])
	if event["request_id"] == failed.line["request_id"] {
		t.Fatal("two requests share a request_id")
	}
	absent(t, ok.raw, created.ID)
}

// ---------------------------------------------------------------------------

// The whole authorization flow plus the REST login: each event shares its
// request_id with exactly one access line, ids differ between requests and
// the client's X-Request-Id is ignored.
func TestEventsCorrelateWithTheirAccessLine(t *testing.T) {
	s := newStack(t)
	app := newOAuthApp(s, "Synthetic assistant", lgRedirect)

	app.register()
	l := s.one()
	has(t, l.line, "route", "POST /oauth/register", "status", 201, "client_name", nil)
	prefix := app.id[:8]
	has(t, l.event(t, "oauth.client.registered"), "client_name", "Synthetic assistant", "client_id_prefix", prefix)

	app.begin("")
	l = s.one()
	has(t, l.line, "route", "GET /oauth/authorize", "status", 200, "client_name", "Synthetic assistant", "client_id_prefix", prefix)
	l.noEvents(t)

	r := app.consent("allow", lgEmail, "wrong-password-xyz")
	if r.status != 401 {
		t.Fatalf("wrong password: %d", r.status)
	}
	l = s.one()
	has(t, l.line, "route", "POST /oauth/authorize", "status", 401, "auth_type", "none", "auth_result", "rejected", "error_code", "invalid_credentials", "error_reason", "invalid_credentials")
	has(t, l.event(t, "oauth.credentials.rejected"), "level", "WARN", "reason", "invalid_credentials", "client_name", "Synthetic assistant", "client_id_prefix", prefix)

	code := callbackCode(t, app.consent("allow", lgEmail, lgPassword))
	l = s.one()
	has(t, l.line, "route", "POST /oauth/authorize", "status", 200, "error_code", nil, "client_name", "Synthetic assistant")
	has(t, l.event(t, "oauth.consent.granted"), "level", "INFO", "scope", "mcp:access", "client_name", "Synthetic assistant", "client_id_prefix", prefix)
	if len(l.events) != 1 {
		t.Fatalf("one event expected, got %v", l.eventNames())
	}

	token, _ := app.token(code)
	l = s.one()
	has(t, l.line, "route", "POST /oauth/token", "status", 200, "client_id_prefix", prefix, "client_name", nil)
	l.noEvents(t)

	for _, rpc := range []string{lgInitRPC, lgListRPC} {
		if r := s.do("POST", "/mcp", rpc, bearer(token), header("Origin", lgOrigin)); r.status != 200 {
			t.Fatalf("/mcp: %d %s", r.status, r.body)
		}
		l = s.one()
		has(t, l.line, "route", "/mcp", "status", 200, "auth_type", "oauth", "auth_result", "ok", "client_name", "Synthetic assistant", "client_id_prefix", prefix, "error_code", nil)
		l.noEvents(t)
	}

	form := url.Values{"client_id": {app.id}, "token": {token}}.Encode()
	if r := s.do("POST", "/oauth/revoke", form, asForm); r.status != 200 {
		t.Fatalf("revoke: %d", r.status)
	}
	l = s.one()
	has(t, l.line, "route", "POST /oauth/revoke", "status", 200)
	has(t, l.event(t, "oauth.token.revoked"), "client_name", "Synthetic assistant", "client_id_prefix", prefix)

	if r := s.do("POST", "/mcp", lgListRPC, bearer(token)); r.status != 401 || r.header.Get("WWW-Authenticate") == "" {
		t.Fatalf("revoked token: %d", r.status)
	}
	l = s.one()
	has(t, l.line, "status", 401, "auth_type", "oauth", "auth_result", "rejected", "error_code", "unauthorized", "error_reason", "invalid_token", "client_name", nil)

	// REST login and logout.
	if r := s.login(lgEmail, "wrong-password-xyz"); r.status != 401 {
		t.Fatalf("login failure: %d", r.status)
	}
	l = s.one()
	has(t, l.line, "route", "POST /api/auth/login", "status", 401, "auth_type", "none", "auth_result", "rejected", "error_code", "unauthorized", "error_reason", "invalid_credentials")
	has(t, l.event(t, "auth.login.failure"), "level", "WARN", "reason", "invalid_credentials")

	login := s.login(lgEmail, lgPassword)
	if login.status != 200 || len(login.cookies) != 1 {
		t.Fatalf("login: %d %s", login.status, login.body)
	}
	l = s.one()
	has(t, l.line, "route", "POST /api/auth/login", "status", 200, "error_code", nil)
	has(t, l.event(t, "auth.login.success"), "level", "INFO")
	if len(l.events) != 1 {
		t.Fatalf("login produced %v", l.eventNames())
	}

	if r := s.do("POST", "/api/auth/logout", "", browser, cookie(login.cookies[0].Name, login.cookies[0].Value)); r.status != 200 {
		t.Fatalf("logout: %d", r.status)
	}
	l = s.one()
	has(t, l.line, "route", "POST /api/auth/logout", "status", 200, "auth_type", "session", "auth_result", "ok")
	l.event(t, "auth.logout")

	if strings.Contains(s.sink.all(), lgForgedID) {
		t.Fatal("the client's X-Request-Id reached the log")
	}
	if len(s.ids) != 12 {
		t.Fatalf("expected 12 requests with 12 distinct ids, saw %d", len(s.ids))
	}
}

// ---------------------------------------------------------------------------

// peer_ip is the TCP peer; forwarded headers are unverified diagnostics that
// neither the rate limits nor any response depend on.
func TestPeerIPIsTheConnectionAndForwardedHeadersAreOnlyDiagnostic(t *testing.T) {
	s := newStack(t)
	var local string
	trace := func(r *http.Request) {
		ctx := httptrace.WithClientTrace(r.Context(), &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
			local, _, _ = net.SplitHostPort(info.Conn.LocalAddr().String())
		}})
		*r = *r.WithContext(ctx)
	}
	xff, cf := "X-Forwarded-For", "CF-Connecting-IP"

	tests := []struct {
		name string
		mods []mod
		want string // forwarded_ip, "" for absent
	}{
		{"no forwarding headers", nil, ""},
		{"forged pair, Cloudflare header wins", []mod{header(xff, "203.0.113.9"), header(cf, "198.51.100.7")}, "198.51.100.7"},
		{"first X-Forwarded-For element", []mod{header(xff, "203.0.113.9, 10.0.0.1")}, "203.0.113.9"},
		{"IPv6", []mod{header(xff, "2001:db8::7")}, "2001:db8::7"},
		{"invalid X-Forwarded-For", []mod{header(xff, "not-an-ip")}, ""},
		{"invalid CF-Connecting-IP does not fall back", []mod{header(cf, "bogus"), header(xff, "203.0.113.9")}, ""},
		{"repeated CF-Connecting-IP is dropped", []mod{addHeader(cf, "198.51.100.1"), addHeader(cf, "198.51.100.2"), header(xff, "203.0.113.9")}, ""},
		{"scoped address", []mod{header(xff, "fe80::1%eth0")}, ""},
		{"unspecified address", []mod{header(xff, "0.0.0.0")}, ""},
	}
	var baseline reply
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := s.do("GET", "/api/categories", "", append([]mod{trace}, tc.mods...)...)
			l := s.one()
			if local == "" {
				t.Fatal("no connection information")
			}
			has(t, l.line, "peer_ip", local, "status", 401, "user_agent", lgUA)
			if tc.want == "" {
				has(t, l.line, "forwarded_ip", nil)
			} else {
				has(t, l.line, "forwarded_ip", tc.want)
			}
			if i == 0 {
				baseline = r
			} else if r.status != baseline.status || r.body != baseline.body {
				t.Errorf("forwarded headers changed the response: %d %s", r.status, r.body)
			}
		})
	}

	t.Run("forged addresses do not influence the login rate limit", func(t *testing.T) {
		const target = "ratelimit-sentinel@leak.test"
		for i := 1; i <= 5; i++ {
			r := s.login(target, "wrong-password-xyz", header(xff, fmt.Sprintf("203.0.113.%d", i)), header(cf, fmt.Sprintf("198.51.100.%d", i)))
			if r.status != 401 {
				t.Fatalf("attempt %d: %d", i, r.status)
			}
			s.one()
		}
		r := s.login(target, "wrong-password-xyz", header(xff, "203.0.113.200"), header(cf, "198.51.100.200"))
		if r.status != 429 || r.header.Get("Retry-After") != "60" {
			t.Fatalf("sixth attempt from a fresh forged address: %d", r.status)
		}
		l := s.one()
		has(t, l.line, "status", 429, "peer_ip", local, "forwarded_ip", "198.51.100.200", "error_code", "too-many-attempts", "error_reason", "window_exceeded")
		has(t, l.event(t, "security.rate_limited"), "limiter", "auth_attempts", "reason", "window_exceeded")

		// The limit is per account, not per address: another account is not
		// blocked, even from the address that was.
		if r := s.login("other-sentinel@leak.test", "wrong-password-xyz", header(xff, "203.0.113.200")); r.status != 401 {
			t.Fatalf("another account from the blocked address: %d", r.status)
		}
		s.one()
		if r := s.login(lgEmail, lgPassword, header(xff, "203.0.113.200"), header(cf, "198.51.100.200")); r.status != 200 {
			t.Fatalf("owner login with the blocked forged address: %d", r.status)
		}
		s.one()
	})
}

// ---------------------------------------------------------------------------

// A whole session of sensitive and hostile input; none of it may reach the log.
func TestNothingSensitiveReachesTheLogAcrossTheServer(t *testing.T) {
	s := newStack(t)
	const (
		evilName   = "Evil\r\n{\"event\":\"fake\"}\u2028\x1b[31m\u0085\u202e client"
		redirect   = "https://redirect-sentinel.test/cb/path-sentinel?q=redirect-query-sentinel"
		state      = "state-sentinel-5a6f"
		wrongPW    = "wrong-pw-sentinel-4b2e"
		secretKey  = "client-secret-sentinel-8e1"
		queryMark  = "request-query-sentinel-2c9"
		bodyMark   = "register-body-sentinel-6d3"
		originMark = "origin-sentinel.test"
		hostileUA  = "ua-sentinel\u2028{\"event\":\"fake\"}\u0085\t\u202e end"
	)
	app := newOAuthApp(s, evilName, redirect)
	app.state = state
	app.verifier = "verifier-sentinel-" + strings.Repeat("x", 30)

	body, _ := json.Marshal(map[string]any{"client_name": evilName, "redirect_uris": []string{redirect}, "software_id": bodyMark})
	reg := s.do("POST", "/oauth/register", string(body), asJSON, header("User-Agent", hostileUA))
	var created struct {
		ID string `json:"client_id"`
	}
	if reg.status != 201 || json.Unmarshal([]byte(reg.body), &created) != nil {
		t.Fatalf("register: %d %s", reg.status, reg.body)
	}
	app.id = created.ID
	app.begin("&q=" + queryMark)
	csrf := app.csrf.Value
	app.consent("allow", "someone-sentinel@leak.test", wrongPW)
	app.consent("allow", lgEmail, wrongPW, header("Origin", "https://"+originMark))
	real := app.csrf
	app.csrf = &http.Cookie{Name: real.Name, Value: "forged-csrf-sentinel"}
	app.consent("allow", lgEmail, wrongPW)
	app.csrf = real
	code := callbackCode(t, app.consent("allow", lgEmail, lgPassword))

	// Credentials the token endpoint refuses outright.
	form := app.tokenForm(code)
	form.Set("client_secret", secretKey)
	s.do("POST", "/oauth/token", form.Encode(), asForm)
	s.do("POST", "/oauth/token", app.tokenForm(code).Encode(), asForm, header("Authorization", "Basic YmFzaWMtc2VudGluZWw6eA=="))
	app.begin("")
	code2 := callbackCode(t, app.consent("allow", lgEmail, lgPassword))
	token, _ := app.token(code2)

	rpc := func(mods ...mod) { s.do("POST", "/mcp", lgListRPC, mods...) }
	rpc(bearer(token))
	rpc(bearer("bad-bearer-sentinel"))
	rpc(bearer(token), header("Origin", "https://"+originMark))
	rpc(header("Authorization", "Token header-sentinel-xyz"))
	s.do("POST", "/oauth/revoke", url.Values{"client_id": {app.id}, "token": {token}, "client_secret": {secretKey}}.Encode(), asForm)
	s.do("POST", "/oauth/revoke", url.Values{"client_id": {app.id}, "token": {token}}.Encode(), asForm)

	// REST: bad and good login, a request carrying a query, a spoofed origin.
	s.login("someone-sentinel@leak.test", wrongPW)
	login := s.login(lgEmail, lgPassword)
	session := cookie(login.cookies[0].Name, login.cookies[0].Value)
	s.do("GET", "/api/categories?token="+queryMark, "", session)
	s.do("POST", "/api/categories", `{"name":"body-sentinel-77"}`, session, asJSON, header("Origin", "https://"+originMark), header("X-Julius-Request", "1"))
	s.do("GET", "/api/%0d%0a{%22event%22:%22pathfake%22}", "", session)
	s.do("POST", "/api/auth/logout", "", browser, session)
	s.do("GET", "/api/categories", "", session)

	// A user agent that breaks lines with an obsolete header fold.
	raw := rawExchange(t, s, "GET /api/categories HTTP/1.1\r\nHost: julius.test\r\nUser-Agent: fold-sentinel\r\n {\"event\":\"fake\"}\r\nConnection: close\r\n\r\n")
	if !strings.HasPrefix(raw, "HTTP/1.1 401") {
		t.Fatalf("folded header request: %q", raw)
	}

	c := s.take()
	secrets := []string{
		lgEmail, "owner-sentinel", "leak.test", lgPassword, wrongPW, "someone-sentinel",
		token, code, code2, state, app.verifier, "verifier-sentinel", app.challenge(), app.id,
		csrf, "forged-csrf-sentinel", login.cookies[0].Value, "bad-bearer-sentinel", "header-sentinel", "Bearer", "Basic", "YmFzaWMtc2VudGluZWw6eA==",
		secretKey, "redirect-sentinel", "path-sentinel", "redirect-query-sentinel", queryMark, bodyMark, "body-sentinel-77", "pathfake",
		originMark, lgForgedID, "julius.test",
	}
	absent(t, c.raw, secrets...)

	// Line breaks in the user agent never split a record.
	uaOf := func(prefix string) string {
		for _, m := range c.access {
			if ua := str(m, "user_agent"); strings.HasPrefix(ua, prefix) {
				return ua
			}
		}
		t.Fatalf("no access line with a user agent starting %q", prefix)
		return ""
	}
	if got := uaOf("ua-sentinel"); got != `ua-sentinel {"event":"fake"} end` {
		t.Errorf("user_agent = %q", got)
	}
	if got := uaOf("fold-sentinel"); got != `fold-sentinel {"event":"fake"}` {
		t.Errorf("folded user_agent = %q", got)
	}
	for _, escaped := range []string{`\n`, `\r`, `\t`, `\u001b`, `\u2028`, `\u0085`, `\u202e`} {
		if strings.Contains(c.raw, escaped) {
			t.Errorf("an escaped control character %s reached the log", escaped)
		}
	}
	if len(c.access) < 20 {
		t.Fatalf("expected a line per request, got %d", len(c.access))
	}
	if n := strings.Count(c.raw, "\n"); n != len(c.access)+len(c.events) {
		t.Fatalf("%d newlines for %d records: a record spans several lines", n, len(c.access)+len(c.events))
	}
	seenEvents := map[string]bool{}
	for _, e := range c.events {
		seenEvents[str(e, "event")] = true
	}
	for _, want := range []string{"oauth.client.registered", "oauth.credentials.rejected", "oauth.consent.granted", "oauth.token.revoked", "security.invalid_origin", "auth.login.failure", "auth.login.success", "auth.logout"} {
		if !seenEvents[want] {
			t.Errorf("scenario never produced %s", want)
		}
	}
	for _, m := range append(slices.Clone(c.access), c.events...) {
		if m["event"] == "fake" {
			t.Fatalf("forged record: %v", m)
		}
		for _, key := range []string{"client_name", "user_agent"} {
			value, ok := m[key].(string)
			if !ok {
				continue
			}
			if len(value) > 200 {
				t.Errorf("%s is not truncated: %q", key, value)
			}
			for _, r := range value {
				if unicode.IsControl(r) || (unicode.IsSpace(r) && r != ' ') || unicode.Is(unicode.Cf, r) {
					t.Errorf("%s keeps %U: %q", key, r, value)
				}
			}
		}
		if name, ok := m["client_name"].(string); ok && !strings.HasPrefix(name, "Evil {") {
			t.Errorf("client_name = %q", name)
		}
	}
}

// rawExchange writes a request verbatim and returns the start of the response.
func rawExchange(t *testing.T, s *stack, request string) string {
	t.Helper()
	conn, err := net.Dial("tcp", strings.TrimPrefix(s.url, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	s.issued.Add(1)
	if _, err := io.WriteString(conn, request); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	s.settle()
	return resp.Proto + " " + resp.Status
}

// ---------------------------------------------------------------------------

type snapshot struct {
	status int
	header http.Header
	body   string
}

func snap(r reply) snapshot {
	h := r.header.Clone()
	h.Del("Date")
	return snapshot{r.status, h, r.body}
}

// The middleware only observes: a sample of routes answers byte for byte as
// the same server without it.
func TestLoggingDoesNotChangeResponses(t *testing.T) {
	s := newStack(t)
	plain := startPlain(t, s.conn, s.cfg)
	plainClient := lgClient(t)
	token := newOAuthApp(s, "Synthetic assistant", lgRedirect).accessToken()
	app := newOAuthApp(s, "Redirect client", lgRedirect)
	app.register()
	session := cookie(lgSession, s.sessionToken())
	badResponseType := "/oauth/authorize?" + func() string {
		q := app.authorizeQuery()
		q.Set("response_type", "token")
		return q.Encode()
	}()
	s.take()

	samples := []struct {
		name, method, target, body string
		mods                       []mod
		status                     int
	}{
		{"health", "GET", "/health", "", nil, 200},
		{"frontend root", "GET", "/", "", nil, 200},
		{"frontend navigation", "GET", "/transactions", "", nil, 200},
		{"asset", "GET", "/assets/app.js", "", nil, 200},
		{"missing asset", "GET", "/missing.js", "", nil, 404},
		{"session probe", "GET", "/api/auth/session", "", nil, 200},
		{"authenticated API", "GET", "/api/categories", "", []mod{session}, 200},
		{"rejected API", "GET", "/api/categories", "", nil, 401},
		{"API write without the browser headers", "POST", "/api/categories", `{"name":"x"}`, []mod{session, asJSON}, 403},
		{"MCP without a bearer", "POST", "/mcp", "{}", nil, 401},
		{"MCP with an invalid bearer", "POST", "/mcp", "{}", []mod{bearer("invalid")}, 401},
		{"MCP initialize", "POST", "/mcp", lgInitRPC, []mod{bearer(token)}, 200},
		{"MCP tools/list", "POST", "/mcp", lgListRPC, []mod{bearer(token)}, 200},
		{"MCP forbidden origin", "POST", "/mcp", "{}", []mod{bearer(token), header("Origin", "https://evil.example")}, 403},
		{"reserved MCP subpath", "GET", "/mcp/x", "", nil, 404},
		{"registration", "POST", "/oauth/register", `{"redirect_uris":[]}`, []mod{asJSON}, 400},
		{"authorization redirect", "GET", badResponseType, "", nil, 303},
		{"consent without a pending request", "POST", "/oauth/authorize", "request=nope&decision=allow", []mod{asForm, header("Origin", lgOrigin)}, 400},
		{"token without a code", "POST", "/oauth/token", "grant_type=authorization_code", []mod{asForm}, 400},
		{"wrong method on a route", "GET", "/oauth/register", "", nil, 405},
		{"authorization server metadata", "GET", "/.well-known/oauth-authorization-server", "", nil, 200},
	}
	for _, tc := range samples {
		t.Run(tc.name, func(t *testing.T) {
			with := s.do(tc.method, tc.target, tc.body, tc.mods...)
			without := send(t, plainClient, plain.URL, tc.method, tc.target, tc.body, tc.mods...)
			if with.status != tc.status {
				t.Fatalf("status %d, want %d: %s", with.status, tc.status, with.body)
			}
			if a, b := snap(with), snap(without); a.status != b.status || a.body != b.body || !headersEqual(a.header, b.header) {
				t.Fatalf("response differs without the middleware:\nwith:    %d %v %q\nwithout: %d %v %q", a.status, a.header, truncate(a.body), b.status, b.header, truncate(b.body))
			}
		})
	}
	if r := s.do("POST", "/mcp", "{}"); r.header.Get("WWW-Authenticate") == "" {
		t.Fatal("the 401 lost its challenge")
	}
}

func headersEqual(a, b http.Header) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if !slices.Equal(v, b[k]) {
			return false
		}
	}
	return true
}

func truncate(s string) string {
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}

// A 429 must come out identical too, not only be a 429.
func TestRateLimitedResponseIsIdenticalWithoutTheMiddleware(t *testing.T) {
	s := newStack(t)
	plain := startPlain(t, s.conn, s.cfg)
	plainClient := lgClient(t)
	attempt := func(logged bool) reply {
		body, _ := json.Marshal(map[string]string{"email": "limit-sentinel@leak.test", "password": "wrong-password-xyz"})
		var last reply
		for i := 0; i < 6; i++ {
			if logged {
				last = s.do("POST", "/api/auth/login", string(body), browser, asJSON)
			} else {
				last = send(t, plainClient, plain.URL, "POST", "/api/auth/login", string(body), browser, asJSON)
			}
		}
		return last
	}
	with, without := attempt(true), attempt(false)
	if with.status != 429 {
		t.Fatalf("status %d", with.status)
	}
	a, b := snap(with), snap(without)
	if a.status != b.status || a.body != b.body || !headersEqual(a.header, b.header) {
		t.Fatalf("429 differs: %v %q vs %v %q", a.header, a.body, b.header, b.body)
	}
}

// ---------------------------------------------------------------------------

func newStub(t *testing.T, api http.HandlerFunc) *stack {
	t.Helper()
	disabled, err := mcp.NewHandler(false, auth.Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return startLogged(t, api, disabled)
}

// http.ResponseController reaches the real connection through the middleware,
// as mcp.NewHandler needs for its deadlines.
func TestResponseControllerWorksBehindTheMiddleware(t *testing.T) {
	type probe struct{ setRead, setWrite, flush, read error }
	results := make(chan probe, 1)
	s := newStub(t, func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		switch r.URL.Path {
		case "/probe":
			var p probe
			p.setRead = rc.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
			p.setWrite = rc.SetWriteDeadline(time.Now().Add(5 * time.Second))
			_, p.read = io.ReadAll(r.Body)
			results <- p
			w.WriteHeader(http.StatusRequestTimeout)
		case "/flush":
			io.WriteString(w, "a")
			if err := rc.Flush(); err != nil {
				t.Errorf("Flush: %v", err)
			}
			io.WriteString(w, "b")
		}
	})

	conn, err := net.Dial("tcp", strings.TrimPrefix(s.url, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	s.issued.Add(1)
	// Valid headers; the body is never finished.
	if _, err := io.WriteString(conn, "POST /probe HTTP/1.1\r\nHost: julius.test\r\nContent-Length: 100\r\n\r\n{"); err != nil {
		t.Fatal(err)
	}
	var p probe
	select {
	case p = <-results:
	case <-time.After(5 * time.Second):
		t.Fatal("the handler never gave up on the body")
	}
	if p.setRead != nil || p.setWrite != nil || !errors.Is(p.read, os.ErrDeadlineExceeded) {
		t.Fatalf("deadlines: %+v", p)
	}
	s.settle()
	has(t, s.one().line, "route", "/", "status", 408)

	if r := s.do("GET", "/flush", ""); r.status != 200 || r.body != "ab" {
		t.Fatalf("flush: %d %q", r.status, r.body)
	}
	has(t, s.one().line, "status", 200, "response_bytes", 2)
}

// A client that goes away mid-request leaves one line, no hung handler and
// no goroutine behind.
func TestClientCancellationLeavesOneLineAndNoGoroutines(t *testing.T) {
	entered := make(chan struct{}, 1)
	exited := make(chan struct{}, 1)
	s := newStub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/slow" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		entered <- struct{}{}
		<-r.Context().Done()
		exited <- struct{}{}
	})
	s.do("GET", "/warmup", "")
	s.take()
	before := runtime.NumGoroutine()

	const cancelled = 10
	for i := 0; i < cancelled; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		req, err := http.NewRequestWithContext(ctx, "GET", s.url+"/slow", nil)
		if err != nil {
			t.Fatal(err)
		}
		s.issued.Add(1)
		done := make(chan error, 1)
		go func() {
			resp, err := s.client.Do(req)
			if err == nil {
				resp.Body.Close()
			}
			done <- err
		}()
		wait(t, entered, "the handler never started")
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("client error = %v", err)
		}
		wait(t, exited, "the handler did not see the cancellation")
	}
	s.settle()
	c := s.take()
	if len(c.access) != cancelled || len(c.events) != 0 {
		t.Fatalf("want %d lines, got %d lines and %d events:\n%s", cancelled, len(c.access), len(c.events), c.raw)
	}
	for _, line := range c.access {
		has(t, line, "route", "/", "response_bytes", 0, "error_code", nil)
		if line["status"].(float64) >= 500 {
			t.Errorf("a client cancellation is logged as a server error: %v", line)
		}
	}

	s.client.CloseIdleConnections()
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > before {
		if time.Now().After(deadline) {
			buf := make([]byte, 1<<16)
			t.Fatalf("goroutines: %d before, %d after\n%s", before, runtime.NumGoroutine(), buf[:runtime.Stack(buf, true)])
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func wait(t *testing.T, ch <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal(message)
	}
}

// A client that disconnects while the real registration endpoint reads its
// body still yields exactly one access line.
func TestAbortedRegistrationBodyLogsOnce(t *testing.T) {
	s := newStack(t)
	conn, err := net.Dial("tcp", strings.TrimPrefix(s.url, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	s.issued.Add(1)
	io.WriteString(conn, "POST /oauth/register HTTP/1.1\r\nHost: julius.test\r\nContent-Type: application/json\r\nContent-Length: 200\r\n\r\n{\"client_name\":")
	conn.Close()
	s.settle()
	// The TimeoutHandler answers for the vanished client while the handler may
	// still be finishing, so only the route is stable here.
	l := s.one()
	has(t, l.line, "route", "POST /oauth/register")
	l.noEvents(t)
}

// serve drains an in-flight request, and the request is still logged.
func TestGracefulShutdownLogsTheDrainedRequest(t *testing.T) {
	sink := &logSink{}
	entered, release := make(chan struct{}), make(chan struct{})
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusAccepted)
	})
	disabled, err := mcp.NewHandler(false, auth.Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := &http.Server{Handler: loggedHandler(slog.New(slog.NewJSONHandler(sink, nil)), api, disabled), ReadHeaderTimeout: time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- serve(ctx, server, listener) }()

	status := make(chan int, 1)
	go func() {
		resp, err := (&http.Client{Timeout: 5 * time.Second}).Get("http://" + listener.Addr().String() + "/slow")
		if err != nil {
			status <- -1
			return
		}
		resp.Body.Close()
		status <- resp.StatusCode
	}()
	wait(t, entered, "the request never arrived")
	cancel()
	select {
	case err := <-stopped:
		t.Fatalf("serve returned with a request in flight: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if got := <-status; got != http.StatusAccepted {
		t.Fatalf("drained request answered %d", got)
	}
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not finish")
	}
	var line map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(sink.all())), &line); err != nil {
		t.Fatalf("expected exactly one JSON line: %v\n%s", err, sink.all())
	}
	has(t, line, "event", "http_request", "route", "/", "status", 202)
}
