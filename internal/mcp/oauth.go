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
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/mcpaccess"
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
	Authorization                                             *string
}

const grantColumns = `client_id,redirect_uri,resource,scope,challenge,state,csrf_hash,revision,authorization_id`

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
func (o *oauthServer) unavailable(w http.ResponseWriter) {
	oauthError(w, 503, "temporarily_unavailable")
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
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("X-Frame-Options", "DENY")
		if r.URL.Path != "/mcp" {
			if o.limited(false) {
				w.Header().Set("Retry-After", "60")
				oauthError(w, 429, "temporarily_unavailable")
				return
			}
			enabled, err := settings.AuthenticationEnabled(r.Context(), o.store.DB)
			if err != nil {
				o.unavailable(w)
				return
			}
			mcpEnabled, err := mcpaccess.Enabled(r.Context(), o.store.DB)
			if err != nil {
				o.unavailable(w)
				return
			}
			if !enabled || !mcpEnabled {
				oauthError(w, 403, "access_denied")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		}
		mux.ServeHTTP(w, r)
	})
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

func validRedirect(value string) bool {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" || len(value) > 2048 {
		return false
	}
	return u.Scheme == "https" || (u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "::1" || u.Hostname() == "localhost"))
}
func (o *oauthServer) register(w http.ResponseWriter, r *http.Request) {
	var c clientMetadata
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&c); err != nil {
		oauthError(w, 400, "invalid_client_metadata")
		return
	}
	if dec.Decode(new(any)) != io.EOF {
		oauthError(w, 400, "invalid_client_metadata")
		return
	}
	if c.ID != "" || len(c.Name) > 200 || len(c.Redirects) == 0 || len(c.Redirects) > 10 || (c.Method != "" && c.Method != "none") || (len(c.Grants) > 0 && (len(c.Grants) != 1 || c.Grants[0] != "authorization_code")) || (len(c.Responses) > 0 && (len(c.Responses) != 1 || c.Responses[0] != "code")) {
		oauthError(w, 400, "invalid_client_metadata")
		return
	}
	for _, uri := range c.Redirects {
		if !validRedirect(uri) {
			oauthError(w, 400, "invalid_redirect_uri")
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
		o.unavailable(w)
		return
	}
	n, err := result.RowsAffected()
	if err != nil {
		o.unavailable(w)
		return
	}
	if n != 1 {
		oauthError(w, 429, "temporarily_unavailable")
		return
	}
	jsonResponse(w, 201, c)
}
func (o *oauthServer) client(r *http.Request, id, redirect string) (string, error) {
	var name, raw string
	err := o.store.DB.QueryRowContext(r.Context(), `SELECT name,redirect_uris FROM oauth_clients WHERE id=?`, id).Scan(&name, &raw)
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
	return "", sql.ErrNoRows
}
func parseForm(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") || r.ParseForm() != nil {
		oauthError(w, 400, "invalid_request")
		return false
	}
	for _, values := range r.PostForm {
		if len(values) != 1 {
			oauthError(w, 400, "invalid_request")
			return false
		}
	}
	return true
}
func (o *oauthServer) save(r *http.Request, kind, raw string, g grant, lifetime time.Duration) error {
	result, err := o.store.DB.ExecContext(r.Context(), `INSERT INTO oauth_grants(token_hash,kind,`+grantColumns+`,expires_at) SELECT ?,?,?,?,?,?,?,?,?,?,?,? WHERE ?='consent' OR EXISTS (SELECT 1 FROM mcp_authorizations a JOIN auth_owner o ON o.id=1 AND o.revision=a.revision WHERE a.id=? AND a.revision=? AND a.revoked_at IS NULL AND a.expires_at>?)`, tokenHash(raw), kind, g.Client, g.Redirect, g.Resource, g.Scope, g.Challenge, g.State, g.CSRF, g.Revision, g.Authorization, db.FormatTime(o.store.Now().Add(lifetime)), kind, g.Authorization, g.Revision, db.FormatTime(o.store.Now()))
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
func (o *oauthServer) load(r *http.Request, kind, raw string, consume bool) (grant, error) {
	query := `SELECT ` + grantColumns + ` FROM oauth_grants WHERE token_hash=? AND kind=? AND expires_at>?`
	if consume {
		query = `DELETE FROM oauth_grants WHERE token_hash=? AND kind=? AND expires_at>? RETURNING ` + grantColumns
	}
	var g grant
	err := o.store.DB.QueryRowContext(r.Context(), query, tokenHash(raw), kind, db.FormatTime(o.store.Now())).Scan(&g.Client, &g.Redirect, &g.Resource, &g.Scope, &g.Challenge, &g.State, &g.CSRF, &g.Revision, &g.Authorization)
	return g, err
}
func (o *oauthServer) token(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	p := r.PostForm
	if p.Get("grant_type") != "authorization_code" {
		oauthError(w, 400, "unsupported_grant_type")
		return
	}
	if r.Header.Get("Authorization") != "" || p.Get("client_secret") != "" {
		oauthError(w, 401, "invalid_client")
		return
	}
	g, err := o.load(r, "code", p.Get("code"), true)
	if errors.Is(err, sql.ErrNoRows) {
		oauthError(w, 400, "invalid_grant")
		return
	}
	if err != nil {
		o.unavailable(w)
		return
	}
	verifier := p.Get("code_verifier")
	if !validVerifier(verifier) || subtle.ConstantTimeCompare([]byte(pkce(verifier)), []byte(g.Challenge)) != 1 || p.Get("client_id") != g.Client || p.Get("redirect_uri") != g.Redirect {
		oauthError(w, 400, "invalid_grant")
		return
	}
	if p.Get("resource") != g.Resource || g.Resource != o.resource() {
		oauthError(w, 400, "invalid_target")
		return
	}
	var revision int
	if err = o.store.DB.QueryRowContext(r.Context(), `SELECT revision FROM auth_owner WHERE id=1`).Scan(&revision); err != nil {
		o.unavailable(w)
		return
	}
	if revision != g.Revision || g.Authorization == nil {
		oauthError(w, 400, "invalid_grant")
		return
	}
	token := randomToken()
	if err = o.save(r, "access", token, g, accessLifetime); err != nil {
		if errors.Is(err, auth.ErrCredentials) {
			oauthError(w, 400, "invalid_grant")
			return
		}
		o.unavailable(w)
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
	if r.Header.Get("Authorization") != "" || r.PostForm.Get("client_secret") != "" {
		oauthError(w, 401, "invalid_client")
		return
	}
	var id string
	if err := o.store.DB.QueryRowContext(r.Context(), `SELECT id FROM oauth_clients WHERE id=?`, r.PostForm.Get("client_id")).Scan(&id); errors.Is(err, sql.ErrNoRows) {
		oauthError(w, 401, "invalid_client")
		return
	} else if err != nil {
		o.unavailable(w)
		return
	}
	if _, err := o.store.DB.ExecContext(r.Context(), `UPDATE mcp_authorizations SET revoked_at=COALESCE(revoked_at,?) WHERE id=(SELECT authorization_id FROM oauth_grants WHERE token_hash=? AND client_id=? AND kind='access')`, db.FormatTime(o.store.Now()), tokenHash(r.PostForm.Get("token")), id); err != nil {
		o.unavailable(w)
		return
	}
	w.WriteHeader(200)
}

// authenticate returns the narrower of the token's scope and the owner's current
// limit on its authorization, so a downgrade applies to tokens already issued.
func (o *oauthServer) authenticate(r *http.Request, token string) (string, error) {
	var scope, tokenScope string
	err := o.store.DB.QueryRowContext(r.Context(), `UPDATE mcp_authorizations SET last_used_at=? WHERE id=(SELECT authorization_id FROM oauth_grants WHERE token_hash=? AND kind='access' AND resource=? AND expires_at>? AND revision=(SELECT revision FROM auth_owner WHERE id=1)) AND revoked_at IS NULL AND expires_at>? AND revision=(SELECT revision FROM auth_owner WHERE id=1) RETURNING scope`, db.FormatTime(o.store.Now()), tokenHash(token), o.resource(), db.FormatTime(o.store.Now()), db.FormatTime(o.store.Now())).Scan(&scope)
	if err != nil {
		return "", err
	}
	if err = o.store.DB.QueryRowContext(r.Context(), `SELECT scope FROM oauth_grants WHERE token_hash=?`, tokenHash(token)).Scan(&tokenScope); err != nil {
		return "", err
	}
	return mcpaccess.Narrowest(scope, tokenScope), nil
}
func (o *oauthServer) unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+o.config.PublicURL+`/.well-known/oauth-protected-resource/mcp", scope="`+mcpScope+`"`)
	http.Error(w, "Authentication required", 401)
}
