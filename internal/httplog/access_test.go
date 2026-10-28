package httplog

import (
	"context"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

var hexID = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestAccessLogFields(t *testing.T) {
	baseKeys := []string{"time", "level", "msg", "event", "request_id", "method", "route", "status", "duration_ms", "response_bytes", "peer_ip", "auth_type", "auth_result"}
	tests := []struct {
		name    string
		pattern string
		target  string
		method  string
		handler http.HandlerFunc
		status  int
		level   string
	}{
		{"explicit status and body", "POST /things", "/things", "POST", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, "hello")
		}, 201, "INFO"},
		{"implicit 200", "GET /things/{id}", "/things/7", "GET", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("abc"))
		}, 200, "INFO"},
		{"empty handler", "GET /empty", "/empty", "GET", func(w http.ResponseWriter, r *http.Request) {}, 200, "INFO"},
		{"redirect", "GET /old", "/old", "GET", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/new", http.StatusFound)
		}, 302, "INFO"},
		{"client error", "GET /missing", "/missing", "GET", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}, 404, "WARN"},
		{"server error with body", "PUT /boom", "/boom", "PUT", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusServiceUnavailable)
		}, 503, "ERROR"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc(tc.pattern, tc.handler)
			l, s := newTestLogger()
			rr := serve(Middleware(l)(mux), newReq(tc.method, tc.target))
			lines := s.lines()
			if len(lines) != 1 {
				t.Fatalf("want 1 line, got %q", s.String())
			}
			if got := keysOf(t, lines[0]); !equalStrings(got, baseKeys) {
				t.Fatalf("keys = %v, want %v", got, baseKeys)
			}
			m := decode(t, lines[0])
			want := map[string]any{
				"msg": "http_request", "event": "http_request", "level": tc.level,
				"method": tc.method, "route": tc.pattern, "status": float64(tc.status),
				"response_bytes": float64(rr.Body.Len()), "peer_ip": "192.0.2.1",
				"auth_type": "none", "auth_result": "not_required",
			}
			for k, v := range want {
				if m[k] != v {
					t.Errorf("%s = %v, want %v", k, m[k], v)
				}
			}
			if rr.Code != tc.status {
				t.Errorf("response status = %d, want %d", rr.Code, tc.status)
			}
			if !hexID.MatchString(m["request_id"].(string)) {
				t.Errorf("request_id = %v", m["request_id"])
			}
		})
	}
}

func TestResponseBytesExact(t *testing.T) {
	m, _ := access(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("12345"))
		_, _ = io.WriteString(w, "678")
	}), newReq("GET", "/x"))
	if m["response_bytes"] != float64(8) {
		t.Fatalf("response_bytes = %v", m["response_bytes"])
	}
}

func TestDuration(t *testing.T) {
	m, _ := access(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(20 * time.Millisecond)
	}), newReq("GET", "/slow"))
	d := m["duration_ms"].(float64)
	if d < 20 || d > 5000 {
		t.Fatalf("duration_ms = %v", d)
	}
	if scaled := d * 1000; math.Abs(scaled-math.Round(scaled)) > 1e-6 {
		t.Fatalf("duration_ms = %v has more than 3 decimals", d)
	}
}

func TestDurationRounding(t *testing.T) {
	tests := map[time.Duration]float64{
		0:                                  0,
		1234 * time.Nanosecond:             0.001,
		1500 * time.Nanosecond:             0.002,
		1234567 * time.Nanosecond:          1.235,
		12 * time.Millisecond:              12,
		2*time.Second + 5*time.Microsecond: 2000.005,
	}
	for d, want := range tests {
		if got := durationMS(d); got != want {
			t.Errorf("durationMS(%v) = %v, want %v", d, got, want)
		}
	}
}

func TestMethodNormalization(t *testing.T) {
	tests := map[string]string{
		"GET": "GET", "POST": "POST", "PUT": "PUT", "PATCH": "PATCH", "DELETE": "DELETE",
		"HEAD": "HEAD", "OPTIONS": "OPTIONS", "CONNECT": "CONNECT", "TRACE": "TRACE",
		"BREW": "OTHER", "get": "OTHER", "": "OTHER", "GET\r\nINJECT": "OTHER", "PROPFIND": "OTHER",
	}
	for in, want := range tests {
		if got := knownMethod(in); got != want {
			t.Errorf("knownMethod(%q) = %q, want %q", in, got, want)
		}
	}
	r := newReq("GET", "/x")
	r.Method = "GET\r\nINJECT"
	m, s := access(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), r)
	if m["method"] != "OTHER" || strings.Contains(s.String(), "INJECT") {
		t.Fatalf("method not normalized: %s", s.String())
	}
}

func TestRouteSources(t *testing.T) {
	api := http.NewServeMux()
	api.HandleFunc("GET /api/accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})
	api.HandleFunc("POST /oauth/register", func(w http.ResponseWriter, r *http.Request) {})

	// copy mimics middleware that hands the next handler a new *http.Request.
	copying := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), struct{}{}, 1)))
		})
	}
	rejecting := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		SetAuth(r.Context(), AuthOAuth, AuthRejected)
		w.WriteHeader(http.StatusUnauthorized)
	})
	outer := http.NewServeMux()
	outer.Handle("GET /outer/{x}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	tests := []struct {
		name    string
		handler http.Handler
		method  string
		target  string
		route   string
		status  float64
	}{
		{"WithRoute through a copying handler", WithRoute(api, copying(copying(api))), "GET", "/api/accounts/ACCT-SECRET-1?x=1", "GET /api/accounts/{id}", 200},
		{"WithRoute when next rejects before the mux", WithRoute(api, rejecting), "POST", "/oauth/register", "POST /oauth/register", 401},
		{"outer mux pattern", outer, "GET", "/outer/RAW-SEGMENT", "GET /outer/{x}", 200},
		{"unmatched through WithRoute", WithRoute(api, api), "GET", "/secret/raw/path?token=T", "unmatched", 404},
		{"unmatched without any mux", http.NotFoundHandler(), "GET", "/secret/raw/path", "unmatched", 404},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, s := access(t, tc.handler, newReq(tc.method, tc.target))
			if m["route"] != tc.route || m["status"] != tc.status {
				t.Fatalf("route/status = %v/%v, want %v/%v", m["route"], m["status"], tc.route, tc.status)
			}
			for _, leak := range []string{"ACCT-SECRET-1", "RAW-SEGMENT", "secret/raw", "token=T", "x=1"} {
				if strings.Contains(s.String(), leak) {
					t.Errorf("raw URL part %q leaked into log: %s", leak, s.String())
				}
			}
		})
	}
}

func TestWithRouteDoesNotMutateRequestAndNoopWithoutMiddleware(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /x", func(w http.ResponseWriter, r *http.Request) {})
	var seen *http.Request
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seen = r })

	r := newReq("GET", "/x")
	l, _ := newTestLogger()
	serve(Middleware(l)(WithRoute(mux, next)), r)
	if seen == nil || seen.Pattern != "" {
		t.Fatalf("WithRoute must not set r.Pattern, got %q", seen.Pattern)
	}

	// Without Middleware the request passes through untouched.
	r2 := newReq("GET", "/x")
	seen = nil
	serve(WithRoute(mux, next), r2)
	if seen != r2 {
		t.Fatal("request was replaced")
	}
}

func TestPeerIP(t *testing.T) {
	tests := map[string]string{
		"192.0.2.1:8080":       "192.0.2.1",
		"[2001:db8::1]:443":    "2001:db8::1",
		"[::ffff:192.0.2.9]:1": "192.0.2.9",
		"192.0.2.77":           "192.0.2.77",
		"[fe80::1%eth0]:80":    "fe80::1",
		"":                     "unknown",
		"@":                    "unknown",
		"/run/app.sock":        "unknown",
		"not an address":       "unknown",
	}
	for in, want := range tests {
		if got := peerIP(in); got != want {
			t.Errorf("peerIP(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestForwardedIP(t *testing.T) {
	long := strings.Repeat("1", 65)
	tests := []struct {
		name  string
		lines map[string][]string
		want  string
	}{
		{"cf v4", map[string][]string{"CF-Connecting-IP": {"203.0.113.9"}}, "203.0.113.9"},
		{"cf v6 canonicalized", map[string][]string{"CF-Connecting-IP": {"2001:DB8:0:0::A"}}, "2001:db8::a"},
		{"cf mapped v4", map[string][]string{"CF-Connecting-IP": {"::ffff:203.0.113.9"}}, "203.0.113.9"},
		{"cf with spaces", map[string][]string{"CF-Connecting-IP": {"  203.0.113.9 "}}, "203.0.113.9"},
		{"cf invalid", map[string][]string{"CF-Connecting-IP": {"not-an-ip"}}, ""},
		{"cf empty", map[string][]string{"CF-Connecting-IP": {""}}, ""},
		{"cf with port", map[string][]string{"CF-Connecting-IP": {"203.0.113.9:443"}}, ""},
		{"cf zone", map[string][]string{"CF-Connecting-IP": {"fe80::1%eth0"}}, ""},
		{"cf unspecified v4", map[string][]string{"CF-Connecting-IP": {"0.0.0.0"}}, ""},
		{"cf unspecified v6", map[string][]string{"CF-Connecting-IP": {"::"}}, ""},
		{"cf unspecified mapped", map[string][]string{"CF-Connecting-IP": {"::ffff:0.0.0.0"}}, ""},
		{"cf too long", map[string][]string{"CF-Connecting-IP": {long}}, ""},
		{"cf multiple lines", map[string][]string{"CF-Connecting-IP": {"203.0.113.9", "203.0.113.10"}}, ""},
		{"cf injection attempt", map[string][]string{"CF-Connecting-IP": {`203.0.113.9","event":"x`}}, ""},
		{"cf present but invalid never falls back", map[string][]string{"CF-Connecting-IP": {"garbage"}, "X-Forwarded-For": {"198.51.100.1"}}, ""},
		{"cf multiple lines never falls back", map[string][]string{"CF-Connecting-IP": {"a", "b"}, "X-Forwarded-For": {"198.51.100.1"}}, ""},
		{"cf wins over xff", map[string][]string{"CF-Connecting-IP": {"203.0.113.9"}, "X-Forwarded-For": {"198.51.100.1"}}, "203.0.113.9"},
		{"xff first element", map[string][]string{"X-Forwarded-For": {"198.51.100.1, 203.0.113.9, 192.0.2.1"}}, "198.51.100.1"},
		{"xff first of several lines", map[string][]string{"X-Forwarded-For": {"198.51.100.1", "203.0.113.9"}}, "198.51.100.1"},
		{"xff v6", map[string][]string{"X-Forwarded-For": {"2001:db8::7,198.51.100.1"}}, "2001:db8::7"},
		{"xff first invalid does not use the second", map[string][]string{"X-Forwarded-For": {"bogus, 203.0.113.9"}}, ""},
		{"xff long header but short first element", map[string][]string{"X-Forwarded-For": {"198.51.100.1, " + strings.Repeat("10.0.0.1, ", 20)}}, "198.51.100.1"},
		{"xff first element too long", map[string][]string{"X-Forwarded-For": {long + ", 198.51.100.1"}}, ""},
		{"xff zone", map[string][]string{"X-Forwarded-For": {"fe80::1%eth0"}}, ""},
		{"xff unspecified", map[string][]string{"X-Forwarded-For": {"0.0.0.0, 198.51.100.1"}}, ""},
		{"other headers ignored", map[string][]string{"X-Real-Ip": {"198.51.100.1"}, "Forwarded": {"for=198.51.100.1"}, "True-Client-Ip": {"198.51.100.1"}}, ""},
		{"none", nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			for k, v := range tc.lines {
				for _, one := range v {
					h.Add(k, one)
				}
			}
			if got := forwardedIP(h); got != tc.want {
				t.Fatalf("forwardedIP = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSpoofedHeadersNeverReachPeerIP(t *testing.T) {
	r := newReq("GET", "/x")
	r.RemoteAddr = "192.0.2.50:4000"
	r.Header.Set("X-Forwarded-For", "198.51.100.7")
	r.Header.Set("CF-Connecting-IP", "203.0.113.8")
	r.Header.Set("X-Real-IP", "198.51.100.9")
	m, _ := access(t, http.NotFoundHandler(), r)
	if m["peer_ip"] != "192.0.2.50" || m["forwarded_ip"] != "203.0.113.8" {
		t.Fatalf("peer/forwarded = %v / %v", m["peer_ip"], m["forwarded_ip"])
	}

	r = newReq("GET", "/x")
	r.RemoteAddr = "192.0.2.50:4000"
	r.Header.Set("CF-Connecting-IP", "bad value")
	r.Header.Set("X-Forwarded-For", "198.51.100.7")
	m, _ = access(t, http.NotFoundHandler(), r)
	if _, ok := m["forwarded_ip"]; ok || m["peer_ip"] != "192.0.2.50" {
		t.Fatalf("forwarded_ip should be omitted: %v", m)
	}
}

func TestClean(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"plain", "Mozilla/5.0 (X11; Linux)", 200, "Mozilla/5.0 (X11; Linux)"},
		{"CRLF", "a\r\nb", 200, "a b"},
		{"tab and DEL and NUL", "a\tb\x7fc\x00d", 200, "a b c d"},
		{"invalid utf-8", "a\xffb\xc3", 200, "a b"},
		{"line and paragraph separators", "a\u2028b\u2029c", 200, "a b c"},
		{"NEL and C1", "a\u0085b\u009bc", 200, "a b c"},
		{"bidi and zero width", "a\u202eb\u200bc", 200, "a b c"},
		{"unicode spaces", "a\u00a0\u2003b", 200, "a b"},
		{"collapse and trim", "  lots \t of \n\n space  ", 200, "lots of space"},
		{"quotes survive for the encoder", `x","event":"y`, 200, `x","event":"y`},
		{"empty", "", 10, ""},
		{"only separators", " \r\n\t", 10, ""},
		{"cap is rune safe", strings.Repeat("é", 300), 200, strings.Repeat("é", 200)},
		{"cap counts runes of 4 bytes", strings.Repeat("😀", 10), 3, "😀😀😀"},
		{"cap never leaves a trailing space", "ab cd", 3, "ab"},
		{"cap exact fit", "ab cd", 5, "ab cd"},
		{"cap zero", "abc", 0, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := clean(tc.in, tc.max)
			if got != tc.want {
				t.Fatalf("clean(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
			}
			if !utf8.ValidString(got) {
				t.Fatal("result is not valid UTF-8")
			}
		})
	}
}

func TestUserAgentInLog(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		want string
	}{
		{"CRLF line injection", "Mozilla/5.0\r\n{\"event\":\"fake\"}", `Mozilla/5.0 {"event":"fake"}`},
		{"json breakout", "x\",\"event\":\"x", `x","event":"x`},
		{"invalid utf-8", "bad\xff\xfeagent", "bad agent"},
		{"U+2028", "a\u2028b", "a b"},
		{"capped", strings.Repeat("ü", 500), strings.Repeat("ü", 200)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newReq("GET", "/x")
			r.Header.Set("User-Agent", tc.ua)
			m, s := access(t, http.NotFoundHandler(), r)
			if m["user_agent"] != tc.want {
				t.Fatalf("user_agent = %q, want %q", m["user_agent"], tc.want)
			}
			if m["event"] != "http_request" {
				t.Fatalf("event spoofed: %v", m["event"])
			}
			if n := len(s.lines()); n != 1 {
				t.Fatalf("%d lines", n)
			}
		})
	}
	r := newReq("GET", "/x")
	r.Header.Del("User-Agent")
	m, _ := access(t, http.NotFoundHandler(), r)
	if _, ok := m["user_agent"]; ok {
		t.Fatal("empty user agent should be omitted")
	}
}

func TestRequestID(t *testing.T) {
	l, s := newTestLogger()
	var inHandler []string
	h := Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inHandler = append(inHandler, RequestID(r.Context()))
	}))
	var headers []http.Header
	for i := 0; i < 3; i++ {
		r := newReq("GET", "/x")
		r.Header.Set("X-Request-Id", "client-chosen-id")
		r.Header.Set("X-Correlation-Id", "client-chosen-id")
		headers = append(headers, serve(h, r).Header())
	}
	lines := s.lines()
	if len(lines) != 3 {
		t.Fatalf("lines: %q", s.String())
	}
	seen := map[string]bool{}
	for i, line := range lines {
		id := decode(t, line)["request_id"].(string)
		if !hexID.MatchString(id) || id == "client-chosen-id" || seen[id] {
			t.Fatalf("bad or repeated request_id %q", id)
		}
		seen[id] = true
		if inHandler[i] != id {
			t.Errorf("RequestID(ctx) = %q, log has %q", inHandler[i], id)
		}
		if len(headers[i]) != 0 {
			t.Errorf("middleware added response headers: %v", headers[i])
		}
	}
	if RequestID(context.Background()) != "" {
		t.Fatal("RequestID without middleware must be empty")
	}
}

func TestPanicIsLoggedAndRepanicked(t *testing.T) {
	tests := []struct {
		name     string
		handler  http.HandlerFunc
		wantCode string
		status   float64
	}{
		{"before writing", func(w http.ResponseWriter, r *http.Request) { panic("SECRET-PANIC-VALUE") }, "handler_panic", 500},
		{"after writing", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
			panic("SECRET-PANIC-VALUE")
		}, "handler_panic", 202},
		{"abort", func(w http.ResponseWriter, r *http.Request) { panic(http.ErrAbortHandler) }, "handler_aborted", 500},
		{"explicit code wins", func(w http.ResponseWriter, r *http.Request) {
			SetError(r.Context(), "custom_code", "", "")
			panic("SECRET-PANIC-VALUE")
		}, "custom_code", 500},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l, s := newTestLogger()
			h := Middleware(l)(tc.handler)
			var got any
			func() {
				defer func() { got = recover() }()
				h.ServeHTTP(httptest.NewRecorder(), newReq("GET", "/boom"))
			}()
			if got == nil {
				t.Fatal("panic was swallowed")
			}
			if tc.name == "abort" && got != http.ErrAbortHandler {
				t.Fatalf("re-panicked with %v", got)
			}
			if tc.name != "abort" && got != "SECRET-PANIC-VALUE" {
				t.Fatalf("re-panicked with %v", got)
			}
			lines := s.lines()
			if len(lines) != 1 {
				t.Fatalf("lines: %q", s.String())
			}
			m := decode(t, lines[0])
			if m["level"] != "ERROR" || m["status"] != tc.status || m["error_code"] != tc.wantCode {
				t.Fatalf("unexpected line: %v", m)
			}
			if strings.Contains(s.String(), "SECRET-PANIC-VALUE") {
				t.Fatal("panic value leaked into the log")
			}
		})
	}
}

func TestPanicOnQuietRouteIsStillLogged(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		panic("x")
	})
	l, s := newTestLogger()
	func() {
		defer func() { _ = recover() }()
		Middleware(l)(mux).ServeHTTP(httptest.NewRecorder(), newReq("GET", "/health"))
	}()
	if len(s.lines()) != 1 {
		t.Fatalf("lines: %q", s.String())
	}
}

func TestPanicOverRealServer(t *testing.T) {
	l, s := newTestLogger()
	srv := httptest.NewUnstartedServer(Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("SECRET-PANIC-VALUE")
	})))
	srv.Config.ErrorLog = discardLogger()
	srv.Start()
	defer srv.Close()
	if resp, err := http.Get(srv.URL + "/p"); err == nil {
		resp.Body.Close()
		t.Fatal("expected the connection to be aborted")
	}
	m := decode(t, s.waitLines(t, 1)[0])
	if m["status"] != float64(500) || m["error_code"] != "handler_panic" {
		t.Fatalf("line: %v", m)
	}
}

func TestForbiddenStringsNeverLogged(t *testing.T) {
	secrets := []string{
		"CODESECRET", "STATESECRET", "TOKENSECRET", "BEARERSECRET", "COOKIESECRET",
		"SETCOOKIESECRET", "PASSWORDSECRET", "alice@example.test", "ACCOUNTSEGMENT",
		"origin.example.test", "referer.example.test", "host.example.test", "BODYMARKER",
		"CUSTOMHEADERSECRET",
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "PASSWORDSECRET") {
			t.Error("handler did not receive the body")
		}
		SetAuth(r.Context(), AuthSession, AuthRejected)
		SetError(r.Context(), "invalid_credentials", "bad_password", "password")
		SetOAuthClient(r.Context(), "client-1234567890", "Demo App")
		Event(r.Context(), "auth.login.failure", slog.String("reason", "bad_password"))
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: "SETCOOKIESECRET"})
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
	body := strings.NewReader(`{"email":"alice@example.test","password":"PASSWORDSECRET","note":"BODYMARKER"}`)
	r := httptest.NewRequest("POST", "/api/accounts/ACCOUNTSEGMENT?code=CODESECRET&state=STATESECRET&token=TOKENSECRET", body)
	r.Host = "host.example.test"
	r.Header.Set("Authorization", "Bearer BEARERSECRET")
	r.Header.Set("Cookie", "session=COOKIESECRET")
	r.Header.Set("Origin", "https://origin.example.test")
	r.Header.Set("Referer", "https://referer.example.test/page")
	r.Header.Set("X-Custom", "CUSTOMHEADERSECRET")
	r.Header.Set("User-Agent", "Test/1.0")

	l, s := newTestLogger()
	serve(Middleware(l)(WithRoute(mux, mux)), r)
	lines := s.lines()
	if len(lines) != 2 {
		t.Fatalf("want event + access lines, got %q", s.String())
	}
	out := s.String()
	for _, secret := range secrets {
		if strings.Contains(out, secret) {
			t.Errorf("log contains %q:\n%s", secret, out)
		}
	}
	m := decode(t, lines[1])
	if m["route"] != "POST /api/accounts/{id}" || m["user_agent"] != "Test/1.0" || m["client_id_prefix"] != "client-1" {
		t.Errorf("unexpected access line: %v", m)
	}
}

func TestMiddlewareRejectsNilLogger(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic for a nil logger")
		}
	}()
	Middleware(nil)
}

func TestRequestWithoutURL(t *testing.T) {
	r := newReq("GET", "/x")
	r.URL = nil
	m, _ := access(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), r)
	if m["route"] != "unmatched" {
		t.Fatalf("line: %v", m)
	}
}
