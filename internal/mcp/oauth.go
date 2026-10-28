package mcp

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/httplog"
	"github.com/greg0x46/julius/internal/settings"
)

const mcpScope = "mcp:access"
const writeScope = "mcp:write"

const operationsScope = "mcp:operations"

func normalizeScope(scope string) string {
	seen := map[string]bool{}
	for _, field := range strings.Fields(scope) {
		if seen[field] || (field != mcpScope && field != writeScope && field != operationsScope) {
			return ""
		}
		seen[field] = true
	}
	if !seen[mcpScope] || (seen[operationsScope] && !seen[writeScope]) {
		return ""
	}
	result := mcpScope
	if seen[writeScope] {
		result += " " + writeScope
	}
	if seen[operationsScope] {
		result += " " + operationsScope
	}
	return result
}

// scopeProblem mirrors normalizeScope to say why it refused a scope request.
func scopeProblem(scope string) string {
	seen := map[string]bool{}
	for _, field := range strings.Fields(scope) {
		switch {
		case field != mcpScope && field != writeScope && field != operationsScope:
			return "scope_not_supported"
		case seen[field]:
			return "scope_duplicate"
		}
		seen[field] = true
	}
	return "scope_invalid_combination"
}
func validScope(scope string) bool { return scope != "" && normalizeScope(scope) == scope }
func canWrite(scope string) bool {
	return validScope(scope) && strings.Contains(" "+scope+" ", " "+writeScope+" ")
}
func canOperate(scope string) bool {
	return canWrite(scope) && strings.HasSuffix(scope, " "+operationsScope)
}

const accessLifetime = time.Hour
const codeLifetime = 5 * time.Minute

// Only opaque hashes reach storage. Browser sessions cannot satisfy this gate.
type oauthServer struct {
	store            *auth.Store
	config           auth.Config
	mu               sync.Mutex
	window           time.Time
	requests, logins int
	hashing          chan struct{}
	writes           *writeManager
}

type grant struct {
	Client, Redirect, Resource, Scope, Challenge, State, CSRF string
	Revision                                                  int
}

const grantColumns = `client_id,redirect_uri,resource,scope,challenge,state,csrf_hash,revision`

func randomToken() string {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}
func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
func pkce(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
func jsonResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func oauthError(w http.ResponseWriter, status int, code string) {
	jsonResponse(w, status, map[string]string{"error": code})
}

// reject sends the usual OAuth error and records why for the access log. code,
// reason and field must be literals (see httplog.SetError), never client text.
func reject(w http.ResponseWriter, r *http.Request, status int, code, reason, field string) {
	httplog.SetError(r.Context(), code, reason, field)
	oauthError(w, status, code)
}
func (o *oauthServer) unavailable(w http.ResponseWriter, r *http.Request, reason string) {
	reject(w, r, 503, "temporarily_unavailable", reason, "")
}

// rateLimited logs the security event and sends the 429; callers set
// Retry-After first.
func rateLimited(w http.ResponseWriter, r *http.Request, limiter, reason string) {
	httplog.Event(r.Context(), "security.rate_limited", slog.String("limiter", limiter), slog.String("reason", reason))
	reject(w, r, 429, "temporarily_unavailable", "rate_limited", "")
}

// invalidOrigin logs the security event for a request whose Origin is not the
// public origin and returns the reason. The value itself is never logged.
func invalidOrigin(r *http.Request) string {
	reason := "origin_mismatch"
	if r.Header.Get("Origin") == "" {
		reason = "origin_missing"
	}
	httplog.Event(r.Context(), "security.invalid_origin", slog.String("reason", reason))
	return reason
}

// duplicateKey returns the smallest key that does not carry exactly one value.
func duplicateKey(values url.Values) (string, bool) {
	found, key := false, ""
	for k, v := range values {
		if len(v) != 1 && (!found || k < key) {
			found, key = true, k
		}
	}
	return key, found
}

// Names that may appear in the log; anything else is logged as "other".
var (
	knownParameters  = []string{"client_id", "redirect_uri", "resource", "scope", "code_challenge", "code_challenge_method", "response_type", "state", "grant_type", "code", "code_verifier", "client_secret", "token", "request", "decision", "email", "password"}
	knownAuthMethods = []string{"none", "client_secret_basic", "client_secret_post", "client_secret_jwt", "private_key_jwt", "tls_client_auth", "self_signed_tls_client_auth"}
	knownGrantTypes  = []string{"authorization_code", "refresh_token", "implicit", "client_credentials", "password", "urn:ietf:params:oauth:grant-type:device_code", "urn:ietf:params:oauth:grant-type:jwt-bearer", "urn:ietf:params:oauth:grant-type:token-exchange"}
	knownScopes      = []string{mcpScope, writeScope, operationsScope, "openid", "profile", "email", "offline_access"}
	// Multi-value response types are space separated; the log form uses dots.
	knownResponseTypes = map[string]string{"code": "code", "token": "token", "id_token": "id_token", "none": "none", "code id_token": "code.id_token", "code token": "code.token", "id_token token": "id_token.token", "code id_token token": "code.id_token.token"}
)

// knownValues maps up to the first eight values through httplog.Known.
func knownValues(values, known []string) []string {
	out := make([]string, 0, min(len(values), 8))
	for _, v := range values[:min(len(values), 8)] {
		out = append(out, httplog.Known(v, known...))
	}
	return out
}

func (o *oauthServer) resource() string { return o.config.PublicURL + "/mcp" }
func (o *oauthServer) limited(login bool) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if time.Since(o.window) >= time.Minute {
		o.window = time.Now()
		o.requests = 0
		o.logins = 0
	}
	if login {
		o.logins++
		return o.logins > 5
	}
	o.requests++
	return o.requests > 120
}
func (o *oauthServer) routes(transport http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/mcp", transport)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", o.resourceMetadata)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", o.resourceMetadata)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", o.metadata)
	mux.HandleFunc("POST /oauth/register", o.register)
	mux.HandleFunc("GET /oauth/write", o.approveWrite)
	mux.HandleFunc("POST /oauth/write", o.approveWrite)
	mux.HandleFunc("GET /oauth/authorize", o.authorize)
	mux.HandleFunc("POST /oauth/authorize", o.consent)
	mux.HandleFunc("POST /oauth/token", o.token)
	mux.HandleFunc("POST /oauth/revoke", o.revoke)
	return httplog.WithRoute(mux, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("X-Frame-Options", "DENY")
		if r.URL.Path != "/mcp" {
			if o.limited(false) {
				w.Header().Set("Retry-After", "60")
				rateLimited(w, r, "oauth_requests", "window_exceeded")
				return
			}
			enabled, err := settings.AuthenticationEnabled(r.Context(), o.store.DB)
			if err != nil {
				o.unavailable(w, r, "settings_unavailable")
				return
			}
			if !enabled {
				reject(w, r, 403, "access_denied", "authentication_disabled", "")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		}
		mux.ServeHTTP(w, r)
	}))
}
func (o *oauthServer) resourceMetadata(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, 200, map[string]any{"resource": o.resource(), "authorization_servers": []string{o.config.PublicURL}, "scopes_supported": []string{mcpScope, writeScope, operationsScope}, "bearer_methods_supported": []string{"header"}})
}
func (o *oauthServer) metadata(w http.ResponseWriter, r *http.Request) {
	base := o.config.PublicURL
	jsonResponse(w, 200, map[string]any{"issuer": base, "authorization_endpoint": base + "/oauth/authorize", "token_endpoint": base + "/oauth/token", "registration_endpoint": base + "/oauth/register", "revocation_endpoint": base + "/oauth/revoke", "response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code"}, "code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"none"}, "revocation_endpoint_auth_methods_supported": []string{"none"}, "scopes_supported": []string{mcpScope, writeScope, operationsScope}, "authorization_response_iss_parameter_supported": true, "client_id_metadata_document_supported": false})
}

type clientMetadata struct {
	ID        string   `json:"client_id,omitempty"`
	Name      string   `json:"client_name"`
	Redirects []string `json:"redirect_uris"`
	Method    string   `json:"token_endpoint_auth_method"`
	Grants    []string `json:"grant_types"`
	Responses []string `json:"response_types"`
}

// redirectProblem says why a redirect URI is refused, or "" when it is valid.
// The checks run in a fixed order and only the category is ever logged.
func redirectProblem(value string) string {
	u, err := url.Parse(value)
	switch {
	case err != nil:
		return "unparseable"
	case u.Host == "":
		return "no_host"
	case u.User != nil:
		return "userinfo"
	case u.Fragment != "":
		return "fragment"
	case u.Opaque != "":
		return "opaque"
	case len(value) > 2048:
		return "too_long"
	case u.Scheme == "https":
		return ""
	case u.Scheme != "http":
		return "scheme_not_allowed"
	}
	switch u.Hostname() {
	case "127.0.0.1", "::1", "localhost":
		return ""
	}
	return "http_non_loopback"
}

// decodeFailure classifies a JSON decoding error of the registration body.
func decodeFailure(err error) (reason, field string) {
	var (
		tooLarge *http.MaxBytesError
		typeErr  *json.UnmarshalTypeError
		syntax   *json.SyntaxError
	)
	switch {
	case errors.Is(err, io.EOF):
		return "empty_body", ""
	case errors.As(err, &tooLarge):
		return "body_too_large", ""
	case errors.As(err, &typeErr):
		if typeErr.Field == "" {
			return "invalid_field_type", "body"
		}
		return "invalid_field_type", httplog.Known(typeErr.Field, "client_name", "redirect_uris", "token_endpoint_auth_method", "grant_types", "response_types", "client_id")
	case errors.As(err, &syntax), errors.Is(err, io.ErrUnexpectedEOF):
		return "invalid_json", ""
	}
	return "body_read_error", ""
}

// invalidMetadata rejects a registration; values are already allowlisted.
func invalidMetadata(w http.ResponseWriter, r *http.Request, reason, field string, values ...string) {
	if len(values) > 0 {
		httplog.SetErrorValues(r.Context(), values...)
	}
	reject(w, r, 400, "invalid_client_metadata", reason, field)
}
func (o *oauthServer) register(w http.ResponseWriter, r *http.Request) {
	var c clientMetadata
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&c); err != nil {
		reason, field := decodeFailure(err)
		invalidMetadata(w, r, reason, field)
		return
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		reason, field := "multiple_json_values", ""
		if err != nil {
			reason, field = decodeFailure(err)
		}
		invalidMetadata(w, r, reason, field)
		return
	}
	switch {
	case c.ID != "":
		invalidMetadata(w, r, "client_id_provided", "client_id")
		return
	case len(c.Name) > 200:
		invalidMetadata(w, r, "name_too_long", "client_name")
		return
	case len(c.Redirects) == 0:
		invalidMetadata(w, r, "redirect_uris_missing", "redirect_uris")
		return
	case len(c.Redirects) > 10:
		invalidMetadata(w, r, "too_many_redirect_uris", "redirect_uris")
		return
	case c.Method != "" && c.Method != "none":
		invalidMetadata(w, r, "unsupported_auth_method", "token_endpoint_auth_method", httplog.Known(c.Method, knownAuthMethods...))
		return
	case len(c.Grants) > 0 && (len(c.Grants) != 1 || c.Grants[0] != "authorization_code"):
		invalidMetadata(w, r, "unsupported_grant_types", "grant_types", knownValues(c.Grants, knownGrantTypes)...)
		return
	case len(c.Responses) > 0 && (len(c.Responses) != 1 || c.Responses[0] != "code"):
		types := make([]string, 0, min(len(c.Responses), 8))
		for _, v := range c.Responses[:min(len(c.Responses), 8)] {
			if token, ok := knownResponseTypes[v]; ok {
				types = append(types, token)
			} else {
				types = append(types, "other")
			}
		}
		invalidMetadata(w, r, "unsupported_response_types", "response_types", types...)
		return
	}
	for _, uri := range c.Redirects {
		if problem := redirectProblem(uri); problem != "" {
			reject(w, r, 400, "invalid_redirect_uri", problem, "redirect_uris")
			return
		}
	}
	c.ID = randomToken()
	c.Method = "none"
	c.Grants = []string{"authorization_code"}
	c.Responses = []string{"code"}
	redirects, _ := json.Marshal(c.Redirects)
	result, err := o.store.DB.ExecContext(r.Context(), `INSERT INTO oauth_clients(id,name,redirect_uris) SELECT ?,?,? WHERE (SELECT COUNT(*) FROM oauth_clients)<1000`, c.ID, c.Name, string(redirects))
	if err != nil {
		o.unavailable(w, r, "db_error")
		return
	}
	n, err := result.RowsAffected()
	if err != nil {
		o.unavailable(w, r, "db_error")
		return
	}
	if n != 1 {
		reject(w, r, 429, "temporarily_unavailable", "client_registry_full", "")
		return
	}
	httplog.Event(r.Context(), "oauth.client.registered", slog.String("client_name", c.Name), slog.String("client_id_prefix", c.ID))
	jsonResponse(w, 201, c)
}

var (
	errUnknownClient    = errors.New("unknown client")
	errRedirectMismatch = errors.New("redirect URI not registered for client")
)

// client returns the registered name of id when redirect is one of its URIs:
// errUnknownClient or errRedirectMismatch otherwise.
func (o *oauthServer) client(r *http.Request, id, redirect string) (string, error) {
	var name, raw string
	err := o.store.DB.QueryRowContext(r.Context(), `SELECT name,redirect_uris FROM oauth_clients WHERE id=?`, id).Scan(&name, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errUnknownClient
	}
	if err != nil {
		return "", err
	}
	var redirects []string
	if err = json.Unmarshal([]byte(raw), &redirects); err != nil {
		return "", err
	}
	for _, uri := range redirects {
		if uri == redirect {
			return name, nil
		}
	}
	return "", errRedirectMismatch
}
func parseForm(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		reject(w, r, 400, "invalid_request", "invalid_content_type", "content_type")
		return false
	}
	if err := r.ParseForm(); err != nil {
		reason := "form_unparseable"
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			reason = "body_too_large"
		}
		reject(w, r, 400, "invalid_request", reason, "")
		return false
	}
	if key, found := duplicateKey(r.PostForm); found {
		reject(w, r, 400, "invalid_request", "duplicate_parameter", httplog.Known(key, knownParameters...))
		return false
	}
	return true
}
func (o *oauthServer) save(r *http.Request, kind, raw string, g grant, lifetime time.Duration) error {
	result, err := o.store.DB.ExecContext(r.Context(), `INSERT INTO oauth_grants(token_hash,kind,`+grantColumns+`,expires_at) SELECT ?,?,?,?,?,?,?,?,?,?,? WHERE ?='consent' OR EXISTS (SELECT 1 FROM auth_owner WHERE id=1 AND revision=?)`, tokenHash(raw), kind, g.Client, g.Redirect, g.Resource, g.Scope, g.Challenge, g.State, g.CSRF, g.Revision, db.FormatTime(o.store.Now().Add(lifetime)), kind, g.Revision)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return auth.ErrCredentials
	}
	return nil
}

// consume deletes and returns the grant of kind identified by raw, if live.
func (o *oauthServer) consume(r *http.Request, kind, raw string) (grant, error) {
	var g grant
	err := o.store.DB.QueryRowContext(r.Context(), `DELETE FROM oauth_grants WHERE token_hash=? AND kind=? AND expires_at>? RETURNING `+grantColumns, tokenHash(raw), kind, db.FormatTime(o.store.Now())).Scan(&g.Client, &g.Redirect, &g.Resource, &g.Scope, &g.Challenge, &g.State, &g.CSRF, &g.Revision)
	return g, err
}

// loadConsent reads a pending consent request together with the registered
// client name, in the same round trip.
func (o *oauthServer) loadConsent(r *http.Request, raw string) (g grant, name string, err error) {
	err = o.store.DB.QueryRowContext(r.Context(), `SELECT g.client_id,g.redirect_uri,g.resource,g.scope,g.challenge,g.state,g.csrf_hash,g.revision,COALESCE(c.name,'') FROM oauth_grants g LEFT JOIN oauth_clients c ON c.id=g.client_id WHERE g.token_hash=? AND g.kind='consent' AND g.expires_at>?`, tokenHash(raw), db.FormatTime(o.store.Now())).Scan(&g.Client, &g.Redirect, &g.Resource, &g.Scope, &g.Challenge, &g.State, &g.CSRF, &g.Revision, &name)
	return g, name, err
}
func (o *oauthServer) token(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	p := r.PostForm
	if p.Get("grant_type") != "authorization_code" {
		httplog.SetErrorValues(r.Context(), httplog.Known(p.Get("grant_type"), knownGrantTypes...))
		reject(w, r, 400, "unsupported_grant_type", "unsupported_grant_type", "grant_type")
		return
	}
	if r.Header.Get("Authorization") != "" {
		reject(w, r, 401, "invalid_client", "client_authentication_not_supported", "authorization")
		return
	}
	if p.Get("client_secret") != "" {
		reject(w, r, 401, "invalid_client", "client_authentication_not_supported", "client_secret")
		return
	}
	g, err := o.consume(r, "code", p.Get("code"))
	if errors.Is(err, sql.ErrNoRows) {
		reject(w, r, 400, "invalid_grant", "code_not_found_or_expired", "code")
		return
	}
	if err != nil {
		o.unavailable(w, r, "db_error")
		return
	}
	verifier := p.Get("code_verifier")
	switch {
	case !validVerifier(verifier):
		reject(w, r, 400, "invalid_grant", "verifier_invalid_format", "code_verifier")
		return
	case subtle.ConstantTimeCompare([]byte(pkce(verifier)), []byte(g.Challenge)) != 1:
		reject(w, r, 400, "invalid_grant", "pkce_mismatch", "code_verifier")
		return
	case p.Get("client_id") != g.Client:
		reject(w, r, 400, "invalid_grant", "client_id_mismatch", "client_id")
		return
	case p.Get("redirect_uri") != g.Redirect:
		reject(w, r, 400, "invalid_grant", "redirect_uri_mismatch", "redirect_uri")
		return
	}
	httplog.SetOAuthClient(r.Context(), g.Client, "")
	if p.Get("resource") != g.Resource || g.Resource != o.resource() {
		reject(w, r, 400, "invalid_target", "resource_mismatch", "resource")
		return
	}
	var revision int
	if err = o.store.DB.QueryRowContext(r.Context(), `SELECT revision FROM auth_owner WHERE id=1`).Scan(&revision); err != nil {
		o.unavailable(w, r, "db_error")
		return
	}
	if revision != g.Revision {
		reject(w, r, 400, "invalid_grant", "owner_revision_changed", "")
		return
	}
	token := randomToken()
	if err = o.save(r, "access", token, g, accessLifetime); err != nil {
		o.unavailable(w, r, "db_error")
		return
	}
	jsonResponse(w, 200, map[string]any{"access_token": token, "token_type": "Bearer", "expires_in": int(accessLifetime / time.Second), "scope": g.Scope})
}
func validVerifier(value string) bool {
	if len(value) < 43 || len(value) > 128 {
		return false
	}
	for _, c := range value {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.ContainsRune("-._~", c)) {
			return false
		}
	}
	return true
}
func (o *oauthServer) revoke(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	if r.Header.Get("Authorization") != "" {
		reject(w, r, 401, "invalid_client", "client_authentication_not_supported", "authorization")
		return
	}
	if r.PostForm.Get("client_secret") != "" {
		reject(w, r, 401, "invalid_client", "client_authentication_not_supported", "client_secret")
		return
	}
	var id, name string
	if err := o.store.DB.QueryRowContext(r.Context(), `SELECT id,name FROM oauth_clients WHERE id=?`, r.PostForm.Get("client_id")).Scan(&id, &name); errors.Is(err, sql.ErrNoRows) {
		reject(w, r, 401, "invalid_client", "unknown_client", "client_id")
		return
	} else if err != nil {
		o.unavailable(w, r, "db_error")
		return
	}
	httplog.SetOAuthClient(r.Context(), id, name)
	result, err := o.store.DB.ExecContext(r.Context(), `DELETE FROM oauth_grants WHERE token_hash=? AND client_id=? AND kind='access'`, tokenHash(r.PostForm.Get("token")), id)
	if err != nil {
		o.unavailable(w, r, "db_error")
		return
	}
	if n, err := result.RowsAffected(); err == nil && n > 0 {
		httplog.Event(r.Context(), "oauth.token.revoked", slog.String("client_name", name), slog.String("client_id_prefix", id))
	}
	w.WriteHeader(200)
}

// accessToken is what a valid bearer token resolves to.
type accessToken struct{ Scope, ClientID, ClientName string }

func (o *oauthServer) authenticate(r *http.Request, token string) (accessToken, error) {
	var t accessToken
	err := o.store.DB.QueryRowContext(r.Context(), `SELECT g.scope,c.id,c.name FROM oauth_grants g JOIN oauth_clients c ON c.id=g.client_id WHERE g.token_hash=? AND g.kind='access' AND g.resource=? AND g.expires_at>? AND g.revision=(SELECT revision FROM auth_owner WHERE id=1)`, tokenHash(token), o.resource(), db.FormatTime(o.store.Now())).Scan(&t.Scope, &t.ClientID, &t.ClientName)
	return t, err
}

// unauthorized answers 401 with the bearer challenge. kind is the mechanism
// whose check failed: oauth when a bearer was presented, none otherwise.
func (o *oauthServer) unauthorized(w http.ResponseWriter, r *http.Request, kind httplog.AuthType, reason string) {
	httplog.SetAuth(r.Context(), kind, httplog.AuthRejected)
	httplog.SetError(r.Context(), "unauthorized", reason, "")
	w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+o.config.PublicURL+`/.well-known/oauth-protected-resource/mcp", scope="`+mcpScope+`"`)
	http.Error(w, "Authentication required", 401)
}
