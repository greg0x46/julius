package mcp

import (
	"database/sql"
	"net/http"
	"net/url"
	"testing"

	"github.com/greg0x46/julius/internal/db/dbtest"
)

// TestWriteApprovalPageIsDiagnosed covers /oauth/write, which shares the
// consent page's helpers.
func TestWriteApprovalPageIsDiagnosed(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		f := newWriteFixture(t, conn)
		var clientID string
		if err := conn.QueryRow(`SELECT client_id FROM oauth_grants WHERE token_hash=?`, tokenHash(f.token)).Scan(&clientID); err != nil {
			t.Fatal(err)
		}
		prefix := clientID[:8]
		const name = "Synthetic client <script>"
		h, sink := withLogging(f.handler)
		f.handler = h

		out := f.call(t, "create_manual_transaction", map[string]any{"data": f.manual(), "idempotency_key": "synthetic-key"})
		u, _ := url.Parse(out.ApprovalURL)
		w := oauthRequest(h, "GET", u.RequestURI(), "", nil, "")
		if w.Code != 200 {
			t.Fatalf("preview: %d", w.Code)
		}
		cookie := w.Result().Cookies()[0]
		c := sink.one(t)
		c.expectRequest(t, "GET /oauth/write", 200, "none", "not_required")
		c.expectClient(t, name, prefix)
		c.expectNoEvents(t)

		form := url.Values{"request": {out.RequestID}, "decision": {"allow"}, "email": {"owner@example.com"}, "password": {"synthetic-password"}}
		tests := []struct {
			name   string
			mutate func(url.Values)
			cookie *http.Cookie
			origin string
			status int
			want   wantError
			event  string
			reason string
			authed string // auth_result
		}{
			{name: "origin", cookie: cookie, origin: "https://evil.example", status: 403, want: wantError{code: "access_denied", reason: "origin_mismatch", field: "origin"}, event: "security.invalid_origin", reason: "origin_mismatch", authed: "not_required"},
			{name: "no origin", cookie: cookie, status: 403, want: wantError{code: "access_denied", reason: "origin_missing", field: "origin"}, event: "security.invalid_origin", reason: "origin_missing", authed: "not_required"},
			{name: "missing cookie", origin: testConfig.PublicURL, status: 403, want: wantError{code: "access_denied", reason: "csrf_cookie_missing"}, authed: "not_required"},
			{name: "forged cookie", cookie: &http.Cookie{Name: cookie.Name, Value: "forged"}, origin: testConfig.PublicURL, status: 403, want: wantError{code: "access_denied", reason: "csrf_mismatch"}, authed: "not_required"},
			{name: "decision", mutate: func(v url.Values) { v.Set("decision", "maybe") }, cookie: cookie, origin: testConfig.PublicURL, status: 400, want: wantError{code: "invalid_request", reason: "invalid_decision", field: "decision"}, authed: "not_required"},
			{name: "credentials", mutate: func(v url.Values) { v.Set("password", "wrong") }, cookie: cookie, origin: testConfig.PublicURL, status: 401, want: wantError{code: "invalid_credentials", reason: "invalid_credentials"}, event: "oauth.credentials.rejected", reason: "invalid_credentials", authed: "rejected"},
			{name: "unknown request", mutate: func(v url.Values) { v.Set("request", "no-such-request") }, cookie: cookie, origin: testConfig.PublicURL, status: 410, want: wantError{code: "invalid_request", reason: "write_request_not_found", field: "request"}, authed: "not_required"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				v := url.Values{}
				for k, val := range form {
					v[k] = val
				}
				if tc.mutate != nil {
					tc.mutate(v)
				}
				postForm(h, "/oauth/write", v, tc.cookie, tc.origin)
				c := sink.one(t)
				c.expectRequest(t, "POST /oauth/write", tc.status, "none", tc.authed)
				c.expectError(t, tc.want)
				if tc.event == "" {
					c.expectNoEvents(t)
				} else {
					attrs := map[string]string{"reason": tc.reason}
					if tc.event == "oauth.credentials.rejected" {
						attrs["client_name"], attrs["client_id_prefix"] = name, prefix
					}
					c.expectEvent(t, tc.event, attrs)
				}
				expectAbsent(t, c.raw, "owner@example.com", "synthetic-password", "evil.example", out.RequestID, "no-such-request")
			})
		}

		deny := url.Values{"request": {out.RequestID}, "decision": {"deny"}}
		postForm(h, "/oauth/write", deny, cookie, testConfig.PublicURL)
		c = sink.one(t)
		c.expectRequest(t, "POST /oauth/write", 200, "none", "not_required")
		c.expectError(t, wantError{code: "access_denied", reason: "user_denied"})
		c.expectClient(t, name, prefix)

		// Revoking the grant that prepared a write closes its approval page.
		out = f.call(t, "create_manual_transaction", map[string]any{"data": f.manual(), "idempotency_key": "synthetic-key-2"})
		if _, err := conn.Exec(`DELETE FROM oauth_grants WHERE token_hash=?`, tokenHash(f.token)); err != nil {
			t.Fatal(err)
		}
		u, _ = url.Parse(out.ApprovalURL)
		if w = oauthRequest(h, "GET", u.RequestURI(), "", nil, ""); w.Code != 403 {
			t.Fatalf("revoked grant: %d", w.Code)
		}
		c = sink.one(t)
		c.expectRequest(t, "GET /oauth/write", 403, "none", "not_required")
		c.expectError(t, wantError{code: "access_denied", reason: "grant_inactive"})
	})
}
