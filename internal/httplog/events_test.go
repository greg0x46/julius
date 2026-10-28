package httplog

import (
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

// emit runs fn inside a request and returns every log line decoded; the
// access line is last.
func emit(t *testing.T, fn func(w http.ResponseWriter, r *http.Request)) []map[string]any {
	t.Helper()
	l, s := newTestLogger()
	serve(Middleware(l)(http.HandlerFunc(fn)), newReq("GET", "/x"))
	var out []map[string]any
	for _, line := range s.lines() {
		out = append(out, decode(t, line))
	}
	return out
}

func lastLine(t *testing.T, lines []map[string]any) map[string]any {
	t.Helper()
	if len(lines) == 0 {
		t.Fatal("no log lines")
	}
	return lines[len(lines)-1]
}

func TestEventNamesAndLevels(t *testing.T) {
	want := map[string]string{
		"auth.login.success":         "INFO",
		"auth.login.failure":         "WARN",
		"auth.logout":                "INFO",
		"oauth.client.registered":    "INFO",
		"oauth.consent.granted":      "INFO",
		"oauth.consent.denied":       "INFO",
		"oauth.credentials.rejected": "WARN",
		"oauth.token.revoked":        "INFO",
		"security.rate_limited":      "WARN",
		"security.invalid_origin":    "WARN",
	}
	for name, level := range want {
		t.Run(name, func(t *testing.T) {
			lines := emit(t, func(w http.ResponseWriter, r *http.Request) { Event(r.Context(), name) })
			if len(lines) != 2 {
				t.Fatalf("want event + access line, got %d", len(lines))
			}
			ev, acc := lines[0], lines[1]
			if ev["msg"] != name || ev["event"] != name || ev["level"] != level {
				t.Fatalf("event line: %v", ev)
			}
			if ev["request_id"] != acc["request_id"] || ev["request_id"] == "" {
				t.Fatalf("request_id differs: %v vs %v", ev["request_id"], acc["request_id"])
			}
		})
	}
}

func TestEventShape(t *testing.T) {
	l, s := newTestLogger()
	serve(Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Event(r.Context(), "oauth.client.registered",
			slog.String("client_id_prefix", "abcdefghijklmnop"),
			slog.String("client_name", "My\nApp"),
			slog.String("scope", "mcp:read"),
			slog.String("limiter", "login"),
			slog.String("reason", "first"),
			slog.String("reason", "second"), // last one wins
		)
	})), newReq("GET", "/x"))
	lines := s.lines()
	wantKeys := []string{"time", "level", "msg", "event", "request_id", "reason", "limiter", "scope", "client_name", "client_id_prefix"}
	if got := keysOf(t, lines[0]); !equalStrings(got, wantKeys) {
		t.Fatalf("keys = %v, want %v", got, wantKeys)
	}
	m := decode(t, lines[0])
	if m["reason"] != "second" || m["client_name"] != "My App" || m["client_id_prefix"] != "abcdefgh" {
		t.Fatalf("event: %v", m)
	}
}

func TestEventDropsAndSanitizes(t *testing.T) {
	long := strings.Repeat("é", 300)
	lines := emit(t, func(w http.ResponseWriter, r *http.Request) {
		Event(r.Context(), "not.an.event", slog.String("reason", "x"))
		Event(r.Context(), "AUTH.LOGIN.SUCCESS")
		Event(r.Context(), "auth.logout\nfake", slog.String("reason", "x"))
		Event(r.Context(), "auth.login.failure",
			slog.String("reason", "a\r\nb\"}"),
			slog.String("limiter", long),
			slog.String("scope", long),
			slog.String("client_name", long),
			slog.String("ip", "203.0.113.5"),
			slog.String("user_agent", "UA"),
			slog.String("password", "LEAK"),
			slog.Int("scope_n", 3),
			slog.Group("reason", slog.String("x", "y")),
		)
	})
	if len(lines) != 2 {
		t.Fatalf("want 1 event + access line, got %d: %v", len(lines), lines)
	}
	ev := lines[0]
	if ev["reason"] != `a b"}` {
		t.Errorf("reason = %q", ev["reason"])
	}
	if got := len([]rune(ev["limiter"].(string))); got != 64 {
		t.Errorf("limiter has %d runes", got)
	}
	if got := len([]rune(ev["scope"].(string))); got != 64 {
		t.Errorf("scope has %d runes", got)
	}
	if got := len([]rune(ev["client_name"].(string))); got != 100 {
		t.Errorf("client_name has %d runes", got)
	}
	for _, k := range []string{"ip", "user_agent", "password", "scope_n"} {
		if _, ok := ev[k]; ok {
			t.Errorf("event has non-allowlisted key %q", k)
		}
	}
}

func TestEventOmitsEmptyValuesAndNoAttrs(t *testing.T) {
	lines := emit(t, func(w http.ResponseWriter, r *http.Request) {
		Event(r.Context(), "auth.logout", slog.String("reason", "\r\n "))
	})
	if got := len(lines[0]); got != 5 { // time level msg event request_id
		t.Fatalf("event has %d keys: %v", got, lines[0])
	}
}

func TestEventWithoutMiddlewareIsNoop(t *testing.T) {
	Event(t.Context(), "auth.logout", slog.String("reason", "x")) // must not panic
}

func TestEventRespectsLoggerLevel(t *testing.T) {
	s := &sink{}
	l := slog.New(slog.NewJSONHandler(s, &slog.HandlerOptions{Level: slog.LevelWarn}))
	serve(Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Event(r.Context(), "auth.logout")
		Event(r.Context(), "auth.login.failure")
	})), newReq("GET", "/x"))
	lines := s.lines()
	if len(lines) != 1 || decode(t, lines[0])["event"] != "auth.login.failure" {
		t.Fatalf("lines: %q", s.String())
	}
}

func TestSetError(t *testing.T) {
	exactly64 := strings.Repeat("a", 64)
	tests := []struct {
		name                string
		code, reason, field string
		wantCode            string
		wantReason          string
		wantField           string
	}{
		{"all valid", "invalid_request", "missing_redirect_uri", "redirect_uri", "invalid_request", "missing_redirect_uri", "redirect_uri"},
		{"separators allowed", "a.b:c-d_e9", "", "", "a.b:c-d_e9", "", ""},
		{"exactly 64", exactly64, "", "", exactly64, "", ""},
		{"too long", exactly64 + "a", "", "", invalidValue, "", ""},
		{"uppercase", "Invalid", "", "", invalidValue, "", ""},
		{"space", "a b", "", "", invalidValue, "", ""},
		{"slash", "a/b", "", "", invalidValue, "", ""},
		{"newline", "a\nb", "", "", invalidValue, "", ""},
		{"quote", `a"b`, "", "", invalidValue, "", ""},
		{"non ascii", "é", "", "", invalidValue, "", ""},
		{"empty code", "", "", "", invalidValue, "", ""},
		{"invalid reason", "ok", "Bad Reason", "", "ok", invalidValue, ""},
		{"invalid field", "ok", "", "x\ty", "ok", "", invalidValue},
		{"reason and field too long", "ok", exactly64 + "a", exactly64 + "a", "ok", invalidValue, invalidValue},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := lastLine(t, emit(t, func(w http.ResponseWriter, r *http.Request) {
				SetError(r.Context(), tc.code, tc.reason, tc.field)
			}))
			check := func(key, want string) {
				got, present := m[key]
				if want == "" {
					if present {
						t.Errorf("%s = %v, want omitted", key, got)
					}
					return
				}
				if got != want {
					t.Errorf("%s = %v, want %v", key, got, want)
				}
			}
			check("error_code", tc.wantCode)
			check("error_reason", tc.wantReason)
			check("invalid_field", tc.wantField)
		})
	}
}

func TestSetErrorLastCallWins(t *testing.T) {
	m := lastLine(t, emit(t, func(w http.ResponseWriter, r *http.Request) {
		SetError(r.Context(), "first", "reason_one", "field_one")
		SetError(r.Context(), "second", "", "")
	}))
	if m["error_code"] != "second" {
		t.Fatalf("error_code = %v", m["error_code"])
	}
	if _, ok := m["error_reason"]; ok {
		t.Fatal("reason from the first call survived")
	}
	if _, ok := m["invalid_field"]; ok {
		t.Fatal("field from the first call survived")
	}
}

func TestAccessLogKeyOrderWithAllOptionalFields(t *testing.T) {
	r := newReq("POST", "/x")
	r.Header.Set("CF-Connecting-IP", "203.0.113.4")
	r.Header.Set("User-Agent", "T/1")
	l, s := newTestLogger()
	serve(Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		SetAuth(r.Context(), AuthOAuth, AuthOK)
		SetOAuthClient(r.Context(), "0123456789", "Name")
		SetError(r.Context(), "c", "r", "f")
	})), r)
	want := []string{"time", "level", "msg", "event", "request_id", "method", "route", "status", "duration_ms",
		"response_bytes", "peer_ip", "forwarded_ip", "user_agent", "auth_type", "auth_result",
		"client_name", "client_id_prefix", "error_code", "error_reason", "invalid_field"}
	if got := keysOf(t, s.lines()[0]); !equalStrings(got, want) {
		t.Fatalf("keys = %v\nwant   %v", got, want)
	}
}

func TestSetWriterError(t *testing.T) {
	t.Run("sets only the code, through foreign wrappers", func(t *testing.T) {
		m := lastLine(t, emit(t, func(w http.ResponseWriter, r *http.Request) {
			SetError(r.Context(), "old_code", "kept_reason", "kept_field")
			SetWriterError(unwrapOnly{unwrapOnly{w}}, "problem.unauthorized")
		}))
		if m["error_code"] != "problem.unauthorized" || m["error_reason"] != "kept_reason" || m["invalid_field"] != "kept_field" {
			t.Fatalf("line: %v", m)
		}
	})
	t.Run("validates the code", func(t *testing.T) {
		m := lastLine(t, emit(t, func(w http.ResponseWriter, r *http.Request) {
			SetWriterError(w, "Not Valid!")
		}))
		if m["error_code"] != invalidValue {
			t.Fatalf("line: %v", m)
		}
	})
	t.Run("no-op when the chain has no httplog writer", func(t *testing.T) {
		SetWriterError(nil, "x")
		SetWriterError(&fakeBase{}, "x")
		SetWriterError(unwrapOnly{&fakeBase{}}, "x")
		SetWriterError(cyclic{}, "x") // must terminate
	})
	t.Run("inner middleware owns its writer", func(t *testing.T) {
		l, s := newTestLogger()
		inner := Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			SetWriterError(w, "inner_only")
		}))
		serve(Middleware(l)(inner), newReq("GET", "/x"))
		lines := s.lines()
		if len(lines) != 2 {
			t.Fatalf("lines: %q", s.String())
		}
		first, second := decode(t, lines[0]), decode(t, lines[1]) // inner logs first
		if first["error_code"] != "inner_only" {
			t.Fatalf("inner: %v", first)
		}
		if _, ok := second["error_code"]; ok {
			t.Fatalf("outer: %v", second)
		}
		if first["request_id"] == second["request_id"] {
			t.Fatal("nested middlewares share a request id")
		}
	})
}

type unwrapOnly struct{ http.ResponseWriter }

func (u unwrapOnly) Unwrap() http.ResponseWriter { return u.ResponseWriter }

type cyclic struct{ http.ResponseWriter }

func (c cyclic) Unwrap() http.ResponseWriter { return c }

func TestAuthAndOAuthClient(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		m := lastLine(t, emit(t, func(w http.ResponseWriter, r *http.Request) {}))
		if m["auth_type"] != "none" || m["auth_result"] != "not_required" {
			t.Fatalf("line: %v", m)
		}
		for _, k := range []string{"client_name", "client_id_prefix", "error_code", "error_reason", "invalid_field"} {
			if _, ok := m[k]; ok {
				t.Errorf("%s present by default", k)
			}
		}
	})
	t.Run("reflected", func(t *testing.T) {
		tests := []struct {
			typ  AuthType
			res  AuthResult
			want [2]string
		}{
			{AuthSession, AuthOK, [2]string{"session", "ok"}},
			{AuthOAuth, AuthRejected, [2]string{"oauth", "rejected"}},
			{AuthNone, AuthNotRequired, [2]string{"none", "not_required"}},
			{"bogus\n", "also bogus", [2]string{invalidValue, invalidValue}},
		}
		for _, tc := range tests {
			m := lastLine(t, emit(t, func(w http.ResponseWriter, r *http.Request) {
				SetAuth(r.Context(), tc.typ, tc.res)
			}))
			if m["auth_type"] != tc.want[0] || m["auth_result"] != tc.want[1] {
				t.Errorf("SetAuth(%q,%q) -> %v/%v", tc.typ, tc.res, m["auth_type"], m["auth_result"])
			}
		}
	})
	t.Run("last call wins", func(t *testing.T) {
		m := lastLine(t, emit(t, func(w http.ResponseWriter, r *http.Request) {
			SetAuth(r.Context(), AuthSession, AuthRejected)
			SetAuth(r.Context(), AuthSession, AuthOK)
		}))
		if m["auth_result"] != "ok" {
			t.Fatalf("line: %v", m)
		}
	})
	t.Run("oauth client sanitized", func(t *testing.T) {
		m := lastLine(t, emit(t, func(w http.ResponseWriter, r *http.Request) {
			SetOAuthClient(r.Context(), "abc\r\ndef-ghijklmnop", "Evil\n\"}{\"event\":\"x"+strings.Repeat("n", 200))
		}))
		if m["client_id_prefix"] != "abc def-" {
			t.Errorf("client_id_prefix = %q", m["client_id_prefix"])
		}
		name := m["client_name"].(string)
		if len([]rune(name)) != 100 || strings.ContainsAny(name, "\r\n") {
			t.Errorf("client_name = %q", name)
		}
	})
	t.Run("empty client omitted", func(t *testing.T) {
		m := lastLine(t, emit(t, func(w http.ResponseWriter, r *http.Request) {
			SetOAuthClient(r.Context(), "", "")
		}))
		if _, ok := m["client_name"]; ok {
			t.Fatal("client_name present")
		}
		if _, ok := m["client_id_prefix"]; ok {
			t.Fatal("client_id_prefix present")
		}
	})
	t.Run("setters without middleware are no-ops", func(t *testing.T) {
		ctx := t.Context()
		SetAuth(ctx, AuthSession, AuthOK)
		SetOAuthClient(ctx, "a", "b")
		SetError(ctx, "a", "b", "c")
	})
}

func TestKnown(t *testing.T) {
	tests := []struct {
		value string
		known []string
		want  string
	}{
		{"authorization_code", []string{"authorization_code", "refresh_token"}, "authorization_code"},
		{"refresh_token", []string{"authorization_code", "refresh_token"}, "refresh_token"},
		{"implicit", []string{"authorization_code", "refresh_token"}, "other"},
		{"Authorization_Code", []string{"authorization_code"}, "other"},
		{"", []string{"a"}, "other"},
		{"a", nil, "other"},
		{"x\ny", []string{"x"}, "other"},
	}
	for _, tc := range tests {
		if got := Known(tc.value, tc.known...); got != tc.want {
			t.Errorf("Known(%q) = %q, want %q", tc.value, got, tc.want)
		}
	}
}
