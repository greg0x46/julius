package httpapi

import (
	"net/http"

	"github.com/greg0x46/julius/internal/auth"
)

// contentSecurityPolicy fits the built frontend: same-origin scripts and API
// calls, React inline styles (applied through the CSSOM, not style attributes)
// and the Baloo 2 brand font from Google Fonts (index.html). Everything else is
// closed, including inline scripts, plugins, <base> and framing.
const contentSecurityPolicy = "default-src 'self'; style-src 'self' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; " +
	"object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// referrerPolicy must stay same-origin, like the OAuth pages (see
// oauthReferrerPolicy in internal/mcp): no-referrer makes browsers send
// "Origin: null" on the consent forms.
const referrerPolicy = "same-origin"

// SecurityHeaders sets browser hardening headers on every response. It runs
// before the handlers, so a stricter header they set (the /oauth/* pages) wins.
// HSTS is sent only for an HTTPS public origin, without includeSubDomains or
// preload: those widen the commitment to the whole domain and belong to the
// reverse proxy.
func SecurityHeaders(config auth.Config) func(http.Handler) http.Handler {
	hsts := config.Secure()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", referrerPolicy)
			h.Set("Content-Security-Policy", contentSecurityPolicy)
			if hsts {
				h.Set("Strict-Transport-Security", "max-age=31536000")
			}
			next.ServeHTTP(w, r)
		})
	}
}
