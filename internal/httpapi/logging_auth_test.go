package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/settings"
)

func TestAccessLogAuthClassification(t *testing.T) {
	g := newLogRig(t, auth.Config{})
	token := g.token(t)

	for _, c := range []struct {
		name string
		mods []func(*http.Request)
		want []any
	}{
		{"no cookie", nil,
			[]any{"status", float64(401), "auth_type", "none", "auth_result", "rejected", "error_code", "unauthorized", "error_reason", "missing_session"}},
		{"unknown session", []func(*http.Request){logCookie(strings.Repeat("A", 43))},
			[]any{"status", float64(401), "auth_type", "session", "auth_result", "rejected", "error_code", "unauthorized", "error_reason", "invalid_session"}},
		{"malformed session", []func(*http.Request){logCookie("garbage")},
			[]any{"status", float64(401), "auth_type", "session", "auth_result", "rejected", "error_code", "unauthorized", "error_reason", "invalid_session"}},
		{"valid session", []func(*http.Request){logCookie(token)},
			[]any{"status", float64(200), "auth_type", "session", "auth_result", "ok", "error_code", nil, "error_reason", nil}},
	} {
		t.Run(c.name, func(t *testing.T) {
			g.sink.Reset()
			g.do("GET", "/api/categories", "", c.mods...)
			line := g.onlyAccess(t)
			wantFields(t, line, append(c.want, "route", "GET /api/categories")...)
			if len(g.byEvent(t, "security.invalid_origin")) != 0 {
				t.Fatal("unexpected security event")
			}
		})
	}
}

func TestAccessLogAuthNotRequired(t *testing.T) {
	t.Run("authentication switched off", func(t *testing.T) {
		g := newLogRig(t, auth.Config{})
		if err := settings.SetAuthenticationEnabled(t.Context(), g.conn, false); err != nil {
			t.Fatal(err)
		}
		g.do("GET", "/api/categories", "")
		wantFields(t, g.onlyAccess(t), "status", float64(200), "auth_type", "none", "auth_result", "not_required")
	})
	t.Run("demo", func(t *testing.T) {
		g := newLogRig(t, auth.Config{Demo: true})
		g.do("GET", "/api/categories", "")
		wantFields(t, g.onlyAccess(t), "status", float64(200), "auth_type", "none", "auth_result", "not_required")

		g.sink.Reset()
		g.login(testPassword)
		wantFields(t, g.onlyAccess(t), "status", float64(403), "auth_type", "none", "auth_result", "not_required",
			"error_code", "demo-mode", "error_reason", "demo_blocked")
		if n := len(g.byEvent(t, "auth.login.success")) + len(g.byEvent(t, "auth.login.failure")); n != 0 {
			t.Fatalf("a blocked login produced %d login events", n)
		}
	})
	t.Run("authentication setting unreadable", func(t *testing.T) {
		g := newLogRig(t, auth.Config{})
		if err := settings.Set(t.Context(), g.conn, settings.KeyAuthenticationEnabled, "invalid", false, nil); err != nil {
			t.Fatal(err)
		}
		g.do("GET", "/api/categories", "")
		wantFields(t, g.onlyAccess(t), "status", float64(503), "auth_type", "none", "auth_result", "not_required",
			"error_code", "auth-config-unavailable", "error_reason", "auth_config_unavailable")
	})
	t.Run("login while authentication is off", func(t *testing.T) {
		g := newLogRig(t, auth.Config{})
		if err := settings.SetAuthenticationEnabled(t.Context(), g.conn, false); err != nil {
			t.Fatal(err)
		}
		g.login(testPassword)
		wantFields(t, g.onlyAccess(t), "status", float64(409), "auth_type", "none", "auth_result", "not_required", "error_code", "auth-disabled")
		if len(g.byEvent(t, "auth.login.success")) != 0 {
			t.Fatal("login event without a login")
		}
	})
}

// The frontend probes /api/auth/session every minute: a missing or expired
// session is the expected answer, never a rejection. 2xx probes are not
// logged, so "not rejected" is what is observable (a rejection would force a
// line); a 5xx is logged like any other.
func TestSessionProbeIsNeverARejection(t *testing.T) {
	g := newLogRig(t, auth.Config{})
	token := g.token(t)
	for name, mods := range map[string][]func(*http.Request){
		"no cookie":      nil,
		"expired cookie": {logCookie(strings.Repeat("A", 43))},
		"valid cookie":   {logCookie(token)},
	} {
		g.sink.Reset()
		if rec := g.do("GET", "/api/auth/session", "", mods...); rec.Code != 200 {
			t.Fatalf("%s: status %d", name, rec.Code)
		}
		if out := g.sink.String(); out != "" {
			t.Fatalf("%s: probe was logged:\n%s", name, out)
		}
	}

	// Without the sessions table the store fails: infrastructure, not
	// authentication, so the probe reports 503 with default auth fields while
	// the gate denies access on a protected route.
	if _, err := g.conn.Exec(`DROP TABLE auth_sessions`); err != nil {
		t.Fatal(err)
	}
	g.sink.Reset()
	g.do("GET", "/api/auth/session", "", logCookie(token))
	wantFields(t, g.onlyAccess(t), "status", float64(503), "auth_type", "none", "auth_result", "not_required",
		"error_code", "auth-unavailable", "error_reason", "auth_unavailable")

	g.sink.Reset()
	g.do("GET", "/api/categories", "", logCookie(token))
	wantFields(t, g.onlyAccess(t), "status", float64(503), "auth_type", "session", "auth_result", "rejected",
		"error_code", "auth-unavailable", "error_reason", "auth_unavailable")
}

func TestLoginEventsShareTheRequestID(t *testing.T) {
	g := newLogRig(t, auth.Config{})
	noLeak := func(t *testing.T, secrets ...string) {
		t.Helper()
		for _, secret := range append(secrets, logOwnerEmail, "wrong-password-xyz", testPassword) {
			if strings.Contains(g.sink.String(), secret) {
				t.Fatalf("log contains %q:\n%s", secret, g.sink.String())
			}
		}
	}

	t.Run("failure", func(t *testing.T) {
		g.sink.Reset()
		g.login("wrong-password-xyz")
		line := g.onlyAccess(t)
		wantFields(t, line, "route", "POST /api/auth/login", "status", float64(401), "auth_type", "none", "auth_result", "rejected",
			"error_code", "unauthorized", "error_reason", "invalid_credentials")
		events := g.byEvent(t, "auth.login.failure")
		if len(events) != 1 {
			t.Fatalf("login.failure events = %d\n%s", len(events), g.sink.String())
		}
		wantFields(t, events[0], "level", "WARN", "request_id", line["request_id"], "reason", "invalid_credentials")
		if len(events[0]) != 6 { // time, level, msg, event, request_id, reason
			t.Fatalf("unexpected fields: %v", events[0])
		}
		noLeak(t)
	})

	var token string
	t.Run("success", func(t *testing.T) {
		g.sink.Reset()
		rec := g.login(testPassword)
		token = rec.Result().Cookies()[0].Value
		line := g.onlyAccess(t)
		wantFields(t, line, "status", float64(200), "auth_type", "none", "auth_result", "not_required", "error_code", nil)
		events := g.byEvent(t, "auth.login.success")
		if len(events) != 1 {
			t.Fatalf("login.success events = %d\n%s", len(events), g.sink.String())
		}
		wantFields(t, events[0], "level", "INFO", "request_id", line["request_id"])
		if len(events[0]) != 5 { // time, level, msg, event, request_id
			t.Fatalf("unexpected fields: %v", events[0])
		}
		if len(g.byEvent(t, "auth.login.failure")) != 0 {
			t.Fatal("successful login also logged a failure")
		}
		noLeak(t, token)
	})

	t.Run("logout", func(t *testing.T) {
		g.sink.Reset()
		g.do("POST", "/api/auth/logout", "", logBrowser, logCookie(token))
		line := g.onlyAccess(t)
		wantFields(t, line, "route", "POST /api/auth/logout", "status", float64(200), "auth_type", "session", "auth_result", "ok")
		events := g.byEvent(t, "auth.logout")
		if len(events) != 1 {
			t.Fatalf("logout events = %d\n%s", len(events), g.sink.String())
		}
		wantFields(t, events[0], "level", "INFO", "request_id", line["request_id"])
		noLeak(t, token)
	})

	t.Run("logout that fails is not logged as done", func(t *testing.T) {
		token := g.token(t)
		if _, err := g.conn.Exec(`CREATE TRIGGER block_logout BEFORE DELETE ON auth_sessions BEGIN SELECT RAISE(ABORT, 'blocked'); END`); err != nil {
			t.Fatal(err)
		}
		g.sink.Reset()
		g.do("POST", "/api/auth/logout", "", logBrowser, logCookie(token))
		wantFields(t, g.onlyAccess(t), "status", float64(503), "auth_type", "session", "auth_result", "ok",
			"error_code", "auth-unavailable", "error_reason", "auth_unavailable")
		if len(g.byEvent(t, "auth.logout")) != 0 {
			t.Fatal("auth.logout emitted although the session was not deleted")
		}
		noLeak(t, token)
	})
}

// A store outage is not "credentials rejected": no login.failure event, and
// the cause is reported as unavailability.
func TestLoginOutageIsNotAFailedLogin(t *testing.T) {
	g := newLogRig(t, auth.Config{})
	if _, err := g.conn.Exec(`DROP TABLE auth_sessions`); err != nil {
		t.Fatal(err)
	}
	rec := g.login(testPassword)
	if rec.Code != 503 {
		t.Fatalf("status = %d", rec.Code)
	}
	wantFields(t, g.onlyAccess(t), "status", float64(503), "auth_type", "none", "auth_result", "not_required",
		"error_code", "auth-unavailable", "error_reason", "auth_unavailable")
	for _, name := range []string{"auth.login.failure", "auth.login.success", "security.rate_limited"} {
		if n := len(g.byEvent(t, name)); n != 0 {
			t.Fatalf("%s emitted %d times", name, n)
		}
	}
}

func TestRateLimitEvent(t *testing.T) {
	g := newLogRig(t, auth.Config{})
	for i := 1; i <= 5; i++ {
		if rec := g.login("wrong-password-xyz"); rec.Code != 401 {
			t.Fatalf("attempt %d: %d", i, rec.Code)
		}
	}
	g.sink.Reset()
	rec := g.login("wrong-password-xyz")
	if rec.Code != 429 || rec.Header().Get("Retry-After") != "60" {
		t.Fatalf("attempt 6: %d Retry-After=%q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if problemKind(t, rec) != "too-many-attempts" {
		t.Fatalf("body: %s", rec.Body.String())
	}
	line := g.onlyAccess(t)
	wantFields(t, line, "status", float64(429), "error_code", "too-many-attempts", "error_reason", "window_exceeded", "auth_result", "not_required")
	events := g.byEvent(t, "security.rate_limited")
	if len(events) != 1 {
		t.Fatalf("rate_limited events = %d\n%s", len(events), g.sink.String())
	}
	wantFields(t, events[0], "level", "WARN", "request_id", line["request_id"], "limiter", "auth_attempts", "reason", "window_exceeded")
	if len(g.byEvent(t, "auth.login.failure")) != 0 {
		t.Fatal("a throttled attempt is not a credential check")
	}
	if strings.Contains(g.sink.String(), logOwnerEmail) {
		t.Fatal("log contains the e-mail")
	}
}

func TestInvalidOriginEvent(t *testing.T) {
	g := newLogRig(t, auth.Config{})
	token := g.token(t)
	for _, c := range []struct {
		name, reason string
		mods         []func(*http.Request)
	}{
		{"no Origin header", "origin_missing", []func(*http.Request){logHeader("X-Julius-Request", "1")}},
		{"no headers at all", "origin_missing", nil},
		{"foreign origin", "origin_mismatch", []func(*http.Request){logHeader("Origin", "https://SECRET-ORIGIN.example"), logHeader("X-Julius-Request", "1")}},
		{"foreign origin without the request header", "origin_mismatch", []func(*http.Request){logHeader("Origin", "https://SECRET-ORIGIN.example")}},
		{"origin without the request header", "request_header_missing", []func(*http.Request){logHeader("Origin", testOrigin)}},
		{"wrong request header value", "request_header_missing", []func(*http.Request){logHeader("Origin", testOrigin), logHeader("X-Julius-Request", "0")}},
	} {
		t.Run(c.name, func(t *testing.T) {
			g.sink.Reset()
			logged := g.do("POST", "/api/categories", `{}`, append(c.mods, logCookie(token))...)
			plain := serveVia(g.plain, "POST", "/api/categories", `{}`, append(c.mods, logCookie(token))...)

			// Same answer as without logging.
			if logged.Code != 403 || logged.Code != plain.Code || logged.Body.String() != plain.Body.String() {
				t.Fatalf("logged %d %q, plain %d %q", logged.Code, logged.Body.String(), plain.Code, plain.Body.String())
			}
			if problemKind(t, logged) != "invalid-origin" {
				t.Fatalf("body: %s", logged.Body.String())
			}

			line := g.onlyAccess(t)
			wantFields(t, line, "status", float64(403), "error_code", "invalid-origin", "error_reason", c.reason, "auth_type", "none", "auth_result", "not_required")
			events := g.byEvent(t, "security.invalid_origin")
			if len(events) != 1 {
				t.Fatalf("events = %d\n%s", len(events), g.sink.String())
			}
			wantFields(t, events[0], "level", "WARN", "request_id", line["request_id"], "reason", c.reason)
			if len(events[0]) != 6 { // time, level, msg, event, request_id, reason
				t.Fatalf("unexpected fields: %v", events[0])
			}
			if strings.Contains(g.sink.String(), "SECRET-ORIGIN") {
				t.Fatal("log contains the offending origin")
			}
		})
	}

	t.Run("safe methods are not checked", func(t *testing.T) {
		g.sink.Reset()
		g.do("GET", "/api/categories", "", logCookie(token), logHeader("Origin", "https://evil.example"))
		if n := len(g.byEvent(t, "security.invalid_origin")); n != 0 {
			t.Fatalf("events = %d", n)
		}
	})
}
