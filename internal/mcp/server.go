// Package mcp exposes the provider-neutral MCP transport alongside the REST API.
package mcp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/httpapi"
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
// Bearer credentials are scoped OAuth tokens, isolated from owner sessions.
func NewHandler(enabled bool, config auth.Config, store *auth.Store, operations ...Operations) (http.Handler, error) {
	if !enabled {
		return http.NotFoundHandler(), nil
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.Demo {
		return nil, fmt.Errorf("JULIUS_MCP_ENABLED não é permitido com JULIUS_DEMO")
	}
	oauth := &oauthServer{store: store, config: config, hashing: make(chan struct{}, 1)}
	server := sdk.NewServer(&sdk.Implementation{Name: "julius", Version: "1"}, &sdk.ServerOptions{
		Capabilities: &sdk.ServerCapabilities{Tools: &sdk.ToolCapabilities{}},
	})
	httpapi.RegisterMCPPlanningTools(server, store.DB, registerPortfolioApp(server))
	oauth.registerWrites(server)
	var ops Operations
	if len(operations) > 0 {
		ops = operations[0]
	}
	oauth.registerOperations(server, ops)
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
			oauth.unauthorized(w)
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
		scope, err := oauth.authenticate(r, token)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				oauth.unauthorized(w)
			} else {
				http.Error(w, "Authentication unavailable", http.StatusServiceUnavailable)
			}
			return
		}
		if !validScope(scope) {
			w.Header().Set("WWW-Authenticate", `Bearer error="insufficient_scope", scope="`+mcpScope+`", resource_metadata="`+config.PublicURL+`/.well-known/oauth-protected-resource/mcp"`)
			http.Error(w, "Insufficient scope", http.StatusForbidden)
			return
		}
		ctx := context.WithValue(r.Context(), writeContextKey{}, writeIdentity{Hash: tokenHash(token), Scope: scope})
		transport.ServeHTTP(w, r.WithContext(ctx))
	})
	bounded := http.TimeoutHandler(oauth.routes(guarded), requestTimeout, "MCP request timed out")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		// Bound slow body reads as well as work that observes the request context.
		_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(requestTimeout))
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(requestTimeout + 5*time.Second))
		bounded.ServeHTTP(w, r)
	}), nil
}
