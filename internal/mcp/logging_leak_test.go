package mcp

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"unicode"

	"github.com/greg0x46/julius/internal/db/dbtest"
)

// TestNothingSensitiveReachesTheLog drives hostile and sensitive input through
// every endpoint and checks that none of it appears in the log output.
func TestNothingSensitiveReachesTheLog(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		store, _ := setup(t, conn)
		h, sink := newLogged(t, store)
		var all strings.Builder
		const (
			evilName  = "Evil\r\n{\"event\":\"forged\",\"level\":\"ERROR\"}\u2028\x1b[31m\u0085\u202e client"
			redirect  = "https://secret-client.example/callback/with/path?token=secretquery"
			state     = "state-secret-123"
			secretKey = "client-secret-xyz"
			badPass   = "wrong-password-xyz"
			queryMark = "secret-query-marker"
		)
		f := flowOn(t, h, evilName, redirect)
		f.params.Set("state", state)

		// Authorization request carrying a junk query parameter.
		if w := oauthRequest(h, "GET", "/oauth/authorize?"+f.params.Encode()+"&q="+queryMark, "", nil, ""); w.Code != 200 {
			t.Fatalf("authorize: %d", w.Code)
		}
		f.begin(t)
		csrf := f.cookie.Value
		wrong := url.Values{"request": {f.pending}, "decision": {"allow"}, "email": {"someone@example.org"}, "password": {badPass}}
		postForm(h, "/oauth/authorize", wrong, f.cookie, testConfig.PublicURL)
		postForm(h, "/oauth/authorize", wrong, &http.Cookie{Name: f.cookie.Name, Value: "forged-csrf-cookie"}, testConfig.PublicURL)
		postForm(h, "/oauth/authorize", wrong, f.cookie, "https://origin-secret.example")
		allow := url.Values{"request": {f.pending}, "decision": {"allow"}, "email": {"owner@example.com"}, "password": {"synthetic-password"}}
		code := consentCallback(t, postForm(h, "/oauth/authorize", allow, f.cookie, testConfig.PublicURL)).Query().Get("code")

		// A failing exchange spends the code; later ones carry credentials.
		bad := f.exchange(code)
		bad.Set("code_verifier", strings.Repeat("z", 43))
		postForm(h, "/oauth/token", bad, nil, "")
		f.begin(t)
		allow.Set("request", f.pending)
		code2 := consentCallback(t, postForm(h, "/oauth/authorize", allow, f.cookie, testConfig.PublicURL)).Query().Get("code")
		withSecret := f.exchange(code2)
		withSecret.Set("client_secret", secretKey)
		postForm(h, "/oauth/token", withSecret, nil, "")
		r := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(f.exchange(code2).Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Authorization", "Basic YWJjOmRlZg==")
		h.ServeHTTP(httptest.NewRecorder(), r)
		token := f.token(t, code2)

		// The MCP gate with good, bad and hostile credentials.
		request(t, h, token, "", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
		request(t, h, "bad-bearer-value", "", `{}`)
		request(t, h, token, "https://origin-secret.example", `{}`)
		request(t, h, "", "", `{}`)

		// Revocation with a secret, then for real.
		postForm(h, "/oauth/revoke", url.Values{"client_id": {f.client}, "token": {token}, "client_secret": {secretKey}}, nil, "")
		postForm(h, "/oauth/revoke", url.Values{"client_id": {f.client}, "token": {token}}, nil, "")

		// A rejected registration that echoes hostile text everywhere it can.
		jsonRequest(h, "POST", "/oauth/register", `{"client_name":"`+strings.Repeat("N", 300)+`","redirect_uris":["http://hostile-redirect.example/cb"],"grant_types":["x\r\n{\"event\":\"forged\"}"]}`, nil)
		jsonRequest(h, "POST", "/oauth/register", `{"client_name":"x","redirect_uris":["http://hostile-redirect.example/cb"]}`, nil)

		all.WriteString(sink.drain())
		raw := all.String()

		secrets := []string{
			"owner@example.com", "someone@example.org", "synthetic-password", badPass,
			code, code2, state, f.verifier, strings.Repeat("z", 43), pkce(f.verifier),
			token, csrf, "forged-csrf-cookie", "Bearer", "Basic", "YWJjOmRlZg==", "bad-bearer-value", secretKey,
			"secret-client.example", "secretquery", "/callback/with/path", queryMark,
			"origin-secret.example", "hostile-redirect.example", f.client, strings.Repeat("N", 120),
		}
		expectAbsent(t, raw, secrets...)

		allowed := map[string]bool{"http_request": true, "oauth.client.registered": true, "oauth.consent.granted": true, "oauth.credentials.rejected": true, "oauth.token.revoked": true, "security.invalid_origin": true}
		lines := parseLines(t, raw)
		if len(lines) < 20 {
			t.Fatalf("expected a log line per request and event, got %d", len(lines))
		}
		for _, m := range lines {
			if !allowed[m["event"].(string)] {
				t.Fatalf("unexpected event in %v", m)
			}
			if m["event"] != "http_request" && m["level"] == "ERROR" {
				t.Fatalf("forged level: %v", m)
			}
			// Hostile client names are sanitized: one line, no control or odd spaces.
			if name, ok := m["client_name"].(string); ok {
				if !strings.Contains(name, "Evil") || len(name) > 100 {
					t.Fatalf("client_name = %q", name)
				}
				for _, r := range name {
					if unicode.IsControl(r) || (unicode.IsSpace(r) && r != ' ') {
						t.Fatalf("client_name keeps %U: %q", r, name)
					}
				}
			}
		}
		if strings.Count(raw, "\n") != len(lines) {
			t.Fatal("a record spans several lines")
		}
	})
}
