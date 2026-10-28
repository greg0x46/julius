// Package mcp exposes the provider-neutral MCP transport alongside the REST API.
package mcp

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/settings"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const requestTimeout = 30 * time.Second
const maxConcurrentRequests = 16

// ParseEnabled reads JULIUS_MCP_ENABLED; an empty value disables MCP.
func ParseEnabled(value string) (bool, error) {
	if strings.TrimSpace(value) == "" {
		return false, nil
	}
	enabled, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false, fmt.Errorf("JULIUS_MCP_ENABLED deve ser true ou false")
	}
	return enabled, nil
}

// NewHandler never accepts cookies or inherits the REST API's open-access mode.
// Bearer credentials are existing owner sessions, with the same revocation rules.
func NewHandler(enabled bool, config auth.Config, store *auth.Store) (http.Handler, error) {
	if !enabled {
		return http.NotFoundHandler(), nil
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.Demo {
		return nil, fmt.Errorf("JULIUS_MCP_ENABLED não é permitido com JULIUS_DEMO")
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "julius", Version: "1"}, &sdk.ServerOptions{
		Capabilities: &sdk.ServerCapabilities{Tools: &sdk.ToolCapabilities{}},
	})
	transport := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, &sdk.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 1 << 20,
		// Reverse proxies may forward a public Host to a loopback listener.
		// The gate below validates Origin and requires explicit Bearer credentials.
		DisableLocalhostProtection: true,
	})
	slots := make(chan struct{}, maxConcurrentRequests)
	guarded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			w.Header().Set("Retry-After", "1")
			http.Error(w, "MCP busy", http.StatusTooManyRequests)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != config.PublicURL {
			http.Error(w, "Forbidden origin", http.StatusForbidden)
			return
		}
		scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
			unauthorized(w)
			return
		}
		enabled, err := settings.AuthenticationEnabled(r.Context(), store.DB)
		if err != nil {
			http.Error(w, "Authentication unavailable", http.StatusServiceUnavailable)
			return
		}
		if !enabled {
			http.Error(w, "MCP requires authentication enabled", http.StatusForbidden)
			return
		}
		if _, err := store.Authenticate(r.Context(), token); err != nil {
			if errors.Is(err, auth.ErrSession) {
				unauthorized(w)
			} else {
				http.Error(w, "Authentication unavailable", http.StatusServiceUnavailable)
			}
			return
		}
		transport.ServeHTTP(w, r)
	})
	bounded := http.TimeoutHandler(guarded, requestTimeout, "MCP request timed out")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		// Bound slow body reads as well as work that observes the request context.
		_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(requestTimeout))
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(requestTimeout + 5*time.Second))
		bounded.ServeHTTP(w, r)
	}), nil
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="julius-mcp"`)
	http.Error(w, "Authentication required", http.StatusUnauthorized)
}
