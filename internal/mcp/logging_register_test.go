package mcp

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/greg0x46/julius/internal/db/dbtest"
)

const validRedirectField = `"redirect_uris":["https://client.example/callback"]`

func TestRegisterRejectionsAreDiagnosedInTheAccessLine(t *testing.T) {
	store, _ := setup(t, dbtest.OpenSQLite(t))
	h, sink := newLogged(t, store)
	meta := func(extra string) string { return `{` + validRedirectField + `,` + extra + `}` }
	manyURIs := `["` + strings.Repeat(`https://client.example/cb","`, 10) + `https://client.example/cb"]`
	longURI := "https://client.example/" + strings.Repeat("a", 2100)

	tests := []struct {
		name   string
		body   string
		reader io.Reader // replaces body when set
		public string    // public error code
		want   wantError
		absent []string // client text that must never reach the log
	}{
		{name: "empty body", body: ``, want: wantError{reason: "empty_body"}},
		{name: "blank body", body: "  \n", want: wantError{reason: "empty_body"}},
		{name: "malformed JSON", body: `not json`, want: wantError{reason: "invalid_json"}, absent: []string{"not json"}},
		{name: "truncated JSON", body: `{"client_name":`, want: wantError{reason: "invalid_json"}},
		{name: "trailing garbage", body: meta(`"client_name":"x"`) + ` trailing`, want: wantError{reason: "invalid_json"}, absent: []string{"trailing"}},
		{name: "body read error", reader: iotest.ErrReader(errors.New("boom")), want: wantError{reason: "body_read_error"}},
		{name: "oversized body", body: `{"client_name":"` + strings.Repeat("a", 20<<10) + `",` + validRedirectField + `}`, want: wantError{reason: "body_too_large"}},
		{name: "two JSON values", body: meta(`"client_name":"x"`) + ` {}`, want: wantError{reason: "multiple_json_values"}},
		{name: "top-level array", body: `[]`, want: wantError{reason: "invalid_field_type", field: "body"}},
		{name: "client_name type", body: `{"client_name":5,` + validRedirectField + `}`, want: wantError{reason: "invalid_field_type", field: "client_name"}},
		{name: "redirect_uris type", body: `{"redirect_uris":"https://client.example/cb"}`, want: wantError{reason: "invalid_field_type", field: "redirect_uris"}},
		{name: "redirect_uris element type", body: `{"redirect_uris":[1]}`, want: wantError{reason: "invalid_field_type", field: "redirect_uris"}},
		{name: "auth method type", body: meta(`"token_endpoint_auth_method":true`), want: wantError{reason: "invalid_field_type", field: "token_endpoint_auth_method"}},
		{name: "grant_types type", body: meta(`"grant_types":"authorization_code"`), want: wantError{reason: "invalid_field_type", field: "grant_types"}},
		{name: "response_types type", body: meta(`"response_types":{}`), want: wantError{reason: "invalid_field_type", field: "response_types"}},
		{name: "client_id type", body: meta(`"client_id":7`), want: wantError{reason: "invalid_field_type", field: "client_id"}},
		{name: "client_id provided", body: meta(`"client_id":"chosen-by-client"`), want: wantError{reason: "client_id_provided", field: "client_id"}, absent: []string{"chosen-by-client"}},
		{name: "name too long", body: meta(`"client_name":"` + strings.Repeat("n", 201) + `"`), want: wantError{reason: "name_too_long", field: "client_name"}},
		{name: "no redirect_uris", body: `{}`, want: wantError{reason: "redirect_uris_missing", field: "redirect_uris"}},
		{name: "empty redirect_uris", body: `{"redirect_uris":[]}`, want: wantError{reason: "redirect_uris_missing", field: "redirect_uris"}},
		{name: "too many redirect_uris", body: `{"redirect_uris":` + manyURIs + `}`, want: wantError{reason: "too_many_redirect_uris", field: "redirect_uris"}},
		{name: "auth method client_secret_post", body: meta(`"token_endpoint_auth_method":"client_secret_post"`), want: wantError{reason: "unsupported_auth_method", field: "token_endpoint_auth_method", values: []string{"client_secret_post"}}},
		{name: "auth method private_key_jwt", body: meta(`"token_endpoint_auth_method":"private_key_jwt"`), want: wantError{reason: "unsupported_auth_method", field: "token_endpoint_auth_method", values: []string{"private_key_jwt"}}},
		{name: "auth method unknown", body: meta(`"token_endpoint_auth_method":"<script>"`), want: wantError{reason: "unsupported_auth_method", field: "token_endpoint_auth_method", values: []string{"other"}}, absent: []string{"script"}},
		{name: "grants refresh_token without authorization_code", body: meta(`"grant_types":["refresh_token"]`), want: wantError{reason: "unsupported_grant_types", field: "grant_types", values: []string{"refresh_token"}}},
		{name: "grants refresh_token repeated", body: meta(`"grant_types":["refresh_token","refresh_token"]`), want: wantError{reason: "unsupported_grant_types", field: "grant_types", values: []string{"refresh_token", "refresh_token"}}},
		{name: "grants with implicit", body: meta(`"grant_types":["authorization_code","refresh_token","implicit"]`), want: wantError{reason: "unsupported_grant_types", field: "grant_types", values: []string{"authorization_code", "refresh_token", "implicit"}}},
		{name: "grant unknown", body: meta(`"grant_types":["bogus-grant"]`), want: wantError{reason: "unsupported_grant_types", field: "grant_types", values: []string{"other"}}, absent: []string{"bogus"}},
		{name: "grant implicit", body: meta(`"grant_types":["implicit"]`), want: wantError{reason: "unsupported_grant_types", field: "grant_types", values: []string{"implicit"}}},
		{name: "grant device code", body: meta(`"grant_types":["urn:ietf:params:oauth:grant-type:device_code"]`), want: wantError{reason: "unsupported_grant_types", field: "grant_types", values: []string{"urn:ietf:params:oauth:grant-type:device_code"}}},
		{name: "grant repeated", body: meta(`"grant_types":["authorization_code","authorization_code"]`), want: wantError{reason: "unsupported_grant_types", field: "grant_types", values: []string{"authorization_code", "authorization_code"}}},
		{name: "grants capped at eight", body: meta(`"grant_types":["` + strings.Repeat(`authorization_code","`, 11) + `authorization_code"]`), want: wantError{reason: "unsupported_grant_types", field: "grant_types", values: strings.Fields(strings.Repeat("authorization_code ", 8))}},
		{name: "response code and id_token", body: meta(`"response_types":["code","id_token"]`), want: wantError{reason: "unsupported_response_types", field: "response_types", values: []string{"code", "id_token"}}},
		{name: "response hybrid", body: meta(`"response_types":["code id_token"]`), want: wantError{reason: "unsupported_response_types", field: "response_types", values: []string{"code.id_token"}}},
		{name: "response token", body: meta(`"response_types":["token"]`), want: wantError{reason: "unsupported_response_types", field: "response_types", values: []string{"token"}}},
		{name: "response unknown", body: meta(`"response_types":["x-evil\r\ntype"]`), want: wantError{reason: "unsupported_response_types", field: "response_types", values: []string{"other"}}, absent: []string{"evil"}},
		{name: "redirect unparseable", body: `{"redirect_uris":["https://client.example/%zz"]}`, public: "invalid_redirect_uri", want: wantError{code: "invalid_redirect_uri", reason: "unparseable", field: "redirect_uris"}, absent: []string{"client.example", "%zz"}},
		{name: "redirect without host", body: `{"redirect_uris":["/callback"]}`, public: "invalid_redirect_uri", want: wantError{code: "invalid_redirect_uri", reason: "no_host", field: "redirect_uris"}},
		{name: "redirect opaque", body: `{"redirect_uris":["mailto:owner@client.example"]}`, public: "invalid_redirect_uri", want: wantError{code: "invalid_redirect_uri", reason: "no_host", field: "redirect_uris"}, absent: []string{"mailto", "owner@client.example"}},
		{name: "redirect userinfo", body: `{"redirect_uris":["https://user:pass@client.example/cb"]}`, public: "invalid_redirect_uri", want: wantError{code: "invalid_redirect_uri", reason: "userinfo", field: "redirect_uris"}, absent: []string{"user:pass", "client.example"}},
		{name: "redirect fragment", body: `{"redirect_uris":["https://client.example/cb#frag"]}`, public: "invalid_redirect_uri", want: wantError{code: "invalid_redirect_uri", reason: "fragment", field: "redirect_uris"}, absent: []string{"#frag", "client.example"}},
		{name: "redirect too long", body: `{"redirect_uris":["` + longURI + `"]}`, public: "invalid_redirect_uri", want: wantError{code: "invalid_redirect_uri", reason: "too_long", field: "redirect_uris"}, absent: []string{"aaaaaaaa"}},
		{name: "redirect scheme", body: `{"redirect_uris":["ftp://client.example/cb"]}`, public: "invalid_redirect_uri", want: wantError{code: "invalid_redirect_uri", reason: "scheme_not_allowed", field: "redirect_uris"}, absent: []string{"ftp"}},
		{name: "redirect custom scheme", body: `{"redirect_uris":["app-secret-scheme://callback"]}`, public: "invalid_redirect_uri", want: wantError{code: "invalid_redirect_uri", reason: "scheme_not_allowed", field: "redirect_uris"}, absent: []string{"app-secret-scheme"}},
		{name: "redirect http public host", body: `{"redirect_uris":["http://public.example/callback"]}`, public: "invalid_redirect_uri", want: wantError{code: "invalid_redirect_uri", reason: "http_non_loopback", field: "redirect_uris"}, absent: []string{"public.example"}},
		{name: "second redirect invalid", body: `{"redirect_uris":["https://client.example/ok","http://second.example/cb"]}`, public: "invalid_redirect_uri", want: wantError{code: "invalid_redirect_uri", reason: "http_non_loopback", field: "redirect_uris"}, absent: []string{"second.example"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var body io.Reader = strings.NewReader(tc.body)
			if tc.reader != nil {
				body = tc.reader
			}
			w := jsonReader(h, "POST", "/oauth/register", body, nil)
			public := tc.public
			if public == "" {
				public = "invalid_client_metadata"
			}
			if tc.want.code == "" {
				tc.want.code = "invalid_client_metadata"
			}
			if w.Code != 400 || strings.TrimSpace(w.Body.String()) != fmt.Sprintf(`{"error":%q}`, public) {
				t.Fatalf("public response changed: %d %s", w.Code, w.Body)
			}
			c := sink.one(t)
			c.expectRequest(t, "POST /oauth/register", 400, "none", "not_required")
			c.expectError(t, tc.want)
			c.expectNoEvents(t)
			c.expectClient(t, "", "")
			for _, s := range tc.absent {
				if strings.Contains(c.raw, s) {
					t.Fatalf("client text %q reached the log:\n%s", s, c.raw)
				}
			}
		})
	}
}

func TestRegisterSuccessEmitsEventAfterTheInsert(t *testing.T) {
	store, _ := setup(t, dbtest.OpenSQLite(t))
	h, sink := newLogged(t, store)
	body := `{"client_name":"Synthetic assistant",` + validRedirectField + `,"grant_types":["authorization_code"],"response_types":["code"],"token_endpoint_auth_method":"none"}`
	w := jsonRequest(h, "POST", "/oauth/register", body, nil)
	if w.Code != 201 {
		t.Fatalf("register: %d %s", w.Code, w.Body)
	}
	var created clientMetadata
	mustJSON(t, w.Body.Bytes(), &created)
	c := sink.one(t)
	c.expectRequest(t, "POST /oauth/register", 201, "none", "not_required")
	c.expectError(t, wantError{})
	// The registrant is not a validated client yet: only the event names it.
	c.expectClient(t, "", "")
	c.expectEvent(t, "oauth.client.registered", map[string]string{"client_name": "Synthetic assistant", "client_id_prefix": created.ID[:8]})
	if len(c.events) != 1 || strings.Contains(c.raw, created.ID) {
		t.Fatalf("the full client id must not be logged:\n%s", c.raw)
	}
}

func TestRegisterRegistryFullIsNotReportedAsRateLimiting(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	store, _ := setup(t, conn)
	h, sink := newLogged(t, store)
	tx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		if _, err = tx.Exec(`INSERT INTO oauth_clients(id,name,redirect_uris) VALUES(?,?,?)`, fmt.Sprintf("synthetic-%d", i), "Synthetic", `[]`); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	w := jsonRequest(h, "POST", "/oauth/register", `{`+validRedirectField+`}`, nil)
	if w.Code != 429 || strings.TrimSpace(w.Body.String()) != `{"error":"temporarily_unavailable"}` {
		t.Fatalf("registry full: %d %s", w.Code, w.Body)
	}
	c := sink.one(t)
	c.expectRequest(t, "POST /oauth/register", 429, "none", "not_required")
	c.expectError(t, wantError{code: "temporarily_unavailable", reason: "client_registry_full"})
	c.expectNoEvents(t)
}

func TestRegisterStorageFailureIsLoggedAsDatabaseError(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	store, _ := setup(t, conn)
	h, sink := newLogged(t, store)
	for _, stmt := range []string{`DROP TABLE oauth_grants`, `DROP TABLE oauth_clients`} {
		if _, err := conn.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	w := jsonRequest(h, "POST", "/oauth/register", `{`+validRedirectField+`}`, nil)
	if w.Code != 503 || strings.TrimSpace(w.Body.String()) != `{"error":"temporarily_unavailable"}` {
		t.Fatalf("storage failure: %d %s", w.Code, w.Body)
	}
	c := sink.one(t)
	c.expectError(t, wantError{code: "temporarily_unavailable", reason: "db_error"})
	c.expectNoEvents(t)
	if strings.Contains(c.raw, "no such table") {
		t.Fatalf("database error text reached the log:\n%s", c.raw)
	}
}

func TestSettingsFailureIsLoggedBeforeAnyHandlerRuns(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	store, _ := setup(t, conn)
	h, sink := newLogged(t, store)
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	w := jsonRequest(h, "POST", "/oauth/register", `{`+validRedirectField+`}`, nil)
	if w.Code != 503 {
		t.Fatalf("closed database: %d %s", w.Code, w.Body)
	}
	c := sink.one(t)
	c.expectRequest(t, "POST /oauth/register", 503, "none", "not_required")
	c.expectError(t, wantError{code: "temporarily_unavailable", reason: "settings_unavailable"})
}

// legacyValidRedirect is the pre-diagnostics implementation, kept verbatim as
// the reference that redirectProblem must stay equivalent to.
func legacyValidRedirect(value string) bool {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" || len(value) > 2048 {
		return false
	}
	return u.Scheme == "https" || (u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "::1" || u.Hostname() == "localhost"))
}

func TestRedirectProblemMatchesLegacyValidation(t *testing.T) {
	long := "https://client.example/" + strings.Repeat("a", 2100)
	atLimit := "https://client.example/" + strings.Repeat("a", 2048-len("https://client.example/"))
	tests := []struct{ uri, problem string }{
		{"https://client.example/callback", ""},
		{"https://client.example", ""},
		{"https://client.example/cb?x=1&y=%22", ""},
		{"https://CLIENT.example:8443/cb", ""},
		{"HTTPS://client.example/cb", ""},
		{"https://client.example/cb#", ""},
		{"http://127.0.0.1:45678/callback", ""},
		{"http://localhost/callback", ""},
		{"http://LOCALHOST:3000/callback", "http_non_loopback"},
		{"http://[::1]:8080/callback", ""},
		{atLimit, ""},
		{long, "too_long"},
		{"http://public.example/callback", "http_non_loopback"},
		{"http://127.0.0.2/callback", "http_non_loopback"},
		{"http://localhost.evil.example/callback", "http_non_loopback"},
		{"ftp://client.example/cb", "scheme_not_allowed"},
		{"custom-scheme://callback", "scheme_not_allowed"},
		{"javascript://client.example/%0aalert(1)", "scheme_not_allowed"},
		{"https://user@client.example/cb", "userinfo"},
		{"https://user:pass@client.example/cb", "userinfo"},
		{"https://client.example/cb#frag", "fragment"},
		{"/callback", "no_host"},
		{"callback", "no_host"},
		{"", "no_host"},
		{"https:///callback", "no_host"},
		{"mailto:owner@client.example", "no_host"},
		{"urn:ietf:wg:oauth:2.0:oob", "no_host"},
		{"https://client.example/%zz", "unparseable"},
		{"https://[::1/cb", "unparseable"},
		{"http://client.example:port/cb", "unparseable"},
		{"https://client .example/cb", "unparseable"},
		{"\x7fhttps://client.example", "unparseable"},
		{"https://user:pass@client.example/cb#frag", "userinfo"},
		{"ftp://user@client.example/cb", "userinfo"},
	}
	for _, tc := range tests {
		got := redirectProblem(tc.uri)
		if got != tc.problem {
			t.Errorf("redirectProblem(%.60q) = %q, want %q", tc.uri, got, tc.problem)
		}
		if (got == "") != legacyValidRedirect(tc.uri) {
			t.Errorf("redirectProblem(%.60q) = %q disagrees with the legacy boolean", tc.uri, got)
		}
		if got != "" && !tokenValue.MatchString(got) {
			t.Errorf("reason %q is not a log token", got)
		}
	}
}

func TestScopeProblemExplainsEveryRefusedScope(t *testing.T) {
	tests := []struct{ scope, problem string }{
		{"bogus", "scope_not_supported"},
		{"mcp:access openid", "scope_not_supported"},
		{"openid mcp:access mcp:access", "scope_not_supported"},
		{"mcp:access mcp:access", "scope_duplicate"},
		{"mcp:access mcp:write mcp:access", "scope_duplicate"},
		{"mcp:write", "scope_invalid_combination"},
		{"mcp:operations", "scope_invalid_combination"},
		{"mcp:access mcp:operations", "scope_invalid_combination"},
		{"mcp:write mcp:operations", "scope_invalid_combination"},
	}
	for _, tc := range tests {
		if normalizeScope(tc.scope) != "" {
			t.Fatalf("%q must be refused", tc.scope)
		}
		if got := scopeProblem(tc.scope); got != tc.problem {
			t.Errorf("scopeProblem(%q) = %q, want %q", tc.scope, got, tc.problem)
		}
	}
}
