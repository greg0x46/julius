package mcp

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/greg0x46/julius/internal/mcpaccess"
	"github.com/greg0x46/julius/internal/settings"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Every tool call rechecks current persistence (disable, revocation, downgrade),
// including queued calls. With a non-nil map, tools must be explicitly
// classified and unclassified ones fail closed; with nil, each tool enforces its
// own scope from the identity placed in the context.
func (o *oauthServer) permissions(scopes map[string]string) sdk.Middleware {
	return func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
			if method != "tools/call" {
				return next(ctx, method, req)
			}
			p, ok := req.GetParams().(*sdk.CallToolParamsRaw)
			if !ok {
				return nil, errors.New("Invalid tool request")
			}
			required, ok := scopes[p.Name]
			if scopes != nil && (!ok || !mcpaccess.ValidScope(required)) {
				return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "Tool permission is not configured"}
			}
			extra := req.GetExtra()
			if extra == nil {
				return nil, errors.New("Authorization required")
			}
			scheme, token, ok := strings.Cut(extra.Header.Get("Authorization"), " ")
			if !ok || !strings.EqualFold(scheme, "Bearer") {
				return nil, errors.New("Authorization required")
			}
			enabled, err := mcpaccess.Enabled(ctx, o.store.DB)
			if err != nil || !enabled {
				return nil, errors.New("MCP unavailable")
			}
			enabled, err = settings.AuthenticationEnabled(ctx, o.store.DB)
			if err != nil || !enabled {
				return nil, errors.New("MCP unavailable")
			}
			// Recheck current persistence at execution, including queued calls.
			scope, err := o.authenticate((&http.Request{}).WithContext(ctx), token)
			if err != nil || !mcpaccess.ValidScope(scope) || (scopes != nil && mcpaccess.Rank(scope) < mcpaccess.Rank(required)) {
				return nil, errors.New("Insufficient MCP permission")
			}
			// Tools see the freshly narrowed scope, not the one from request start.
			if identity, ok := ctx.Value(writeContextKey{}).(writeIdentity); ok && identity.Hash == tokenHash(token) {
				ctx = context.WithValue(ctx, writeContextKey{}, writeIdentity{Hash: identity.Hash, Scope: scope})
			}
			return next(ctx, method, req)
		}
	}
}
