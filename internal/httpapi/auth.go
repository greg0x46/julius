package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/httplog"
	"github.com/greg0x46/julius/internal/settings"
)

// errNoCookie is auth.ErrSession for a request that carried no session cookie.
var errNoCookie = fmt.Errorf("%w (no cookie)", auth.ErrSession)

type identityKey struct{}
type authenticationEnabledKey struct{}
type identity struct {
	Email string
	Token string
}
type authAPI struct {
	store  *auth.Store
	config auth.Config
	// Budgets of failed credential checks only; successes never spend them.
	logins    *auth.Limiter // per normalized e-mail, plus a global backstop
	passwords *auth.Limiter // per session, so only its holder can exhaust it
	hashing   chan struct{}
}

func newAuthAPI(store *auth.Store, config auth.Config) *authAPI {
	return &authAPI{store: store, config: config, logins: auth.NewLimiter(5, 30), passwords: auth.NewLimiter(5, 30), hashing: make(chan struct{}, 1)}
}
func (a *authAPI) cookieName() string {
	if a.config.Secure() {
		return "__Host-julius_session"
	}
	return "julius_session"
}
func (a *authAPI) setCookie(w http.ResponseWriter, token string) {
	age := int(auth.Lifetime / time.Second)
	expires := time.Now().Add(auth.Lifetime)
	if token == "" {
		age = -1
		expires = time.Unix(1, 0)
	}
	http.SetCookie(w, &http.Cookie{Name: a.cookieName(), Value: token, Path: "/", HttpOnly: true, Secure: a.config.Secure(), SameSite: http.SameSiteLaxMode, MaxAge: age, Expires: expires})
}
func (a *authAPI) identity(r *http.Request) (identity, error) {
	cookie, err := r.Cookie(a.cookieName())
	if err != nil {
		return identity{}, errNoCookie
	}
	email, err := a.store.Authenticate(r.Context(), cookie.Value)
	return identity{Email: email, Token: cookie.Value}, err
}
func (a *authAPI) gate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api" && !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		// A demo is always open, whatever the stored setting says.
		enabled := false
		if !a.config.Demo {
			var err error
			if enabled, err = settings.AuthenticationEnabled(r.Context(), a.store.DB); err != nil {
				httplog.SetError(r.Context(), "auth-config-unavailable", "auth_config_unavailable", "")
				writeProblem(w, 503, "auth-config-unavailable", "Configuração de autenticação indisponível", "Tente novamente em instantes.")
				return
			}
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			if origin := r.Header.Get("Origin"); origin != a.config.PublicURL || r.Header.Get("X-Julius-Request") != "1" {
				// Categories only: neither header value is logged.
				reason := "origin_mismatch"
				switch {
				case origin == a.config.PublicURL:
					reason = "request_header_missing"
				case origin == "":
					reason = "origin_missing"
				}
				httplog.Event(r.Context(), "security.invalid_origin", slog.String("reason", reason))
				httplog.SetError(r.Context(), "invalid-origin", reason, "")
				writeProblem(w, 403, "invalid-origin", "Origem da solicitação inválida", "")
				return
			}
			if a.config.Demo && demoBlocked(r) {
				httplog.SetError(r.Context(), "demo-mode", "demo_blocked", "")
				writeProblem(w, 403, "demo-mode", "Indisponível na demonstração", "Este recurso fica desativado no ambiente de demonstração.")
				return
			}
		}
		r = r.WithContext(context.WithValue(r.Context(), authenticationEnabledKey{}, enabled))
		public := !enabled || (r.URL.Path == "/api/auth/login" && r.Method == "POST") || (r.URL.Path == "/api/auth/session" && r.Method == "GET")
		if !public {
			who, err := a.identity(r)
			if err != nil {
				// Infrastructure failures also deny access; the cause is in error_code.
				typ := httplog.AuthSession
				if errors.Is(err, errNoCookie) {
					typ = httplog.AuthNone
				}
				httplog.SetAuth(r.Context(), typ, httplog.AuthRejected)
				a.authError(w, r, err)
				return
			}
			httplog.SetAuth(r.Context(), httplog.AuthSession, httplog.AuthOK)
			r = r.WithContext(context.WithValue(r.Context(), identityKey{}, who))
		}
		next.ServeHTTP(w, r)
	})
}

// demoBlocked reports whether a write would change access (login, password,
// the authentication switch) or reach an external integration (provider
// credentials and connections, sync, quote providers). The public demo never
// allows them: anyone could otherwise lock everyone out or store credentials.
func demoBlocked(r *http.Request) bool {
	p := path.Clean(r.URL.Path)
	return strings.HasPrefix(p, "/api/auth/") || strings.HasPrefix(p, "/api/settings/") ||
		p == "/api/sync-runs" || p == "/api/data-sources" || strings.HasPrefix(p, "/api/data-sources/")
}
func (a *authAPI) authError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, auth.ErrSession) || errors.Is(err, auth.ErrCredentials) {
		reason := "invalid_session"
		switch {
		case errors.Is(err, errNoCookie):
			reason = "missing_session"
		case errors.Is(err, auth.ErrCredentials):
			reason = "invalid_credentials"
		}
		httplog.SetError(r.Context(), "unauthorized", reason, "")
		writeProblem(w, 401, "unauthorized", "Autenticação necessária", "E-mail ou senha incorretos, ou sessão expirada.")
		return
	}
	httplog.SetError(r.Context(), "auth-unavailable", "auth_unavailable", "")
	writeProblem(w, 503, "auth-unavailable", "Autenticação indisponível", "Tente novamente em instantes.")
}
func (a *authAPI) session(w http.ResponseWriter, r *http.Request) {
	if !r.Context().Value(authenticationEnabledKey{}).(bool) {
		session := map[string]any{"authenticated": false, "authentication_enabled": false}
		if a.config.Demo {
			session["demo"] = true
		}
		writeJSON(w, 200, session)
		return
	}
	who, err := a.identity(r)
	if errors.Is(err, auth.ErrSession) {
		// Expected probe result, not a rejection.
		writeJSON(w, 200, map[string]any{"authenticated": false, "authentication_enabled": true})
		return
	}
	if err != nil {
		a.authError(w, r, err)
		return
	}
	httplog.SetAuth(r.Context(), httplog.AuthSession, httplog.AuthOK)
	writeJSON(w, 200, map[string]any{"authenticated": true, "authentication_enabled": true, "email": who.Email})
}

// acquire refuses the request while the failure budget for key is spent, then
// takes the single hashing slot. The caller records a failed credential check
// with limiter.Record(key) before releasing the slot.
func (a *authAPI) acquire(w http.ResponseWriter, r *http.Request, limiter *auth.Limiter, key, name string) bool {
	reason := "window_exceeded"
	if !limiter.Blocked(key) {
		select {
		case a.hashing <- struct{}{}:
			return true
		default:
			name, reason = "auth_hashing_busy", "busy"
		}
	}
	httplog.Event(r.Context(), "security.rate_limited", slog.String("reason", reason), slog.String("limiter", name))
	httplog.SetError(r.Context(), "too-many-attempts", reason, "")
	w.Header().Set("Retry-After", "60")
	writeProblem(w, 429, "too-many-attempts", "Muitas tentativas", "Tente novamente em um minuto.")
	return false
}
func (a *authAPI) login(w http.ResponseWriter, r *http.Request) {
	if !r.Context().Value(authenticationEnabledKey{}).(bool) {
		writeProblem(w, 409, "auth-disabled", "Autenticação desativada", "Ative a autenticação nas configurações para entrar.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if decodeStrict(r, &req) != nil || len(req.Email) > 254 {
		writeProblem(w, 422, "invalid-login", "Solicitação inválida", "")
		return
	}
	email := auth.NormalizeEmail(req.Email)
	if !a.acquire(w, r, a.logins, email, "auth_attempts") {
		return
	}
	defer func() { <-a.hashing }()
	token, err := a.store.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		// Only rejected credentials count as a failed login; an unavailable
		// store is an outage, reported as such by authError.
		if errors.Is(err, auth.ErrCredentials) {
			a.logins.Record(email)
			httplog.SetAuth(r.Context(), httplog.AuthNone, httplog.AuthRejected)
			httplog.Event(r.Context(), "auth.login.failure", slog.String("reason", "invalid_credentials"))
		}
		a.authError(w, r, err)
		return
	}
	a.setCookie(w, token)
	httplog.Event(r.Context(), "auth.login.success")
	writeJSON(w, 200, map[string]any{"authenticated": true, "authentication_enabled": true, "email": email})
}
func (a *authAPI) logout(w http.ResponseWriter, r *http.Request) {
	if !r.Context().Value(authenticationEnabledKey{}).(bool) {
		writeProblem(w, 409, "auth-disabled", "Autenticação desativada", "")
		return
	}
	who := r.Context().Value(identityKey{}).(identity)
	if err := a.store.Logout(r.Context(), who.Token); err != nil {
		a.authError(w, r, err)
		return
	}
	httplog.Event(r.Context(), "auth.logout")
	a.setCookie(w, "")
	writeJSON(w, 200, map[string]bool{"authenticated": false, "authentication_enabled": true})
}
func (a *authAPI) password(w http.ResponseWriter, r *http.Request) {
	if !r.Context().Value(authenticationEnabledKey{}).(bool) {
		writeProblem(w, 409, "auth-disabled", "Autenticação desativada", "")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req struct {
		CurrentPassword string `json:"current_password"`
		Password        string `json:"password"`
	}
	if decodeStrict(r, &req) != nil || !auth.ValidPassword(req.Password) {
		writeProblem(w, 422, "invalid-password", "Senha inválida", "A nova senha não pode ficar vazia.")
		return
	}
	who := r.Context().Value(identityKey{}).(identity)
	// Keyed by the session, not the login e-mail: someone who can only guess
	// at the login form cannot spend this budget.
	digest := sha256.Sum256([]byte(who.Token))
	key := hex.EncodeToString(digest[:])
	if !a.acquire(w, r, a.passwords, key, "auth_password_attempts") {
		return
	}
	defer func() { <-a.hashing }()
	if err := a.store.ChangePassword(r.Context(), req.CurrentPassword, req.Password); err != nil {
		if errors.Is(err, auth.ErrCredentials) {
			a.passwords.Record(key)
		}
		a.authError(w, r, err)
		return
	}
	a.setCookie(w, "")
	writeJSON(w, 200, map[string]bool{"authenticated": false, "authentication_enabled": true})
}

func (a *authAPI) updateConfig(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if decodeStrict(r, &req) != nil || req.Enabled == nil {
		writeProblem(w, 422, "invalid-auth-config", "Configuração de autenticação inválida", "Informe enabled como true ou false.")
		return
	}
	if err := settings.SetAuthenticationEnabled(r.Context(), a.store.DB, *req.Enabled); err != nil {
		writeProblem(w, 503, "auth-config-unavailable", "Configuração de autenticação indisponível", "Tente novamente em instantes.")
		return
	}
	writeJSON(w, 200, map[string]bool{"authenticated": false, "authentication_enabled": *req.Enabled})
}
