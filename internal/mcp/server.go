// Package mcp exposes the provider-neutral MCP transport alongside the REST API.
package mcp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/httpapi"
	"github.com/greg0x46/julius/internal/httplog"
	"github.com/greg0x46/julius/internal/mcpaccess"
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
	server.AddReceivingMiddleware(oauth.permissions(nil))
	registerReadTools(server, store.DB)
	httpapi.RegisterMCPPlanningTools(server, store.DB)
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
		ctx := r.Context()
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			httplog.Event(ctx, "security.rate_limited", slog.String("limiter", "mcp_concurrency"), slog.String("reason", "busy"))
			httplog.SetError(ctx, "too_many_requests", "busy", "")
			w.Header().Set("Retry-After", "1")
			http.Error(w, "MCP busy", http.StatusTooManyRequests)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != config.PublicURL {
			httplog.SetError(ctx, "access_denied", invalidOrigin(r), "origin")
			http.Error(w, "Forbidden origin", http.StatusForbidden)
			return
		}
		header := r.Header.Get("Authorization")
		scheme, token, ok := strings.Cut(header, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
			reason := "missing_bearer"
			if header != "" {
				reason = "malformed_authorization"
			}
			oauth.unauthorized(w, r, httplog.AuthNone, reason)
			return
		}
		unavailable := func() {
			httplog.SetAuth(ctx, httplog.AuthOAuth, httplog.AuthRejected)
			httplog.SetError(ctx, "temporarily_unavailable", "auth_unavailable", "")
			http.Error(w, "Authentication unavailable", http.StatusServiceUnavailable)
		}
		enabled, err := settings.AuthenticationEnabled(ctx, store.DB)
		if err != nil {
			unavailable()
			return
		}
		mcpEnabled, err := mcpaccess.Enabled(r.Context(), store.DB)
		if err != nil {
			unavailable()
			return
		}
		if !enabled || !mcpEnabled {
			reason := "authentication_disabled"
			if enabled {
				reason = "mcp_disabled"
			}
			httplog.SetError(ctx, "access_denied", reason, "")
			http.Error(w, "MCP requires authentication enabled", http.StatusForbidden)
			return
		}
		access, err := oauth.authenticate(r, token)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				oauth.unauthorized(w, r, httplog.AuthOAuth, "invalid_token")
			} else {
				unavailable()
			}
			return
		}
		httplog.SetAuth(ctx, httplog.AuthOAuth, httplog.AuthOK)
		httplog.SetOAuthClient(ctx, access.ClientID, access.ClientName)
		scope := access.Scope
		if !validScope(scope) {
			httplog.SetError(ctx, "insufficient_scope", "scope_invalid", "")
			w.Header().Set("WWW-Authenticate", `Bearer error="insufficient_scope", scope="`+mcpScope+`", resource_metadata="`+config.PublicURL+`/.well-known/oauth-protected-resource/mcp"`)
			http.Error(w, "Insufficient scope", http.StatusForbidden)
			return
		}
		transport.ServeHTTP(w, r.WithContext(context.WithValue(ctx, writeContextKey{}, writeIdentity{Hash: tokenHash(token), Scope: scope})))
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
