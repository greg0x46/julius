package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/greg0x46/julius/internal/auth"
)

// Whatever the request carries, none of it reaches the log: credentials,
// session tokens, headers, query strings, path parameters, origins.
func TestLogNeverContainsRequestSecrets(t *testing.T) {
	g := newLogRig(t, auth.Config{})

	leaky := func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer SECRET-BEARER")
		r.AddCookie(&http.Cookie{Name: "other", Value: "SECRET-COOKIE"})
	}
	g.login("wrong-password-xyz", leaky)
	g.do("POST", "/api/auth/login?token=SECRET-QUERY", `{"email":"SECRET-EMAIL@example.com","password":"SECRET-PASSWORD"}`, logBrowser, leaky)

	login := g.login(testPassword, leaky)
	token := login.Result().Cookies()[0].Value

	g.do("GET", "/api/accounts/SECRET-PATH-ID?token=SECRET-QUERY&password=SECRET-PASSWORD", "", logCookie(token), leaky)
	g.do("GET", "/api/accounts/SECRET-PATH-ID?token=SECRET-QUERY", "", leaky)
	g.do("GET", "/api/auth/session?token=SECRET-QUERY", "", logCookie(strings.Repeat("S", 43)), leaky)
	g.do("POST", "/api/categories", `{"name":"SECRET-BODY"}`, logCookie(token), leaky,
		logHeader("Origin", "https://SECRET-ORIGIN.example"), logHeader("X-Julius-Request", "1"))
	g.do("POST", "/api/auth/logout", "", logBrowser, logCookie(token), leaky)
	g.do("GET", "/api/categories", "", logCookie(token), leaky) // the session is gone

	out := g.sink.String()
	if !strings.Contains(out, `"event":"http_request"`) || !strings.Contains(out, `"event":"auth.login.success"`) {
		t.Fatalf("scenario did not log anything useful:\n%s", out)
	}
	for _, secret := range []string{
		logOwnerEmail, "SECRET-EMAIL", "SECRET-PASSWORD", testPassword, "wrong-password-xyz",
		token, "SECRET-BEARER", "SECRET-COOKIE", "SECRET-QUERY", "SECRET-PATH-ID", "SECRET-BODY", "SECRET-ORIGIN",
		strings.Repeat("S", 43), "Bearer", "Authorization",
	} {
		if strings.Contains(out, secret) {
			t.Errorf("log contains %q", secret)
		}
	}
	if t.Failed() {
		t.Logf("output:\n%s", out)
	}
}

// The middleware only observes: a sample of routes answers byte for byte as
// the same server without it.
func TestLoggingDoesNotChangeResponses(t *testing.T) {
	g := newLogRig(t, auth.Config{})
	token := g.token(t)
	big := strings.Repeat("a", 1<<20+1)

	for _, c := range []struct {
		name, method, target, body string
		mods                       []func(*http.Request)
	}{
		{"health", "GET", "/health", "", nil},
		{"health head", "HEAD", "/health", "", nil},
		{"spa navigation", "GET", "/transactions", "", nil},
		{"asset", "GET", "/assets/app.js", "", nil},
		{"missing asset", "GET", "/assets/missing.js", "", nil},
		{"protected without a cookie", "GET", "/api/categories", "", nil},
		{"protected with a cookie", "GET", "/api/categories", "", []func(*http.Request){logCookie(token)}},
		{"path parameter", "GET", "/api/accounts/missing", "", []func(*http.Request){logCookie(token)}},
		{"unknown API path", "GET", "/api/nothing-here", "", []func(*http.Request){logCookie(token)}},
		{"probe without a cookie", "GET", "/api/auth/session", "", nil},
		{"probe with a cookie", "GET", "/api/auth/session", "", []func(*http.Request){logCookie(token)}},
		{"probe with a stale cookie", "GET", "/api/auth/session", "", []func(*http.Request){logCookie(strings.Repeat("A", 43))}},
		{"login rejected", "POST", "/api/auth/login", `{"email":"x@example.com","password":"nope"}`, []func(*http.Request){logBrowser}},
		{"login malformed", "POST", "/api/auth/login", `{"email":1}`, []func(*http.Request){logBrowser}},
		{"invalid origin", "POST", "/api/categories", `{}`, []func(*http.Request){logCookie(token)}},
		{"missing request header", "POST", "/api/categories", `{}`, []func(*http.Request){logHeader("Origin", testOrigin), logCookie(token)}},
		{"body too large", "POST", "/api/categories", big, []func(*http.Request){logBrowser, logCookie(token)}},
		{"handler validation", "POST", "/api/categories", `{"name":""}`, []func(*http.Request){logBrowser, logCookie(token)}},
	} {
		t.Run(c.name, func(t *testing.T) {
			logged := serveVia(g.logged, c.method, c.target, c.body, c.mods...)
			plain := serveVia(g.plain, c.method, c.target, c.body, c.mods...)
			sameResponse(t, logged, plain)
		})
	}

	// A login succeeds on both; only the freshly minted token differs.
	logged := g.login(testPassword)
	plain := serveVia(g.plain, "POST", "/api/auth/login", `{"email":"`+logOwnerEmail+`","password":"`+testPassword+`"}`, logBrowser)
	if logged.Code != 200 || plain.Code != 200 || logged.Body.String() != plain.Body.String() {
		t.Fatalf("login: logged %d %q, plain %d %q", logged.Code, logged.Body.String(), plain.Code, plain.Body.String())
	}
	a, b := logged.Result().Cookies()[0], plain.Result().Cookies()[0]
	if a.Name != b.Name || a.Path != b.Path || a.Domain != b.Domain || a.HttpOnly != b.HttpOnly ||
		a.Secure != b.Secure || a.SameSite != b.SameSite || a.MaxAge != b.MaxAge {
		t.Fatalf("session cookie attributes differ: %+v vs %+v", a, b)
	}
}

func sameResponse(t *testing.T, logged, plain *httptest.ResponseRecorder) {
	t.Helper()
	if logged.Code != plain.Code {
		t.Errorf("status %d, plain %d", logged.Code, plain.Code)
	}
	if !reflect.DeepEqual(logged.Header(), plain.Header()) {
		t.Errorf("headers differ:\nlogged %v\nplain  %v", logged.Header(), plain.Header())
	}
	if logged.Body.String() != plain.Body.String() {
		t.Errorf("body differs:\nlogged %q\nplain  %q", logged.Body.String(), plain.Body.String())
	}
}
