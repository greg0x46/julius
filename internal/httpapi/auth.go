package httpapi

import (
	"context"
	"errors"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/settings"
)

type identityKey struct{}
type authenticationEnabledKey struct{}
type identity struct {
	Email string
	Token string
}
type authAPI struct {
	store    *auth.Store
	config   auth.Config
	mu       sync.Mutex
	window   time.Time
	total    int
	attempts map[string]int
	hashing  chan struct{}
}

func newAuthAPI(store *auth.Store, config auth.Config) *authAPI {
	return &authAPI{store: store, config: config, attempts: make(map[string]int), hashing: make(chan struct{}, 1)}
}
func (a *authAPI) limited(email string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	if now.Sub(a.window) >= time.Minute {
		a.window = now
		a.total = 0
		clear(a.attempts)
	}
	if a.total >= 30 {
		return true
	}
	a.total++
	email = auth.NormalizeEmail(email)
	if a.attempts[email] >= 5 {
		return true
	}
	a.attempts[email]++
	return false
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
		return identity{}, auth.ErrSession
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
				writeProblem(w, 503, "auth-config-unavailable", "Configuração de autenticação indisponível", "Tente novamente em instantes.")
				return
			}
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			if r.Header.Get("Origin") != a.config.PublicURL || r.Header.Get("X-Julius-Request") != "1" {
				writeProblem(w, 403, "invalid-origin", "Origem da solicitação inválida", "")
				return
			}
			if a.config.Demo && demoBlocked(r) {
				writeProblem(w, 403, "demo-mode", "Indisponível na demonstração", "Este recurso fica desativado no ambiente de demonstração.")
				return
			}
		}
		r = r.WithContext(context.WithValue(r.Context(), authenticationEnabledKey{}, enabled))
		public := !enabled || (r.URL.Path == "/api/auth/login" && r.Method == "POST") || (r.URL.Path == "/api/auth/session" && r.Method == "GET")
		if !public {
			who, err := a.identity(r)
			if err != nil {
				a.authError(w, err)
				return
			}
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
func (a *authAPI) authError(w http.ResponseWriter, err error) {
	if errors.Is(err, auth.ErrSession) || errors.Is(err, auth.ErrCredentials) {
		writeProblem(w, 401, "unauthorized", "Autenticação necessária", "E-mail ou senha incorretos, ou sessão expirada.")
		return
	}
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
		writeJSON(w, 200, map[string]any{"authenticated": false, "authentication_enabled": true})
		return
	}
	if err != nil {
		a.authError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"authenticated": true, "authentication_enabled": true, "email": who.Email})
}
func (a *authAPI) acquire(w http.ResponseWriter, email string) bool {
	if !a.limited(email) {
		select {
		case a.hashing <- struct{}{}:
			return true
		default:
		}
	}
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
	if !a.acquire(w, req.Email) {
		return
	}
	defer func() { <-a.hashing }()
	token, err := a.store.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		a.authError(w, err)
		return
	}
	a.setCookie(w, token)
	writeJSON(w, 200, map[string]any{"authenticated": true, "authentication_enabled": true, "email": auth.NormalizeEmail(req.Email)})
}
func (a *authAPI) logout(w http.ResponseWriter, r *http.Request) {
	if !r.Context().Value(authenticationEnabledKey{}).(bool) {
		writeProblem(w, 409, "auth-disabled", "Autenticação desativada", "")
		return
	}
	who := r.Context().Value(identityKey{}).(identity)
	if err := a.store.Logout(r.Context(), who.Token); err != nil {
		a.authError(w, err)
		return
	}
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
	if !a.acquire(w, who.Email) {
		return
	}
	defer func() { <-a.hashing }()
	if err := a.store.ChangePassword(r.Context(), req.CurrentPassword, req.Password); err != nil {
		a.authError(w, err)
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
