package mcp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db/dbtest"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) advance(d time.Duration) { c.now = c.now.Add(d) }

func consentAttempt(h http.Handler, f *oauthFlow, password string) *httptest.ResponseRecorder {
	form := url.Values{"request": {f.pending}, "decision": {"allow"}, "email": {"owner@example.com"}, "password": {password}}
	return postForm(h, "/oauth/authorize", form, f.cookie, testConfig.PublicURL)
}

// Only a failed owner credential check spends the budget: any number of
// successful consents leaves it intact, and once five failures are recorded
// even the right password gets a 429 until the window passes.
func TestConsentOnlyFailuresSpendTheCredentialBudget(t *testing.T) {
	store, _ := setup(t, dbtest.OpenSQLite(t))
	o := &oauthServer{store: store, config: testConfig, hashing: make(chan struct{}, 1)}
	o.limiters()
	clock := &fakeClock{now: time.Now()}
	o.credentials.Now = clock.Now
	h, sink := withLogging(o.routes(http.NotFoundHandler()))
	newPending := func(i int) *oauthFlow {
		f := flowOn(t, h, fmt.Sprintf("Synthetic assistant %d", i), syntheticRedirect)
		f.begin(t)
		return f
	}

	for i := 0; i < 8; i++ {
		f := newPending(i)
		consentCallback(t, consentAttempt(h, f, "synthetic-password"))
	}

	first := newPending(8)
	for i := 0; i < 4; i++ {
		if w := consentAttempt(h, first, "wrong"); w.Code != 401 {
			t.Fatalf("wrong password %d: %d", i, w.Code)
		}
	}
	consentCallback(t, consentAttempt(h, first, "synthetic-password"))

	second := newPending(9)
	if w := consentAttempt(h, second, "wrong"); w.Code != 401 {
		t.Fatalf("fifth failure: %d", w.Code)
	}
	sink.drain()
	w := consentAttempt(h, second, "synthetic-password")
	expectOAuthError(t, w, 429, "temporarily_unavailable")
	if w.Header().Get("Retry-After") != "60" {
		t.Fatalf("Retry-After = %q", w.Header().Get("Retry-After"))
	}
	c := sink.one(t)
	c.expectError(t, wantError{code: "temporarily_unavailable", reason: "rate_limited"})
	c.expectEvent(t, "security.rate_limited", map[string]string{"limiter": "oauth_consent_attempts", "reason": "window_exceeded"})

	clock.advance(time.Minute + time.Second)
	consentCallback(t, consentAttempt(h, second, "synthetic-password"))
}

// A write approval draws on the same failure budget as the consent page, and a
// successful approval does not spend it.
func TestWriteApprovalSharesTheCredentialBudgetAndSuccessIsFree(t *testing.T) {
	f := newWriteFixture(t, dbtest.OpenSQLite(t))
	for i := 0; i < 6; i++ {
		out := f.call(t, "create_manual_transaction", map[string]any{"data": f.manual(), "idempotency_key": fmt.Sprintf("synthetic-key-%d", i)})
		if w := f.approve(t, out); w.Code != 200 {
			t.Fatalf("approval %d: %d", i, w.Code)
		}
	}

	out := f.call(t, "create_manual_transaction", map[string]any{"data": f.manual(), "idempotency_key": "synthetic-key-last"})
	u, _ := url.Parse(out.ApprovalURL)
	cookie := oauthRequest(f.handler, "GET", u.RequestURI(), "", nil, "").Result().Cookies()[0]
	form := url.Values{"request": {out.RequestID}, "decision": {"allow"}, "email": {"owner@example.com"}, "password": {"wrong"}}
	for i := 0; i < 5; i++ {
		if w := oauthRequest(f.handler, "POST", "/oauth/write", form.Encode(), cookie, testConfig.PublicURL); w.Code != 401 {
			t.Fatalf("wrong password %d: %d", i, w.Code)
		}
	}
	form.Set("password", "synthetic-password")
	w := oauthRequest(f.handler, "POST", "/oauth/write", form.Encode(), cookie, testConfig.PublicURL)
	expectOAuthError(t, w, 429, "temporarily_unavailable")
	if w.Header().Get("Retry-After") != "60" {
		t.Fatalf("Retry-After = %q", w.Header().Get("Retry-After"))
	}

	g := flowOn(t, f.handler, "Second assistant", syntheticRedirect)
	g.begin(t)
	expectOAuthError(t, consentAttempt(f.handler, g, "synthetic-password"), 429, "temporarily_unavailable")
}
