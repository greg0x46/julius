package mcp

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/greg0x46/julius/internal/db/dbtest"
)

// browserFormOrigin is the Origin a browser sends when a page of origin
// pageOrigin submits a same-origin POST form under the given Referrer-Policy
// (Fetch, "append a request Origin header"): no-referrer yields "null".
func browserFormOrigin(policy, pageOrigin string) string {
	if policy == "no-referrer" {
		return "null"
	}
	return pageOrigin
}

func TestOAuthPagesKeepTheFormOrigin(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, _ := setup(t, conn)

		t.Run("consent", func(t *testing.T) {
			f := newFlow(t, store)
			page := f.begin(t)
			// Fails if the page goes back to no-referrer: the browser would then
			// submit "Origin: null" and the exact Origin check would reject it.
			policy := page.Header().Get("Referrer-Policy")
			if policy != "same-origin" {
				t.Fatalf("Referrer-Policy = %q, want same-origin", policy)
			}
			form := url.Values{"request": {f.pending}, "decision": {"allow"}, "email": {"owner@example.com"}, "password": {"synthetic-password"}, "scope": {mcpScope}}
			origin := browserFormOrigin(policy, testConfig.PublicURL)
			if origin != testConfig.PublicURL {
				t.Fatalf("a browser would send Origin %q", origin)
			}
			w := oauthRequest(f.handler, "POST", "/oauth/authorize", form.Encode(), f.cookie, origin)
			if u := consentCallback(t, w); u.Query().Get("code") == "" {
				t.Fatalf("no code in %s", u)
			}
		})

		t.Run("null origin stays rejected", func(t *testing.T) {
			f := newFlow(t, store)
			f.begin(t)
			form := url.Values{"request": {f.pending}, "decision": {"allow"}, "email": {"owner@example.com"}, "password": {"synthetic-password"}, "scope": {mcpScope}}
			w := oauthRequest(f.handler, "POST", "/oauth/authorize", form.Encode(), f.cookie, "null")
			if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "access_denied") {
				t.Fatalf("null origin: %d %s", w.Code, w.Body)
			}
			if strings.Contains(w.Body.String(), "refresh") {
				t.Fatal("null origin reached the callback")
			}
			// The pending request is untouched: the same form still works with the real origin.
			w = oauthRequest(f.handler, "POST", "/oauth/authorize", form.Encode(), f.cookie, testConfig.PublicURL)
			consentCallback(t, w)
		})

		t.Run("callback page does not leak to other sites", func(t *testing.T) {
			f := newFlow(t, store)
			f.begin(t)
			form := url.Values{"request": {f.pending}, "decision": {"deny"}}
			w := oauthRequest(f.handler, "POST", "/oauth/authorize", form.Encode(), f.cookie, testConfig.PublicURL)
			// same-origin sends a Referer only to this origin; the redirect_uri is another one.
			if got := w.Header().Get("Referrer-Policy"); got != "same-origin" {
				t.Fatalf("callback Referrer-Policy = %q", got)
			}
		})
	})
}

func TestWriteApprovalFormKeepsTheOrigin(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		f := newWriteFixture(t, conn)
		out := f.call(t, "create_manual_transaction", map[string]any{"data": f.manual(), "idempotency_key": "synthetic-key"})
		u, err := url.Parse(out.ApprovalURL)
		if err != nil {
			t.Fatal(err)
		}
		page := oauthRequest(f.handler, "GET", u.RequestURI(), "", nil, "")
		policy := page.Header().Get("Referrer-Policy")
		if page.Code != 200 || policy != "same-origin" {
			t.Fatalf("approval page: %d, Referrer-Policy %q", page.Code, policy)
		}
		cookie := page.Result().Cookies()[0]
		form := url.Values{"request": {out.RequestID}, "decision": {"allow"}, "email": {"owner@example.com"}, "password": {"synthetic-password"}}
		if w := oauthRequest(f.handler, "POST", "/oauth/write", form.Encode(), cookie, "null"); w.Code != http.StatusForbidden {
			t.Fatalf("null origin accepted: %d", w.Code)
		}
		w := oauthRequest(f.handler, "POST", "/oauth/write", form.Encode(), cookie, browserFormOrigin(policy, testConfig.PublicURL))
		if w.Code != 200 {
			t.Fatalf("approval with the browser origin: %d %s", w.Code, w.Body)
		}
	})
}

func TestInvalidOriginReasons(t *testing.T) {
	for _, tc := range []struct{ origin, want string }{
		{"", "origin_missing"},
		{"null", "origin_null"},
		{"https://other.example", "origin_mismatch"},
	} {
		r := httptest.NewRequest("POST", "/oauth/authorize", nil)
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		if got := invalidOrigin(r); got != tc.want {
			t.Errorf("Origin %q: reason %q, want %q", tc.origin, got, tc.want)
		}
	}
}
