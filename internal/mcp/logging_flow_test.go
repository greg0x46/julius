package mcp

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/settings"
)

const syntheticRedirect = "https://client.example/callback"

func postForm(h http.Handler, path string, form url.Values, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
	return oauthRequest(h, "POST", path, form.Encode(), cookie, origin)
}

// expectOAuthError checks the public JSON error, which diagnostics must not change.
func expectOAuthError(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if w.Code != status || strings.TrimSpace(w.Body.String()) != `{"error":"`+code+`"}` {
		t.Fatalf("public response changed: %d %s (want %d %s)", w.Code, w.Body, status, code)
	}
}

// expectRedirectError checks the 303 back to the validated client.
func expectRedirectError(t *testing.T, w *httptest.ResponseRecorder, code string) {
	t.Helper()
	u, err := url.Parse(w.Header().Get("Location"))
	if w.Code != 303 || err != nil || u.Query().Get("error") != code || u.Query().Get("state") != "opaque-state" || u.Query().Get("iss") != testConfig.PublicURL {
		t.Fatalf("public redirect changed: %d %q (want error %s)", w.Code, w.Header().Get("Location"), code)
	}
}

func expectAbsent(t *testing.T, raw string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if s != "" && strings.Contains(raw, s) {
			t.Fatalf("%q reached the log:\n%s", s, raw)
		}
	}
}

func TestOAuthFlowAccessAndEventLines(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, _ := setup(t, conn)
		h, sink := newLogged(t, store)
		f := flowOn(t, h, "Synthetic assistant", syntheticRedirect)
		prefix := f.client[:8]

		reg := sink.one(t)
		reg.expectRequest(t, "POST /oauth/register", 201, "none", "not_required")
		reg.expectEvent(t, "oauth.client.registered", map[string]string{"client_name": "Synthetic assistant", "client_id_prefix": prefix})

		f.begin(t)
		c := sink.one(t)
		c.expectRequest(t, "GET /oauth/authorize", 200, "none", "not_required")
		c.expectClient(t, "Synthetic assistant", prefix)
		c.expectError(t, wantError{})
		c.expectNoEvents(t)

		allow := url.Values{"request": {f.pending}, "decision": {"allow"}, "email": {"owner@example.com"}, "password": {"synthetic-password"}}
		w := postForm(h, "/oauth/authorize", allow, f.cookie, testConfig.PublicURL)
		code := consentCallback(t, w).Query().Get("code")
		c = sink.one(t)
		c.expectRequest(t, "POST /oauth/authorize", 200, "none", "not_required")
		c.expectClient(t, "Synthetic assistant", prefix)
		c.expectError(t, wantError{})
		c.expectEvent(t, "oauth.consent.granted", map[string]string{"scope": mcpScope, "client_name": "Synthetic assistant", "client_id_prefix": prefix})
		if len(c.events) != 1 {
			t.Fatalf("one event expected: %v", c.eventNames())
		}

		token := f.token(t, code)
		c = sink.one(t)
		c.expectRequest(t, "POST /oauth/token", 200, "none", "not_required")
		c.expectClient(t, "", prefix)
		c.expectNoEvents(t)

		w = request(t, h, token, "", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
		if w.Code != 200 {
			t.Fatalf("tools: %d %s", w.Code, w.Body)
		}
		c = sink.one(t)
		c.expectRequest(t, "/mcp", 200, "oauth", "ok")
		c.expectClient(t, "Synthetic assistant", prefix)
		c.expectError(t, wantError{})
		c.expectNoEvents(t)

		revoke := url.Values{"client_id": {f.client}, "token": {token}}
		if w = postForm(h, "/oauth/revoke", revoke, nil, ""); w.Code != 200 {
			t.Fatalf("revoke: %d", w.Code)
		}
		c = sink.one(t)
		c.expectRequest(t, "POST /oauth/revoke", 200, "none", "not_required")
		c.expectClient(t, "Synthetic assistant", prefix)
		c.expectEvent(t, "oauth.token.revoked", map[string]string{"client_id_prefix": prefix})

		// Revoking a token that no longer exists is still a 200 but not an event.
		if w = postForm(h, "/oauth/revoke", revoke, nil, ""); w.Code != 200 {
			t.Fatalf("second revoke: %d", w.Code)
		}
		sink.one(t).expectNoEvents(t)

		w = request(t, h, token, "", `{}`)
		if w.Code != 401 {
			t.Fatalf("revoked token: %d", w.Code)
		}
		c = sink.one(t)
		c.expectRequest(t, "/mcp", 401, "oauth", "rejected")
		c.expectError(t, wantError{code: "unauthorized", reason: "invalid_token"})
		c.expectClient(t, "", "")
		expectAbsent(t, c.raw, token, f.client)
	})
}

func TestRevokeOnlyReportsRealRevocations(t *testing.T) {
	store, _ := setup(t, dbtest.OpenSQLite(t))
	h, sink := newLogged(t, store)
	f := flowOn(t, h, "Synthetic assistant", syntheticRedirect)
	other := flowOn(t, h, "Another assistant", syntheticRedirect)
	token := f.token(t, f.approve(t))
	sink.drain()

	cases := []struct {
		name   string
		form   url.Values
		header string
		status int
		code   string
		want   wantError
		event  bool
		client *oauthFlow // the validated client the line names
	}{
		{name: "token of another client", form: url.Values{"client_id": {other.client}, "token": {token}}, status: 200, client: other},
		{name: "unknown token", form: url.Values{"client_id": {f.client}, "token": {"never-issued"}}, status: 200, client: f},
		{name: "unknown client", form: url.Values{"client_id": {"unknown"}, "token": {token}}, status: 401, code: "invalid_client", want: wantError{code: "invalid_client", reason: "unknown_client", field: "client_id"}},
		{name: "client secret", form: url.Values{"client_id": {f.client}, "token": {token}, "client_secret": {"s3cr3t-value"}}, status: 401, code: "invalid_client", want: wantError{code: "invalid_client", reason: "client_authentication_not_supported", field: "client_secret"}},
		{name: "authorization header", form: url.Values{"client_id": {f.client}, "token": {token}}, header: "Basic c2VjcmV0", status: 401, code: "invalid_client", want: wantError{code: "invalid_client", reason: "client_authentication_not_supported", field: "authorization"}},
		{name: "duplicate token", form: url.Values{"client_id": {f.client}, "token": {token, token}}, status: 400, code: "invalid_request", want: wantError{code: "invalid_request", reason: "duplicate_parameter", field: "token"}},
		{name: "duplicate unknown name", form: url.Values{"client_id": {f.client}, "x-evil": {"1", "2"}}, status: 400, code: "invalid_request", want: wantError{code: "invalid_request", reason: "duplicate_parameter", field: "other"}},
		{name: "real revocation", form: url.Values{"client_id": {f.client}, "token": {token}}, status: 200, event: true, client: f},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/oauth/revoke", strings.NewReader(tc.form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tc.header != "" {
				r.Header.Set("Authorization", tc.header)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if tc.code != "" {
				expectOAuthError(t, w, tc.status, tc.code)
			} else if w.Code != tc.status {
				t.Fatalf("status %d", w.Code)
			}
			c := sink.one(t)
			c.expectError(t, tc.want)
			if tc.event {
				c.expectEvent(t, "oauth.token.revoked", map[string]string{"client_id_prefix": f.client[:8], "client_name": "Synthetic assistant"})
			} else {
				c.expectNoEvents(t)
			}
			if tc.client == nil {
				c.expectClient(t, "", "")
			} else {
				c.expectClient(t, map[*oauthFlow]string{f: "Synthetic assistant", other: "Another assistant"}[tc.client], tc.client.client[:8])
			}
			expectAbsent(t, c.raw, token, "s3cr3t-value", "c2VjcmV0", "never-issued", "x-evil")
		})
	}
}

func TestAuthorizeRejectionsAreDiagnosed(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, _ := setup(t, conn)
		h, sink := newLogged(t, store)
		f := flowOn(t, h, "Synthetic assistant", syntheticRedirect)
		sink.drain()
		prefix := f.client[:8]
		with := func(key string, value ...string) string {
			p := url.Values{}
			for k, v := range f.params {
				p[k] = v
			}
			if len(value) == 0 {
				p.Del(key)
			} else {
				p[key] = value
			}
			return p.Encode()
		}
		tests := []struct {
			name, query string
			status      int
			redirect    bool // answered by a redirect to the validated client
			code        string
			want        wantError
			client      bool
		}{
			{name: "query too long", query: "client_id=" + strings.Repeat("a", 9000), status: 400, code: "invalid_request", want: wantError{code: "invalid_request", reason: "query_too_long"}},
			{name: "query unparseable", query: "client_id=%zz", status: 400, code: "invalid_request", want: wantError{code: "invalid_request", reason: "query_unparseable"}},
			{name: "duplicate state", query: f.params.Encode() + "&state=again", status: 400, code: "invalid_request", want: wantError{code: "invalid_request", reason: "duplicate_parameter", field: "state"}},
			{name: "duplicate unknown name", query: f.params.Encode() + "&x-evil=1&x-evil=2", status: 400, code: "invalid_request", want: wantError{code: "invalid_request", reason: "duplicate_parameter", field: "other"}},
			{name: "first duplicate wins", query: f.params.Encode() + "&state=again&client_id=again", status: 400, code: "invalid_request", want: wantError{code: "invalid_request", reason: "duplicate_parameter", field: "client_id"}},
			{name: "unknown client", query: with("client_id", "unknown-client"), status: 400, code: "invalid_client", want: wantError{code: "invalid_client", reason: "unknown_client", field: "client_id"}},
			{name: "missing client", query: with("client_id"), status: 400, code: "invalid_client", want: wantError{code: "invalid_client", reason: "unknown_client", field: "client_id"}},
			{name: "redirect mismatch", query: with("redirect_uri", "https://other.example/callback"), status: 400, code: "invalid_client", want: wantError{code: "invalid_client", reason: "redirect_uri_mismatch", field: "redirect_uri"}},
			{name: "response type", query: with("response_type", "token"), status: 303, redirect: true, code: "unsupported_response_type", want: wantError{code: "unsupported_response_type", reason: "response_type_not_code", field: "response_type"}, client: true},
			{name: "resource", query: with("resource", "https://other.example/mcp"), status: 303, redirect: true, code: "invalid_target", want: wantError{code: "invalid_target", reason: "resource_mismatch", field: "resource"}, client: true},
			{name: "missing resource", query: with("resource"), status: 303, redirect: true, code: "invalid_target", want: wantError{code: "invalid_target", reason: "resource_mismatch", field: "resource"}, client: true},
			{name: "unknown scope", query: with("scope", "bogus"), status: 303, redirect: true, code: "invalid_scope", want: wantError{code: "invalid_scope", reason: "scope_not_supported", field: "scope", values: []string{"other"}}, client: true},
			{name: "oidc scopes", query: with("scope", "mcp:access openid offline_access"), status: 303, redirect: true, code: "invalid_scope", want: wantError{code: "invalid_scope", reason: "scope_not_supported", field: "scope", values: []string{"mcp:access", "openid", "offline_access"}}, client: true},
			{name: "duplicate scope", query: with("scope", "mcp:access mcp:access"), status: 303, redirect: true, code: "invalid_scope", want: wantError{code: "invalid_scope", reason: "scope_duplicate", field: "scope", values: []string{"mcp:access", "mcp:access"}}, client: true},
			{name: "operations alone", query: with("scope", "mcp:operations"), status: 303, redirect: true, code: "invalid_scope", want: wantError{code: "invalid_scope", reason: "scope_invalid_combination", field: "scope", values: []string{"mcp:operations"}}, client: true},
			{name: "operations without write", query: with("scope", "mcp:access mcp:operations"), status: 303, redirect: true, code: "invalid_scope", want: wantError{code: "invalid_scope", reason: "scope_invalid_combination", field: "scope", values: []string{"mcp:access", "mcp:operations"}}, client: true},
			{name: "plain PKCE", query: with("code_challenge_method", "plain"), status: 303, redirect: true, code: "invalid_request", want: wantError{code: "invalid_request", reason: "code_challenge_method_not_s256", field: "code_challenge_method"}, client: true},
			{name: "missing PKCE method", query: with("code_challenge_method"), status: 303, redirect: true, code: "invalid_request", want: wantError{code: "invalid_request", reason: "code_challenge_method_not_s256", field: "code_challenge_method"}, client: true},
			{name: "malformed challenge", query: with("code_challenge", "not-a-challenge-value"), status: 303, redirect: true, code: "invalid_request", want: wantError{code: "invalid_request", reason: "code_challenge_invalid", field: "code_challenge"}, client: true},
			{name: "missing challenge", query: with("code_challenge"), status: 303, redirect: true, code: "invalid_request", want: wantError{code: "invalid_request", reason: "code_challenge_invalid", field: "code_challenge"}, client: true},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				w := oauthRequest(h, "GET", "/oauth/authorize?"+tc.query, "", nil, "")
				if tc.redirect {
					expectRedirectError(t, w, tc.code)
				} else {
					expectOAuthError(t, w, tc.status, tc.code)
				}
				c := sink.one(t)
				c.expectRequest(t, "GET /oauth/authorize", tc.status, "none", "not_required")
				c.expectError(t, tc.want)
				c.expectNoEvents(t)
				if tc.client {
					c.expectClient(t, "Synthetic assistant", prefix)
				} else {
					c.expectClient(t, "", "")
				}
				expectAbsent(t, c.raw, "client.example", "opaque-state", f.client, f.params.Get("code_challenge"), "x-evil", "not-a-challenge-value", "bogus", "again")
			})
		}

		t.Run("authentication disabled", func(t *testing.T) {
			if err := settings.SetAuthenticationEnabled(context.Background(), conn, false); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := settings.SetAuthenticationEnabled(context.Background(), conn, true); err != nil {
					t.Fatal(err)
				}
			}()
			expectOAuthError(t, oauthRequest(h, "GET", "/oauth/authorize?"+f.params.Encode(), "", nil, ""), 403, "access_denied")
			c := sink.one(t)
			c.expectRequest(t, "GET /oauth/authorize", 403, "none", "not_required")
			c.expectError(t, wantError{code: "access_denied", reason: "authentication_disabled"})
			c.expectNoEvents(t)
		})
	})
}

func TestConsentRejectionsAreDiagnosed(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, _ := setup(t, conn)
		h, sink := newLogged(t, store)
		f := flowOn(t, h, "Synthetic assistant", syntheticRedirect)
		f.begin(t)
		sink.drain()
		prefix := f.client[:8]
		forged := &http.Cookie{Name: f.cookie.Name, Value: "forged-csrf-value"}
		form := func(mutate func(url.Values)) url.Values {
			v := url.Values{"request": {f.pending}, "decision": {"allow"}, "email": {"owner@example.com"}, "password": {"synthetic-password"}}
			if mutate != nil {
				mutate(v)
			}
			return v
		}
		tests := []struct {
			name   string
			form   url.Values
			cookie *http.Cookie
			origin string
			status int
			code   string
			want   wantError
			event  map[string]string // security event expected next to the line
			client bool
		}{
			{name: "origin mismatch", form: form(nil), cookie: f.cookie, origin: "https://evil.example", status: 403, code: "access_denied", want: wantError{code: "access_denied", reason: "origin_mismatch", field: "origin"}, event: map[string]string{"reason": "origin_mismatch"}},
			{name: "origin null", form: form(nil), cookie: f.cookie, origin: "null", status: 403, code: "access_denied", want: wantError{code: "access_denied", reason: "origin_null", field: "origin"}, event: map[string]string{"reason": "origin_null"}},
			{name: "origin missing", form: form(nil), cookie: f.cookie, status: 403, code: "access_denied", want: wantError{code: "access_denied", reason: "origin_missing", field: "origin"}, event: map[string]string{"reason": "origin_missing"}},
			{name: "request not found", form: form(func(v url.Values) { v.Set("request", "no-such-request") }), cookie: f.cookie, origin: testConfig.PublicURL, status: 400, code: "invalid_request", want: wantError{code: "invalid_request", reason: "consent_request_not_found", field: "request"}},
			{name: "duplicate request", form: form(func(v url.Values) { v.Add("request", "other-request") }), cookie: f.cookie, origin: testConfig.PublicURL, status: 400, code: "invalid_request", want: wantError{code: "invalid_request", reason: "duplicate_parameter", field: "request"}},
			{name: "missing CSRF cookie", form: form(nil), origin: testConfig.PublicURL, status: 403, code: "access_denied", want: wantError{code: "access_denied", reason: "csrf_cookie_missing"}, client: true},
			{name: "forged CSRF cookie", form: form(nil), cookie: forged, origin: testConfig.PublicURL, status: 403, code: "access_denied", want: wantError{code: "access_denied", reason: "csrf_mismatch"}, client: true},
			{name: "invalid decision", form: form(func(v url.Values) { v.Set("decision", "maybe") }), cookie: f.cookie, origin: testConfig.PublicURL, status: 400, code: "invalid_request", want: wantError{code: "invalid_request", reason: "invalid_decision", field: "decision"}, client: true},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				w := postForm(h, "/oauth/authorize", tc.form, tc.cookie, tc.origin)
				expectOAuthError(t, w, tc.status, tc.code)
				c := sink.one(t)
				c.expectRequest(t, "POST /oauth/authorize", tc.status, "none", "not_required")
				c.expectError(t, tc.want)
				if tc.event != nil {
					c.expectEvent(t, "security.invalid_origin", tc.event)
					if len(c.events) != 1 {
						t.Fatalf("one event expected: %v", c.eventNames())
					}
				} else {
					c.expectNoEvents(t)
				}
				if tc.client {
					c.expectClient(t, "Synthetic assistant", prefix)
				} else {
					c.expectClient(t, "", "")
				}
				expectAbsent(t, c.raw, "evil.example", "forged-csrf-value", f.cookie.Value, f.pending, "owner@example.com", "synthetic-password", "other-request", "no-such-request")
			})
		}

		t.Run("form content type", func(t *testing.T) {
			r := httptest.NewRequest("POST", "/oauth/authorize", strings.NewReader(`{"request":"x"}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Origin", testConfig.PublicURL)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			expectOAuthError(t, w, 400, "invalid_request")
			sink.one(t).expectError(t, wantError{code: "invalid_request", reason: "invalid_content_type", field: "content_type"})
		})
	})
}

func TestConsentCredentialsDenyAndGrantEvents(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, _ := setup(t, conn)
		h, sink := newLogged(t, store)
		f := flowOn(t, h, "Synthetic assistant", syntheticRedirect)
		prefix := f.client[:8]
		f.begin(t)
		sink.drain()

		wrong := url.Values{"request": {f.pending}, "decision": {"allow"}, "email": {"someone@example.org"}, "password": {"wrong-password-xyz"}}
		w := postForm(h, "/oauth/authorize", wrong, f.cookie, testConfig.PublicURL)
		if w.Code != 401 || !strings.Contains(w.Body.String(), `role="alert"`) {
			t.Fatalf("invalid credentials: %d", w.Code)
		}
		c := sink.one(t)
		c.expectRequest(t, "POST /oauth/authorize", 401, "none", "rejected")
		c.expectError(t, wantError{code: "invalid_credentials", reason: "invalid_credentials"})
		c.expectClient(t, "Synthetic assistant", prefix)
		c.expectEvent(t, "oauth.credentials.rejected", map[string]string{"reason": "invalid_credentials", "client_name": "Synthetic assistant", "client_id_prefix": prefix})
		if len(c.events) != 1 {
			t.Fatalf("only the rejection is an event: %v", c.eventNames())
		}
		expectAbsent(t, c.raw, "someone@example.org", "wrong-password-xyz", "example.org")

		// The request survives a rejected attempt; denying consumes it.
		deny := url.Values{"request": {f.pending}, "decision": {"deny"}}
		w = postForm(h, "/oauth/authorize", deny, f.cookie, testConfig.PublicURL)
		if u := consentCallback(t, w); u.Query().Get("error") != "access_denied" {
			t.Fatalf("deny: %s", u)
		}
		c = sink.one(t)
		c.expectRequest(t, "POST /oauth/authorize", 200, "none", "not_required")
		c.expectError(t, wantError{code: "access_denied", reason: "user_denied"})
		c.expectEvent(t, "oauth.consent.denied", map[string]string{"client_name": "Synthetic assistant", "client_id_prefix": prefix})
		if len(c.events) != 1 {
			t.Fatalf("only the denial is an event: %v", c.eventNames())
		}

		// Replaying a consumed request is neither a denial nor a grant.
		expectOAuthError(t, postForm(h, "/oauth/authorize", deny, f.cookie, testConfig.PublicURL), 400, "invalid_request")
		c = sink.one(t)
		c.expectError(t, wantError{code: "invalid_request", reason: "consent_request_not_found", field: "request"})
		c.expectNoEvents(t)

		// An expired request is not found either, and a granted one reports its scope.
		f.begin(t)
		sink.drain()
		if _, err := conn.Exec(`UPDATE oauth_grants SET expires_at=? WHERE token_hash=?`, db.FormatTime(time.Now().Add(-time.Minute)), tokenHash(f.pending)); err != nil {
			t.Fatal(err)
		}
		allow := url.Values{"request": {f.pending}, "decision": {"allow"}, "email": {"owner@example.com"}, "password": {"synthetic-password"}}
		expectOAuthError(t, postForm(h, "/oauth/authorize", allow, f.cookie, testConfig.PublicURL), 400, "invalid_request")
		c = sink.one(t)
		c.expectError(t, wantError{code: "invalid_request", reason: "consent_request_not_found", field: "request"})
		c.expectNoEvents(t)

		f.params.Set("scope", writeScope+" "+mcpScope)
		f.begin(t)
		sink.drain()
		allow.Set("request", f.pending)
		allow.Set("scope", mcpScope+" "+writeScope)
		consentCallback(t, postForm(h, "/oauth/authorize", allow, f.cookie, testConfig.PublicURL))
		c = sink.one(t)
		c.expectEvent(t, "oauth.consent.granted", map[string]string{"scope": mcpScope + " " + writeScope, "client_name": "Synthetic assistant", "client_id_prefix": prefix})
		expectAbsent(t, c.raw, "owner@example.com", "synthetic-password")
	})
}

func TestConsentAttemptsAreRateLimitedAndReported(t *testing.T) {
	store, _ := setup(t, dbtest.OpenSQLite(t))
	h, sink := newLogged(t, store)
	f := flowOn(t, h, "Synthetic assistant", syntheticRedirect)
	f.begin(t)
	sink.drain()
	wrong := url.Values{"request": {f.pending}, "decision": {"allow"}, "email": {"owner@example.com"}, "password": {"wrong"}}
	for i := 0; i < 5; i++ {
		if w := postForm(h, "/oauth/authorize", wrong, f.cookie, testConfig.PublicURL); w.Code != 401 {
			t.Fatalf("attempt %d: %d", i, w.Code)
		}
		sink.drain()
	}
	w := postForm(h, "/oauth/authorize", wrong, f.cookie, testConfig.PublicURL)
	expectOAuthError(t, w, 429, "temporarily_unavailable")
	if w.Header().Get("Retry-After") != "60" {
		t.Fatal("Retry-After changed")
	}
	c := sink.one(t)
	c.expectRequest(t, "POST /oauth/authorize", 429, "none", "not_required")
	c.expectError(t, wantError{code: "temporarily_unavailable", reason: "rate_limited"})
	c.expectEvent(t, "security.rate_limited", map[string]string{"limiter": "oauth_consent_attempts", "reason": "window_exceeded"})
	if len(c.events) != 1 {
		t.Fatalf("one event expected: %v", c.eventNames())
	}
}

func TestBusyHashingIsReported(t *testing.T) {
	store, _ := setup(t, dbtest.OpenSQLite(t))
	o := &oauthServer{store: store, config: testConfig, hashing: make(chan struct{}, 1)}
	h, sink := withLogging(o.routes(http.NotFoundHandler()))
	f := flowOn(t, h, "Synthetic assistant", syntheticRedirect)
	f.begin(t)
	sink.drain()
	o.hashing <- struct{}{}
	allow := url.Values{"request": {f.pending}, "decision": {"allow"}, "email": {"owner@example.com"}, "password": {"synthetic-password"}}
	w := postForm(h, "/oauth/authorize", allow, f.cookie, testConfig.PublicURL)
	expectOAuthError(t, w, 429, "temporarily_unavailable")
	if w.Header().Get("Retry-After") != "" {
		t.Fatal("Retry-After must not be added")
	}
	c := sink.one(t)
	c.expectError(t, wantError{code: "temporarily_unavailable", reason: "rate_limited"})
	c.expectEvent(t, "security.rate_limited", map[string]string{"limiter": "oauth_hashing_busy", "reason": "busy"})
	c.expectRequest(t, "POST /oauth/authorize", 429, "none", "not_required")
}

func TestOAuthRequestLimiterIsReported(t *testing.T) {
	store, _ := setup(t, dbtest.OpenSQLite(t))
	h, sink := newLogged(t, store)
	for i := 0; i < 120; i++ {
		if w := oauthRequest(h, "GET", "/.well-known/oauth-authorization-server", "", nil, ""); w.Code != 200 {
			t.Fatalf("request %d: %d", i, w.Code)
		}
	}
	sink.drain()
	w := oauthRequest(h, "GET", "/.well-known/oauth-authorization-server", "", nil, "")
	expectOAuthError(t, w, 429, "temporarily_unavailable")
	if w.Header().Get("Retry-After") != "60" {
		t.Fatal("Retry-After changed")
	}
	c := sink.one(t)
	c.expectRequest(t, "GET /.well-known/oauth-authorization-server", 429, "none", "not_required")
	c.expectError(t, wantError{code: "temporarily_unavailable", reason: "rate_limited"})
	c.expectEvent(t, "security.rate_limited", map[string]string{"limiter": "oauth_requests", "reason": "window_exceeded"})
	if len(c.events) != 1 {
		t.Fatalf("one event expected: %v", c.eventNames())
	}
}

func TestTokenRejectionsAreDiagnosed(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, _ := setup(t, conn)
		wrongVerifier := strings.Repeat("w", 43)
		tests := []struct {
			name     string
			mutate   func(url.Values)
			header   string
			status   int
			code     string
			want     wantError
			client   bool // client recorded after the code, PKCE and binding checked out
			consumed bool // the code is spent by the failed request
			setup    func()
		}{
			{name: "refresh grant", mutate: func(v url.Values) { v.Set("grant_type", "refresh_token") }, status: 400, code: "unsupported_grant_type", want: wantError{code: "unsupported_grant_type", reason: "unsupported_grant_type", field: "grant_type", values: []string{"refresh_token"}}},
			{name: "missing grant type", mutate: func(v url.Values) { v.Del("grant_type") }, status: 400, code: "unsupported_grant_type", want: wantError{code: "unsupported_grant_type", reason: "unsupported_grant_type", field: "grant_type", values: []string{"other"}}},
			{name: "unknown grant type", mutate: func(v url.Values) { v.Set("grant_type", "x-evil") }, status: 400, code: "unsupported_grant_type", want: wantError{code: "unsupported_grant_type", reason: "unsupported_grant_type", field: "grant_type", values: []string{"other"}}},
			{name: "client secret", mutate: func(v url.Values) { v.Set("client_secret", "s3cr3t-value") }, status: 401, code: "invalid_client", want: wantError{code: "invalid_client", reason: "client_authentication_not_supported", field: "client_secret"}},
			{name: "authorization header", header: "Basic c2VjcmV0", status: 401, code: "invalid_client", want: wantError{code: "invalid_client", reason: "client_authentication_not_supported", field: "authorization"}},
			{name: "duplicate code", mutate: func(v url.Values) { v.Add("code", "second") }, status: 400, code: "invalid_request", want: wantError{code: "invalid_request", reason: "duplicate_parameter", field: "code"}},
			{name: "unknown code", mutate: func(v url.Values) { v.Set("code", "never-issued") }, status: 400, code: "invalid_grant", want: wantError{code: "invalid_grant", reason: "code_not_found_or_expired", field: "code"}},
			{name: "expired code", status: 400, code: "invalid_grant", want: wantError{code: "invalid_grant", reason: "code_not_found_or_expired", field: "code"}, setup: func() {
				if _, err := conn.Exec(`UPDATE oauth_grants SET expires_at=? WHERE kind='code'`, db.FormatTime(time.Now().Add(-time.Minute))); err != nil {
					t.Fatal(err)
				}
			}},
			{name: "verifier format", mutate: func(v url.Values) { v.Set("code_verifier", "short") }, status: 400, code: "invalid_grant", want: wantError{code: "invalid_grant", reason: "verifier_invalid_format", field: "code_verifier"}, consumed: true},
			{name: "verifier mismatch", mutate: func(v url.Values) { v.Set("code_verifier", wrongVerifier) }, status: 400, code: "invalid_grant", want: wantError{code: "invalid_grant", reason: "pkce_mismatch", field: "code_verifier"}, consumed: true},
			{name: "client mismatch", mutate: func(v url.Values) { v.Set("client_id", "other-client") }, status: 400, code: "invalid_grant", want: wantError{code: "invalid_grant", reason: "client_id_mismatch", field: "client_id"}, consumed: true},
			{name: "redirect mismatch", mutate: func(v url.Values) { v.Set("redirect_uri", "https://other.example/callback") }, status: 400, code: "invalid_grant", want: wantError{code: "invalid_grant", reason: "redirect_uri_mismatch", field: "redirect_uri"}, consumed: true},
			{name: "first failing check wins", mutate: func(v url.Values) { v.Set("code_verifier", wrongVerifier); v.Set("client_id", "other-client") }, status: 400, code: "invalid_grant", want: wantError{code: "invalid_grant", reason: "pkce_mismatch", field: "code_verifier"}, consumed: true},
			{name: "resource", mutate: func(v url.Values) { v.Set("resource", "https://other.example/mcp") }, status: 400, code: "invalid_target", want: wantError{code: "invalid_target", reason: "resource_mismatch", field: "resource"}, client: true, consumed: true},
			{name: "owner revision", status: 400, code: "invalid_grant", want: wantError{code: "invalid_grant", reason: "owner_revision_changed"}, client: true, consumed: true, setup: func() {
				if err := store.ResetPassword(context.Background(), "synthetic-password"); err != nil {
					t.Fatal(err)
				}
			}},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				// A handler per case: the consent attempt limiter is per handler.
				h, sink := newLogged(t, store)
				f := flowOn(t, h, "Synthetic assistant", syntheticRedirect)
				prefix := f.client[:8]
				code := f.approve(t)
				if tc.setup != nil {
					tc.setup()
				}
				form := f.exchange(code)
				if tc.mutate != nil {
					tc.mutate(form)
				}
				r := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(form.Encode()))
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				if tc.header != "" {
					r.Header.Set("Authorization", tc.header)
				}
				sink.drain()
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				expectOAuthError(t, w, tc.status, tc.code)
				c := sink.one(t)
				c.expectRequest(t, "POST /oauth/token", tc.status, "none", "not_required")
				c.expectError(t, tc.want)
				c.expectNoEvents(t)
				if tc.client {
					c.expectClient(t, "", prefix)
				} else {
					c.expectClient(t, "", "")
				}
				expectAbsent(t, c.raw, code, f.verifier, wrongVerifier, "s3cr3t-value", "c2VjcmV0", "never-issued", "x-evil", "client.example", f.client)

				// Whether the code survives is part of the protocol and must not change.
				retry := oauthRequest(h, "POST", "/oauth/token", f.exchange(code).Encode(), nil, "")
				sink.drain()
				switch {
				case tc.name == "owner revision":
					// Reset invalidates every grant of the old revision.
				case tc.consumed || tc.name == "expired code":
					if retry.Code != 400 {
						t.Fatalf("code reusable after %s: %d", tc.name, retry.Code)
					}
				default:
					if retry.Code != 200 {
						t.Fatalf("code consumed by early failure %s: %d %s", tc.name, retry.Code, retry.Body)
					}
				}
			})
		}
	})
}

func TestTokenStorageFailureIsLoggedAsDatabaseError(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	store, _ := setup(t, conn)
	h, sink := newLogged(t, store)
	if _, err := conn.Exec(`DROP TABLE oauth_grants`); err != nil {
		t.Fatal(err)
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {"x"}}
	expectOAuthError(t, postForm(h, "/oauth/token", form, nil, ""), 503, "temporarily_unavailable")
	c := sink.one(t)
	c.expectError(t, wantError{code: "temporarily_unavailable", reason: "db_error"})
	expectAbsent(t, c.raw, "no such table", "oauth_grants")
}

func TestMCPGateIsDiagnosed(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, _ := setup(t, conn)
		h, sink := newLogged(t, store)
		f := flowOn(t, h, "Synthetic assistant", syntheticRedirect)
		prefix := f.client[:8]
		token := f.token(t, f.approve(t))
		sink.drain()

		raw := func(authorization, origin string) *httptest.ResponseRecorder {
			r := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{}`))
			r.Header.Set("Content-Type", "application/json")
			if authorization != "" {
				r.Header.Set("Authorization", authorization)
			}
			if origin != "" {
				r.Header.Set("Origin", origin)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			return w
		}
		t.Run("no bearer", func(t *testing.T) {
			w := raw("", "")
			if w.Code != 401 || w.Header().Get("WWW-Authenticate") == "" {
				t.Fatalf("no bearer: %d", w.Code)
			}
			c := sink.one(t)
			c.expectRequest(t, "/mcp", 401, "none", "rejected")
			c.expectError(t, wantError{code: "unauthorized", reason: "missing_bearer"})
			c.expectClient(t, "", "")
			c.expectNoEvents(t)
		})
		t.Run("other scheme", func(t *testing.T) {
			if w := raw("Basic c2VjcmV0", ""); w.Code != 401 {
				t.Fatalf("other scheme: %d", w.Code)
			}
			c := sink.one(t)
			c.expectRequest(t, "/mcp", 401, "none", "rejected")
			c.expectError(t, wantError{code: "unauthorized", reason: "malformed_authorization"})
			expectAbsent(t, c.raw, "c2VjcmV0")
		})
		t.Run("unknown bearer", func(t *testing.T) {
			if w := raw("Bearer not-a-real-token", ""); w.Code != 401 || w.Header().Get("WWW-Authenticate") == "" {
				t.Fatalf("unknown bearer: %d", w.Code)
			}
			c := sink.one(t)
			c.expectRequest(t, "/mcp", 401, "oauth", "rejected")
			c.expectError(t, wantError{code: "unauthorized", reason: "invalid_token"})
			c.expectClient(t, "", "")
			expectAbsent(t, c.raw, "not-a-real-token")
		})
		t.Run("expired bearer", func(t *testing.T) {
			expired := f.token(t, f.approve(t))
			if _, err := conn.Exec(`UPDATE oauth_grants SET expires_at=? WHERE token_hash=?`, db.FormatTime(time.Now().Add(-time.Minute)), tokenHash(expired)); err != nil {
				t.Fatal(err)
			}
			sink.drain()
			if w := raw("Bearer "+expired, ""); w.Code != 401 {
				t.Fatalf("expired: %d", w.Code)
			}
			c := sink.one(t)
			c.expectRequest(t, "/mcp", 401, "oauth", "rejected")
			c.expectError(t, wantError{code: "unauthorized", reason: "invalid_token"})
		})
		t.Run("forbidden origin", func(t *testing.T) {
			if w := raw("Bearer "+token, "https://evil.example"); w.Code != 403 {
				t.Fatalf("origin: %d", w.Code)
			}
			c := sink.one(t)
			c.expectRequest(t, "/mcp", 403, "none", "not_required")
			c.expectError(t, wantError{code: "access_denied", reason: "origin_mismatch", field: "origin"})
			c.expectEvent(t, "security.invalid_origin", map[string]string{"reason": "origin_mismatch"})
			expectAbsent(t, c.raw, "evil.example", token)
		})
		t.Run("insufficient scope", func(t *testing.T) {
			bad := f.token(t, f.approve(t))
			if _, err := conn.Exec(`UPDATE oauth_grants SET scope='' WHERE token_hash=?`, tokenHash(bad)); err != nil {
				t.Fatal(err)
			}
			sink.drain()
			w := raw("Bearer "+bad, "")
			if w.Code != 403 || !strings.Contains(w.Header().Get("WWW-Authenticate"), "insufficient_scope") {
				t.Fatalf("scope: %d", w.Code)
			}
			c := sink.one(t)
			c.expectRequest(t, "/mcp", 403, "oauth", "ok")
			c.expectError(t, wantError{code: "insufficient_scope", reason: "scope_invalid"})
			c.expectClient(t, "Synthetic assistant", prefix)
		})
		t.Run("authentication disabled", func(t *testing.T) {
			if err := settings.SetAuthenticationEnabled(context.Background(), conn, false); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := settings.SetAuthenticationEnabled(context.Background(), conn, true); err != nil {
					t.Fatal(err)
				}
			}()
			if w := raw("Bearer "+token, ""); w.Code != 403 {
				t.Fatalf("open mode: %d", w.Code)
			}
			c := sink.one(t)
			c.expectError(t, wantError{code: "access_denied", reason: "authentication_disabled"})
		})
		t.Run("database unavailable", func(t *testing.T) {
			if err := conn.Close(); err != nil {
				t.Fatal(err)
			}
			if w := raw("Bearer "+token, ""); w.Code != 503 || strings.Contains(w.Body.String(), "sql") {
				t.Fatalf("database: %d %s", w.Code, w.Body)
			}
			c := sink.one(t)
			c.expectRequest(t, "/mcp", 503, "oauth", "rejected")
			c.expectError(t, wantError{code: "temporarily_unavailable", reason: "auth_unavailable"})
			expectAbsent(t, c.raw, token)
		})
	})
}

func TestBusyMCPIsReportedWithItsLimiter(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	store, token := setup(t, conn)
	h, sink := newLogged(t, store)
	// Hold SQLite's only connection so authentication requests stay in flight.
	held, err := conn.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{}, maxConcurrentRequests)
	for i := 0; i < maxConcurrentRequests; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			r := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{}`)).WithContext(ctx)
			r.Header.Set("Authorization", "Bearer "+token)
			h.ServeHTTP(httptest.NewRecorder(), r)
		}()
	}
	deadline := time.Now().Add(5 * time.Second)
	for conn.Stats().WaitCount < int64(maxConcurrentRequests) {
		if time.Now().After(deadline) {
			t.Fatal("authentication requests did not start")
		}
		time.Sleep(time.Millisecond)
	}
	sink.drain()
	w := request(t, h, token, "", `{}`)
	if w.Code != 429 || w.Header().Get("Retry-After") != "1" {
		t.Fatalf("saturation: %d", w.Code)
	}
	c := sink.one(t)
	c.expectRequest(t, "/mcp", 429, "none", "not_required")
	c.expectError(t, wantError{code: "too_many_requests", reason: "busy"})
	c.expectEvent(t, "security.rate_limited", map[string]string{"limiter": "mcp_concurrency", "reason": "busy"})
	if len(c.events) != 1 {
		t.Fatalf("one event expected: %v", c.eventNames())
	}
	cancel()
	for i := 0; i < maxConcurrentRequests; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("request did not stop on cancellation")
		}
	}
}

func TestFormFailuresAreDiagnosed(t *testing.T) {
	store, _ := setup(t, dbtest.OpenSQLite(t))
	h, sink := newLogged(t, store)
	tests := []struct {
		name        string
		contentType string
		body        string
		path        string
		want        wantError
	}{
		{name: "content type", contentType: "application/json", body: `{}`, path: "/oauth/token", want: wantError{code: "invalid_request", reason: "invalid_content_type", field: "content_type"}},
		{name: "no content type", body: `grant_type=authorization_code`, path: "/oauth/revoke", want: wantError{code: "invalid_request", reason: "invalid_content_type", field: "content_type"}},
		{name: "bad escape", contentType: "application/x-www-form-urlencoded", body: `grant_type=%zz`, path: "/oauth/token", want: wantError{code: "invalid_request", reason: "form_unparseable"}},
		{name: "oversized", contentType: "application/x-www-form-urlencoded", body: "state=" + strings.Repeat("a", 20<<10), path: "/oauth/token", want: wantError{code: "invalid_request", reason: "body_too_large"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body))
			if tc.contentType != "" {
				r.Header.Set("Content-Type", tc.contentType)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			expectOAuthError(t, w, 400, "invalid_request")
			c := sink.one(t)
			c.expectError(t, tc.want)
			c.expectNoEvents(t)
			expectAbsent(t, c.raw, "%zz", "aaaaaaaa")
		})
	}
}
