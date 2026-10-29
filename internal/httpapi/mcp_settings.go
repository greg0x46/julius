package httpapi

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/mcpaccess"
)

func (a *authAPI) mcpOwner(w http.ResponseWriter, r *http.Request) bool {
	if a.config.Demo || r.Context().Value(identityKey{}) == nil {
		writeProblem(w, 403, "mcp-owner-required", "Autenticação necessária", "Ative a autenticação e entre na conta para gerenciar clientes MCP.")
		return false
	}
	return true
}
func (a *authAPI) getMCP(w http.ResponseWriter, r *http.Request) {
	if !a.mcpOwner(w, r) {
		return
	}
	enabled, err := mcpaccess.Enabled(r.Context(), a.store.DB)
	if err != nil {
		a.mcpError(w, err)
		return
	}
	connections, err := mcpaccess.NewStore(a.store.DB).List(r.Context())
	if err != nil {
		a.mcpError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"available": a.config.MCPEnabled, "enabled": enabled, "connections": connections})
}
func (a *authAPI) updateMCP(w http.ResponseWriter, r *http.Request) {
	if !a.mcpOwner(w, r) {
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if decodeStrict(r, &req) != nil || req.Enabled == nil {
		writeProblem(w, 422, "invalid-mcp-settings", "Configuração MCP inválida", "")
		return
	}
	if err := mcpaccess.SetEnabled(r.Context(), a.store.DB, *req.Enabled); err != nil {
		a.mcpError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"saved": true})
}
func (a *authAPI) changeMCPConnection(w http.ResponseWriter, r *http.Request) {
	if !a.mcpOwner(w, r) {
		return
	}
	id := r.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeProblem(w, 422, "invalid-mcp-client", "Cliente MCP inválido", "")
		return
	}
	store := mcpaccess.NewStore(a.store.DB)
	var err error
	if r.Method == "DELETE" {
		err = store.Revoke(r.Context(), id)
	} else {
		var req struct {
			Scope string `json:"scope"`
		}
		if decodeStrict(r, &req) != nil || !mcpaccess.ValidScope(req.Scope) {
			writeProblem(w, 422, "invalid-mcp-scope", "Permissões inválidas", "")
			return
		}
		err = store.SetScope(r.Context(), id, req.Scope)
	}
	if err != nil {
		a.mcpError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"saved": true})
}
func (a *authAPI) mcpError(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeProblem(w, 409, "mcp-client-unavailable", "Não foi possível alterar este cliente", "O acesso expirou, foi revogado ou a permissão não foi autorizada.")
		return
	}
	writeProblem(w, 503, "mcp-unavailable", "Acesso MCP indisponível", "Tente novamente em instantes.")
}
