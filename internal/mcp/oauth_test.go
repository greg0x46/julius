package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/settings"
)

type oauthFlow struct {
	handler          http.Handler
	client, verifier string
	params           url.Values
	cookie           *http.Cookie
	pending          string
}

func oauthRequest(h http.Handler, method, path, body string, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if method == "POST" {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func newFlow(t *testing.T, store *auth.Store) *oauthFlow {
	t.Helper()
	return newFlowWithRedirect(t, store, "http://127.0.0.1:45678/callback")
}
func newFlowWithRedirect(t *testing.T, store *auth.Store, redirect string) *oauthFlow {
	t.Helper()
	h, err := NewHandler(true, testConfig, store)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := json.Marshal(clientMetadata{Name: "Synthetic client <script>", Redirects: []string{redirect}, Method: "none"})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/oauth/register", strings.NewReader(string(metadata)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var c clientMetadata
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &c) != nil {
		t.Fatalf("register: %d %s", w.Code, w.Body)
	}
	f := &oauthFlow{handler: h, client: c.ID, verifier: strings.Repeat("v", 43)}
	f.params = url.Values{"client_id": {c.ID}, "redirect_uri": {c.Redirects[0]}, "response_type": {"code"}, "resource": {testConfig.PublicURL + "/mcp"}, "scope": {mcpScope}, "code_challenge": {pkce(f.verifier)}, "code_challenge_method": {"S256"}, "state": {"opaque-state"}}
	return f
}
func (f *oauthFlow) begin(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	w := oauthRequest(f.handler, "GET", "/oauth/authorize?"+f.params.Encode(), "", nil, "")
	if w.Code != 200 {
		t.Fatalf("authorize: %d %s", w.Code, w.Body)
	}
	found := regexp.MustCompile(`name="request" value="([^"]+)"`).FindStringSubmatch(w.Body.String())
	if len(found) != 2 {
		t.Fatal("missing consent request")
	}
	f.pending = found[1]
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("missing CSRF cookie")
	}
	f.cookie = cookies[0]
	return w
}
func (f *oauthFlow) approve(t *testing.T) string {
	t.Helper()
	f.begin(t)
	p := url.Values{"request": {f.pending}, "decision": {"allow"}, "email": {"owner@example.com"}, "password": {"synthetic-password"}}
	w := oauthRequest(f.handler, "POST", "/oauth/authorize", p.Encode(), f.cookie, testConfig.PublicURL)
	u := consentCallback(t, w)
	if u.Query().Get("state") != "opaque-state" || u.Query().Get("iss") != testConfig.PublicURL || u.Query().Get("code") == "" {
		t.Fatalf("redirect: %s", u)
	}
	return u.Query().Get("code")
}
func consentCallback(t *testing.T, w *httptest.ResponseRecorder) *url.URL {
	t.Helper()
	if w.Code != http.StatusOK || w.Header().Get("Location") != "" {
		t.Fatalf("consent must navigate from a fresh document: %d", w.Code)
	}
	body := w.Body.String()
	refresh := regexp.MustCompile(`http-equiv="refresh" content="0;url=([^"]+)"`).FindStringSubmatch(body)
	link := regexp.MustCompile(`<a href="([^"]+)">`).FindStringSubmatch(body)
	if len(refresh) != 2 || len(link) != 2 || html.UnescapeString(refresh[1]) != html.UnescapeString(link[1]) {
		t.Fatalf("missing or mismatched callback navigation: %s", body)
	}
	if strings.Contains(body, "<form") || strings.Contains(body, "synthetic-password") || strings.Contains(body, "owner@example.com") {
		t.Fatal("continuation contains credentials or a form")
	}
	for _, directive := range []string{"default-src 'none'", "form-action 'self'", "frame-ancestors 'none'", "base-uri 'none'"} {
		if !strings.Contains(w.Header().Get("Content-Security-Policy"), directive) {
			t.Fatalf("missing CSP directive %q", directive)
		}
	}
	if w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("missing callback protection headers")
	}
	u, err := url.Parse(html.UnescapeString(link[1]))
	if err != nil {
		t.Fatal(err)
	}
	return u
}
func (f *oauthFlow) exchange(code string) url.Values {
	return url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {f.client}, "redirect_uri": {f.params.Get("redirect_uri")}, "resource": {f.params.Get("resource")}, "code_verifier": {f.verifier}}
}
func (f *oauthFlow) token(t *testing.T, code string) string {
	t.Helper()
	w := oauthRequest(f.handler, "POST", "/oauth/token", f.exchange(code).Encode(), nil, "")
	var result struct {
		Token   string `json:"access_token"`
		Scope   string `json:"scope"`
		Expires int    `json:"expires_in"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Token == "" || result.Scope != mcpScope || result.Expires != 3600 {
		t.Fatalf("token: %d %s", w.Code, w.Body)
	}
	return result.Token
}
func oauthToken(t *testing.T, store *auth.Store) string {
	t.Helper()
	f := newFlow(t, store)
	return f.token(t, f.approve(t))
}

func TestOAuthFlow(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, _ := setup(t, conn)
		f := newFlow(t, store)
		for _, path := range []string{"/.well-known/oauth-authorization-server", "/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
			w := oauthRequest(f.handler, "GET", path, "", nil, "")
			if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || !json.Valid(w.Body.Bytes()) {
				t.Fatalf("metadata: %d %s", w.Code, w.Body)
			}
		}
		code := f.approve(t)
		token := f.token(t, code)
		if w := request(t, f.handler, token, "", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"get_investment_portfolio"`) {
			t.Fatalf("tools: %d %s", w.Code, w.Body)
		}
		if w := oauthRequest(f.handler, "POST", "/oauth/token", f.exchange(code).Encode(), nil, ""); w.Code != 400 {
			t.Fatalf("code reused: %d", w.Code)
		}
		var raw string
		if err := conn.QueryRow(`SELECT token_hash FROM oauth_grants WHERE token_hash=?`, tokenHash(token)).Scan(&raw); err != nil || raw == token {
			t.Fatal("token not hashed", err)
		}
		// A new handler uses persisted grants after restart.
		restarted, _ := NewHandler(true, testConfig, store)
		if w := request(t, restarted, token, "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`); w.Code != 200 {
			t.Fatalf("restart: %d %s", w.Code, w.Body)
		}
		p := url.Values{"client_id": {f.client}, "token": {token}}
		for i := 0; i < 2; i++ {
			if w := oauthRequest(f.handler, "POST", "/oauth/revoke", p.Encode(), nil, ""); w.Code != 200 {
				t.Fatalf("revocation: %d", w.Code)
			}
		}
		if w := request(t, f.handler, token, "", `{}`); w.Code != 401 {
			t.Fatalf("revoked: %d", w.Code)
		}
	})
}
func TestOAuthRejectsInvalidGrants(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, _ := setup(t, conn)
		for _, field := range []string{"code_verifier", "client_id", "redirect_uri", "resource", "expired", "reset"} {
			t.Run(field, func(t *testing.T) {
				f := newFlow(t, store)
				code := f.approve(t)
				p := f.exchange(code)
				switch field {
				case "expired":
					if _, err := conn.Exec(`UPDATE oauth_grants SET expires_at=? WHERE token_hash=?`, db.FormatTime(time.Now().Add(-time.Minute)), tokenHash(code)); err != nil {
						t.Fatal(err)
					}
				case "reset":
					if err := store.ResetPassword(context.Background(), "synthetic-password"); err != nil {
						t.Fatal(err)
					}
				default:
					p.Set(field, "wrong")
				}
				w := oauthRequest(f.handler, "POST", "/oauth/token", p.Encode(), nil, "")
				if w.Code != 400 {
					t.Fatalf("accepted %s: %d %s", field, w.Code, w.Body)
				}
			})
		}
	})
}
func TestOAuthConsentCallbackNavigation(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, _ := setup(t, conn)
		for _, redirect := range []string{
			"http://127.0.0.1:45678/callback",
			"https://client.example/callback?existing=one&quoted=%22%3E%3Cscript%3E",
		} {
			for _, decision := range []string{"allow", "deny"} {
				f := newFlowWithRedirect(t, store, redirect)
				state := `opaque "quoted" <script>& state`
				f.params.Set("state", state)
				consent := f.begin(t)
				if !strings.Contains(consent.Body.String(), `<form action="/oauth/authorize" method="post">`) {
					t.Fatal("credential form must post to the fixed same-origin endpoint")
				}
				p := url.Values{"request": {f.pending}, "decision": {decision}, "email": {"owner@example.com"}, "password": {"synthetic-password"}}
				w := oauthRequest(f.handler, "POST", "/oauth/authorize", p.Encode(), f.cookie, testConfig.PublicURL)
				u := consentCallback(t, w)
				original, _ := url.Parse(redirect)
				if u.Scheme != original.Scheme || u.Host != original.Host || u.Path != original.Path || u.Query().Get("existing") != original.Query().Get("existing") || u.Query().Get("quoted") != original.Query().Get("quoted") || u.Query().Get("state") != state || u.Query().Get("iss") != testConfig.PublicURL {
					t.Fatalf("callback parameters changed: %s", u)
				}
				if strings.Contains(w.Body.String(), "<script>") || !strings.Contains(w.Body.String(), "&amp;") {
					t.Fatal("callback must be escaped in HTML attributes")
				}
				if decision == "allow" {
					if u.Query().Get("code") == "" || u.Query().Get("error") != "" {
						t.Fatalf("missing authorization code: %s", u)
					}
					f.token(t, u.Query().Get("code"))
				} else if u.Query().Get("error") != "access_denied" || u.Query().Get("code") != "" {
					t.Fatalf("invalid denial: %s", u)
				}
			}
		}
	})
}
func TestOAuthConsentBoundaries(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, _ := setup(t, conn)
		for _, field := range []string{"client_id", "redirect_uri", "resource", "scope", "code_challenge_method", "code_challenge", "response_type"} {
			f := newFlow(t, store)
			f.params.Set(field, "invalid")
			w := oauthRequest(f.handler, "GET", "/oauth/authorize?"+f.params.Encode(), "", nil, "")
			want := 303
			if field == "client_id" || field == "redirect_uri" {
				want = 400
			}
			if w.Code != want {
				t.Fatalf("invalid %s: %d", field, w.Code)
			}
			if want == 400 && w.Header().Get("Location") != "" {
				t.Fatal("redirected to unvalidated client")
			}
			if want == 303 {
				u, _ := url.Parse(w.Header().Get("Location"))
				if u.Query().Get("error") == "" {
					t.Fatal("missing authorization error")
				}
			}
		}
		f := newFlow(t, store)
		w := f.begin(t)
		if strings.Contains(w.Body.String(), "<script>") || !strings.Contains(w.Body.String(), "&lt;script&gt;") || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.Fatal("unsafe consent HTML")
		}
		p := url.Values{"request": {f.pending}, "decision": {"allow"}, "email": {"owner@example.com"}, "password": {"synthetic-password"}}
		for _, tc := range []struct {
			cookie *http.Cookie
			origin string
		}{{nil, testConfig.PublicURL}, {f.cookie, "https://evil.example"}, {f.cookie, ""}, {&http.Cookie{Name: f.cookie.Name, Value: "forged"}, testConfig.PublicURL}} {
			if w := oauthRequest(f.handler, "POST", "/oauth/authorize", p.Encode(), tc.cookie, tc.origin); w.Code != 403 {
				t.Fatalf("CSRF accepted: %d", w.Code)
			}
		}
		p.Set("password", "wrong")
		if w := oauthRequest(f.handler, "POST", "/oauth/authorize", p.Encode(), f.cookie, testConfig.PublicURL); w.Code != 401 || !strings.Contains(w.Body.String(), "role=\"alert\"") {
			t.Fatalf("wrong password: %d %s", w.Code, w.Body)
		}
		p.Set("decision", "deny")
		p.Del("email")
		p.Del("password")
		w = oauthRequest(f.handler, "POST", "/oauth/authorize", p.Encode(), f.cookie, testConfig.PublicURL)
		u := consentCallback(t, w)
		if u.Query().Get("error") != "access_denied" || u.Query().Get("code") != "" || u.Query().Get("state") != "opaque-state" || u.Query().Get("iss") != testConfig.PublicURL {
			t.Fatalf("deny: %s", u)
		}
		if w := oauthRequest(f.handler, "POST", "/oauth/authorize", p.Encode(), f.cookie, testConfig.PublicURL); w.Code != 400 {
			t.Fatalf("consent reused: %d", w.Code)
		}
		f.begin(t)
		if _, err := conn.Exec(`UPDATE oauth_grants SET expires_at=? WHERE token_hash=?`, db.FormatTime(time.Now().Add(-time.Minute)), tokenHash(f.pending)); err != nil {
			t.Fatal(err)
		}
		p.Set("request", f.pending)
		if w := oauthRequest(f.handler, "POST", "/oauth/authorize", p.Encode(), f.cookie, testConfig.PublicURL); w.Code != 400 {
			t.Fatalf("expired consent: %d", w.Code)
		}
		if err := settings.SetAuthenticationEnabled(context.Background(), conn, false); err != nil {
			t.Fatal(err)
		}
		if w := oauthRequest(f.handler, "GET", "/oauth/authorize?"+f.params.Encode(), "", nil, ""); w.Code != 403 {
			t.Fatalf("open mode consent: %d", w.Code)
		}
	})
}
func TestOAuthTokenBoundaries(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, token := setup(t, conn)
		handler, _ := NewHandler(true, testConfig, store)
		browser, err := store.Login(context.Background(), "owner@example.com", "synthetic-password")
		if err != nil {
			t.Fatal(err)
		}
		for _, method := range []string{"tools/list", "tools/call"} {
			if w := request(t, handler, browser, "", `{"jsonrpc":"2.0","id":1,"method":"`+method+`"}`); w.Code != 401 {
				t.Fatalf("browser token accepted: %d", w.Code)
			}
			if w := request(t, handler, "", "", `{"jsonrpc":"2.0","id":1,"method":"`+method+`"}`); w.Code != 401 {
				t.Fatalf("anonymous tool accepted: %d", w.Code)
			}
		}
		if _, err := store.Authenticate(context.Background(), token); !errors.Is(err, auth.ErrSession) {
			t.Fatal("OAuth token accepted as browser session", err)
		}
		for _, tc := range []struct {
			column, value string
			status        int
		}{{"resource", "https://other.example/mcp", 401}, {"scope", "", 403}, {"expires_at", db.FormatTime(time.Now().Add(-time.Minute)), 401}} {
			fresh := oauthToken(t, store)
			if _, err := conn.Exec(`UPDATE oauth_grants SET `+tc.column+`=? WHERE token_hash=?`, tc.value, tokenHash(fresh)); err != nil {
				t.Fatal(err)
			}
			w := request(t, handler, fresh, "", `{}`)
			if w.Code != tc.status {
				t.Fatalf("%s: %d %s", tc.column, w.Code, w.Body)
			}
			if tc.status == 403 && !strings.Contains(w.Header().Get("WWW-Authenticate"), "insufficient_scope") {
				t.Fatal("missing scope challenge")
			}
		}
		if err := store.ResetPassword(context.Background(), "synthetic-password"); err != nil {
			t.Fatal(err)
		}
		if w := request(t, handler, token, "", `{}`); w.Code != 401 {
			t.Fatalf("reset token: %d", w.Code)
		}
	})
}

func TestOAuthCannotAuthorizeREST(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		_, token := setup(t, conn)
		api := httpapi.NewServer(conn, nil, nil, testConfig, nil)
		for _, csrf := range []bool{false, true} {
			r := httptest.NewRequest("POST", "/api/categories", strings.NewReader(`{"name":"Synthetic"}`))
			r.Header.Set("Authorization", "Bearer "+token)
			if csrf {
				r.Header.Set("Origin", testConfig.PublicURL)
				r.Header.Set("X-Julius-Request", "1")
			}
			w := httptest.NewRecorder()
			api.ServeHTTP(w, r)
			want := 403
			if csrf {
				want = 401
			}
			if w.Code != want {
				t.Fatalf("REST bearer with CSRF=%t: %d %s", csrf, w.Code, w.Body)
			}
		}
	})
}
func TestOAuthRejectsRegistrationAndAbuse(t *testing.T) {
	store, _ := setup(t, dbtest.OpenSQLite(t))
	f := newFlow(t, store)
	for _, body := range []string{
		`{}`, `{"redirect_uris":["https://evil.example/#fragment"]}`,
		`{"redirect_uris":["http://public.example/callback"]}`,
		`{"redirect_uris":["https://user:pass@example.com/callback"]}`,
		`{"redirect_uris":["https://example.com/callback"],"token_endpoint_auth_method":"client_secret_basic"}`,
		`{"redirect_uris":["https://example.com/callback"],"grant_types":["implicit"]}`,
		`{"redirect_uris":["https://example.com/callback"]} {}`,
	} {
		r := httptest.NewRequest("POST", "/oauth/register", strings.NewReader(body))
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatalf("registration %s: %d", body, w.Code)
		}
	}
	f.begin(t)
	p := url.Values{"request": {f.pending}, "decision": {"allow"}, "email": {"owner@example.com"}, "password": {"wrong"}}
	for i := 0; i < 6; i++ {
		w := oauthRequest(f.handler, "POST", "/oauth/authorize", p.Encode(), f.cookie, testConfig.PublicURL)
		want := 401
		if i == 5 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("attempt %d: %d", i, w.Code)
		}
	}
}
func TestOAuthCodeExchangeIsAtomic(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, _ := setup(t, conn)
		f := newFlow(t, store)
		code := f.approve(t)
		results := make(chan int, 2)
		for i := 0; i < 2; i++ {
			go func() {
				results <- oauthRequest(f.handler, "POST", "/oauth/token", f.exchange(code).Encode(), nil, "").Code
			}()
		}
		a, b := <-results, <-results
		if !((a == 200 && b == 400) || (a == 400 && b == 200)) {
			t.Fatalf("concurrent exchange: %d %d", a, b)
		}
	})
}
