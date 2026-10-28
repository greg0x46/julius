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

// Tool names must be explicitly classified; unclassified tools fail closed.
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
			if !ok || !mcpaccess.ValidScope(required) {
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
			if err != nil || !mcpaccess.ValidScope(scope) || (required == mcpaccess.ReadWrite && scope != mcpaccess.ReadWrite) {
				return nil, errors.New("Insufficient MCP permission")
			}
			return next(ctx, method, req)
		}
	}
}
