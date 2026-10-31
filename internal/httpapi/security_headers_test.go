package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/httpapi"
)

func TestSecurityHeadersMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	serve := func(config auth.Config, next http.Handler) http.Header {
		w := httptest.NewRecorder()
		httpapi.SecurityHeaders(config)(next).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		return w.Result().Header
	}

	h := serve(auth.Config{PublicURL: "http://localhost:4200"}, inner)
	for name, want := range map[string]string{"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY", "Referrer-Policy": "same-origin"} {
		if got := h.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if h.Get("Content-Security-Policy") == "" {
		t.Error("no Content-Security-Policy")
	}
	if got := h.Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS %q for an HTTP origin", got)
	}

	if got := serve(auth.Config{PublicURL: "https://finance.example"}, inner).Get("Strict-Transport-Security"); got != "max-age=31536000" {
		t.Errorf("HSTS = %q, want max-age=31536000 and nothing else", got)
	}

	// A handler's own header replaces the default: the OAuth pages rely on it.
	stricter := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		w.WriteHeader(http.StatusNoContent)
	})
	if got := serve(auth.Config{PublicURL: "https://finance.example"}, stricter).Get("Content-Security-Policy"); got != "default-src 'none'" {
		t.Errorf("CSP = %q, want the handler's value", got)
	}
}
