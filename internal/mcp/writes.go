package mcp

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/google/jsonschema-go/jsonschema"
	"html/template"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/greg0x46/julius/internal/auth"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/httpapi"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type writeIdentity struct{ Hash, Scope string }
type writeContextKey struct{}
type writeAction struct {
	Preview                               httpapi.MCPWritePreview
	OwnerHash, CSRF, ClientName, ClientID string
	Expires                               time.Time
	State                                 string
	Result                                *httpapi.MCPWriteResult
	Error                                 string
}
type writeManager struct {
	mu      sync.Mutex
	actions map[string]*writeAction
}
type writeToolInput struct {
	ID             string         `json:"id,omitempty" jsonschema:"Existing UUID; omitted for create operations."`
	ScenarioID     string         `json:"scenario_id,omitempty" jsonschema:"Parent standalone scenario UUID for planned transaction writes."`
	Data           map[string]any `json:"data" jsonschema:"Strict REST request fields for this operation. Amounts must be decimal strings. Retroactive rule application is forbidden."`
	IdempotencyKey string         `json:"idempotency_key,omitempty" jsonschema:"Required for create_manual_transaction: 8-128 printable ASCII characters without whitespace or commas."`
}
type writeToolOutput struct {
	RequestID   string                   `json:"request_id"`
	State       string                   `json:"state"`
	ApprovalURL string                   `json:"approval_url,omitempty"`
	Preview     *httpapi.MCPWritePreview `json:"preview,omitempty"`
	Result      *httpapi.MCPWriteResult  `json:"result,omitempty"`
	Error       string                   `json:"error,omitempty"`
}

func (o *oauthServer) registerWrites(server *sdk.Server) {
	o.writes = &writeManager{actions: make(map[string]*writeAction)}
	outputSchema, err := jsonschema.For[writeToolOutput](&jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{reflect.TypeFor[json.RawMessage](): {}}})
	if err != nil {
		panic(err)
	}
	for _, operation := range []string{"assign_transaction_category", "set_transaction_inclusion", "create_manual_transaction", "update_manual_transaction", "delete_manual_transaction", "create_category", "update_category", "create_automation_rule", "update_automation_rule", "delete_automation_rule", "create_goal", "update_goal", "delete_goal", "create_scenario", "update_scenario", "delete_scenario", "create_planned_transaction", "update_planned_transaction", "delete_planned_transaction", "create_recurring_commitment", "update_recurring_commitment", "delete_recurring_commitment", "create_investment_account", "update_investment_account", "delete_investment_account", "create_investment_position", "update_investment_position", "delete_investment_position", "create_investment_operation", "update_investment_operation", "delete_investment_operation"} {
		sdk.AddTool(server, &sdk.Tool{Name: operation, OutputSchema: outputSchema, InputSchema: httpapi.MCPWriteInputSchema(operation), Description: "Prepare " + operation + " for owner review. Does not commit. Requires mcp:write. Open approval_url in Julius and have the owner review and approve; never ask for owner credentials in the MCP client. Use get_write_result afterward. Data fields are published in inputSchema; deletes use {}. Planning and manual investment amounts are BRL. Investment operations preserve provider-owned holdings and balances; local manual positions inside integrated custody are editable. Fees/taxes follow the investment ledger; no transfers or reconciliation writes. Standalone scenarios affect MCP projections only when explicitly selected in scenario_ids; recurrence schedules follow active commitment selection. No retroactive rule application.", Annotations: &sdk.ToolAnnotations{IdempotentHint: false}}, func(ctx context.Context, _ *sdk.CallToolRequest, in writeToolInput) (*sdk.CallToolResult, writeToolOutput, error) {
			identity, _ := ctx.Value(writeContextKey{}).(writeIdentity)
			if !canWrite(identity.Scope) {
				return nil, writeToolOutput{}, errors.New("mcp:write scope required")
			}
			data, err := json.Marshal(in.Data)
			if err != nil {
				return nil, writeToolOutput{}, errors.New("invalid data")
			}
			preview, err := httpapi.PreviewMCPWrite(ctx, o.store.DB, httpapi.MCPWriteRequest{Operation: operation, ID: in.ID, ScenarioID: in.ScenarioID, Data: data, IdempotencyKey: in.IdempotencyKey})
			if err != nil {
				return nil, writeToolOutput{}, errors.New(httpapi.MCPWriteError(err))
			}
			if raw, _ := json.Marshal(preview); len(raw) > 256<<10 {
				return nil, writeToolOutput{}, errors.New("preview too large; use Julius")
			}
			o.writes.mu.Lock()
			defer o.writes.mu.Unlock()
			for id, a := range o.writes.actions {
				if !o.store.Now().Before(a.Expires) {
					delete(o.writes.actions, id)
				}
			}
			if len(o.writes.actions) >= 100 {
				return nil, writeToolOutput{}, errors.New("too many pending writes; wait for expiry")
			}
			var name, client string
			if err = o.store.DB.QueryRowContext(ctx, `SELECT c.name,c.id FROM oauth_grants g JOIN oauth_clients c ON c.id=g.client_id WHERE g.token_hash=? AND g.kind='access'`, identity.Hash).Scan(&name, &client); err != nil {
				return nil, writeToolOutput{}, errors.New("authorization unavailable")
			}
			id := randomToken()
			o.writes.actions[id] = &writeAction{Preview: preview, OwnerHash: identity.Hash, ClientName: name, ClientID: client, Expires: o.store.Now().Add(codeLifetime), State: "pending"}
			return nil, o.writeOutput(id, o.writes.actions[id]), nil
		})
	}
	sdk.AddTool(server, &sdk.Tool{Name: "get_write_result", OutputSchema: outputSchema, Description: "Read a prepared write's preview, approval state and final REST result. Bound to the exact original write-scoped access token. Never commits.", Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true}}, func(ctx context.Context, _ *sdk.CallToolRequest, in struct {
		RequestID string `json:"request_id"`
	}) (*sdk.CallToolResult, writeToolOutput, error) {
		identity, _ := ctx.Value(writeContextKey{}).(writeIdentity)
		if !canWrite(identity.Scope) {
			return nil, writeToolOutput{}, errors.New("mcp:write scope required")
		}
		o.writes.mu.Lock()
		defer o.writes.mu.Unlock()
		a, ok := o.writes.actions[in.RequestID]
		if !ok || a.OwnerHash != identity.Hash || !o.store.Now().Before(a.Expires) {
			return nil, writeToolOutput{}, errors.New("write request unavailable or expired")
		}
		return nil, o.writeOutput(in.RequestID, a), nil
	})
}
func (o *oauthServer) writeOutput(id string, a *writeAction) writeToolOutput {
	out := writeToolOutput{RequestID: id, State: a.State, Result: a.Result, Error: a.Error}
	if a.State == "pending" {
		out.ApprovalURL = o.config.PublicURL + "/oauth/write?request=" + url.QueryEscape(id)
		out.Preview = &a.Preview
	}
	return out
}
func (o *oauthServer) writeGrantActive(ctx context.Context, hash string) (bool, error) {
	var scope string
	err := o.store.DB.QueryRowContext(ctx, `SELECT scope FROM oauth_grants WHERE token_hash=? AND kind='access' AND resource=? AND expires_at>? AND revision=(SELECT revision FROM auth_owner WHERE id=1)`, hash, o.resource(), db.FormatTime(o.store.Now())).Scan(&scope)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return canWrite(scope), err
}
func (o *oauthServer) approveWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if r.Header.Get("Origin") != o.config.PublicURL {
			oauthError(w, 403, "access_denied")
			return
		}
		if !parseForm(w, r) {
			return
		}
	}
	id := r.URL.Query().Get("request")
	if r.Method == http.MethodPost {
		id = r.PostForm.Get("request")
	}
	o.writes.mu.Lock()
	defer o.writes.mu.Unlock()
	a, ok := o.writes.actions[id]
	if !ok || !o.store.Now().Before(a.Expires) {
		o.renderWrite(w, 410, id, nil, "Solicitação expirada.", "")
		return
	}
	active, err := o.writeGrantActive(r.Context(), a.OwnerHash)
	if err != nil {
		o.unavailable(w)
		return
	}
	if !active {
		oauthError(w, 403, "access_denied")
		return
	}
	if a.State != "pending" {
		o.renderWrite(w, 200, id, a, "", "")
		return
	}
	cookieName := o.csrfCookie() + "_write"
	if r.Method == http.MethodGet {
		csrf := randomToken()
		a.CSRF = tokenHash(csrf)
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: csrf, Path: "/", Secure: o.config.Secure(), HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 300})
		o.renderWrite(w, 200, id, a, "", "")
		return
	}
	cookie, err := r.Cookie(cookieName)
	if err != nil || a.CSRF == "" || subtle.ConstantTimeCompare([]byte(tokenHash(cookie.Value)), []byte(a.CSRF)) != 1 {
		oauthError(w, 403, "access_denied")
		return
	}
	decision := r.PostForm.Get("decision")
	if decision != "allow" && decision != "deny" {
		oauthError(w, 400, "invalid_request")
		return
	}
	if decision == "deny" {
		a.State = "denied"
		o.renderWrite(w, 200, id, a, "", "")
		return
	}
	if o.limited(true) {
		oauthError(w, 429, "temporarily_unavailable")
		return
	}
	select {
	case o.hashing <- struct{}{}:
		defer func() { <-o.hashing }()
	default:
		oauthError(w, 429, "temporarily_unavailable")
		return
	}
	_, err = o.store.VerifyOwner(r.Context(), r.PostForm.Get("email"), r.PostForm.Get("password"))
	if errors.Is(err, auth.ErrCredentials) {
		o.renderWrite(w, 401, id, a, "E-mail ou senha inválidos.", r.PostForm.Get("email"))
		return
	}
	if err != nil {
		o.unavailable(w)
		return
	}
	active, err = o.writeGrantActive(r.Context(), a.OwnerHash)
	if err != nil {
		o.unavailable(w)
		return
	}
	if !active {
		oauthError(w, 403, "access_denied")
		return
	}
	// Consume before execution. A response failure cannot cause a second write.
	a.State = "failed"
	a.Expires = o.store.Now().Add(10 * time.Minute)
	result, err := httpapi.ExecuteMCPWrite(r.Context(), o.store.DB, a.Preview)
	if err != nil {
		a.Error = httpapi.MCPWriteError(err)
		if errors.Is(err, httpapi.ErrMCPWriteConflict) {
			a.Error = "Os dados mudaram. Solicite uma nova prévia."
		}
	} else {
		a.Result = &result
		if result.Status < 400 {
			a.State = "applied"
		} else {
			a.Error = "Não foi possível confirmar o resultado. Consulte os dados no Julius antes de tentar novamente."
		}
	}
	o.renderWrite(w, 200, id, a, "", "")
}
func (o *oauthServer) renderWrite(w http.ResponseWriter, status int, id string, a *writeAction, message, email string) {
	type row struct{ Label, Value string }
	rows := []row{}
	beforeRows := []row{}
	if a != nil {
		var data map[string]json.RawMessage
		_ = json.Unmarshal(a.Preview.Request.Data, &data)
		labels := map[string]string{"account_id": "Conta", "description": "Descrição", "amount": "Valor assinado", "occurred_at": "Data", "category_id": "Categoria", "state": "Inclusão", "name": "Nome", "kind": "Tipo", "is_active": "Ativa", "icon": "Ícone", "color": "Cor", "conditions": "Condições", "actions": "Ações", "logic_operator": "Operador lógico", "metric": "Métrica", "target_amount": "Valor alvo (BRL)", "target_date": "Data alvo", "notes": "Observações", "projected_at": "Data planejada", "category": "Categoria", "cadence": "Frequência", "day_of_month": "Dia do mês", "month_of_year": "Mês do ano", "start_date": "Data inicial", "end_date": "Data final", "scenario_id": "Cenário", "financial_account_id": "Conta bancária vinculada", "position_id": "Posição", "asset_id": "Ativo", "ticker": "Código do ativo", "currency_code": "Moeda", "quantity": "Quantidade", "unit_price": "Preço unitário", "fees": "Taxas", "taxes": "Impostos", "occurred_on": "Data da operação", "initial_quantity": "Quantidade inicial", "initial_unit_cost": "Custo unitário inicial", "initial_value": "Saldo inicial", "asset_type": "Tipo de ativo", "active": "Ativa", "source": "Origem", "editable": "Editável"}
		for _, key := range []string{"account_id", "description", "amount", "occurred_at", "category_id", "state", "name", "kind", "is_active", "icon", "color", "logic_operator", "conditions", "actions", "metric", "target_amount", "target_date", "notes", "projected_at", "category", "cadence", "day_of_month", "month_of_year", "start_date", "end_date", "scenario_id", "financial_account_id", "position_id", "asset_id", "ticker", "currency_code", "quantity", "unit_price", "fees", "taxes", "occurred_on", "initial_quantity", "initial_unit_cost", "initial_value", "asset_type", "active", "source", "editable"} {
			if value, ok := data[key]; ok {
				rows = append(rows, row{labels[key], strings.Trim(string(value), `"`)})
			}
		}
		for _, raw := range a.Preview.Before {
			var current map[string]json.RawMessage
			if json.Unmarshal(raw, &current) != nil {
				continue
			}
			for _, key := range []string{"id", "account_id", "account_name", "currency_code", "description", "amount", "occurred_at", "name", "kind", "is_active", "internal_category", "inclusion", "metric", "target_amount", "target_date", "notes", "projected_at", "category", "category_id", "cadence", "day_of_month", "month_of_year", "start_date", "end_date", "scenario_id", "financial_account_id", "position_id", "asset_id", "ticker", "currency_code", "quantity", "unit_price", "fees", "taxes", "occurred_on", "initial_quantity", "initial_unit_cost", "initial_value", "asset_type", "active", "source", "editable"} {
				if value, ok := current[key]; ok {
					label := labels[key]
					if label == "" {
						label = map[string]string{"id": "Identificador", "account_name": "Nome da conta", "currency_code": "Moeda", "internal_category": "Categoria atual", "inclusion": "Inclusão atual"}[key]
					}
					var plain string
					if json.Unmarshal(value, &plain) != nil {
						plain = string(value)
					}
					beforeRows = append(beforeRows, row{label, plain})
				}
			}
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = writePage.Execute(w, struct {
		ID, Message, Email, Operation string
		Action                        *writeAction
		Rows, BeforeRows              []row
	}{id, message, email, writeOperationLabel(a), a, rows, beforeRows})
}

func writeOperationLabel(a *writeAction) string {
	if a == nil {
		return ""
	}
	return map[string]string{
		"delete_investment_operation": "Excluir operação de investimento", "update_investment_operation": "Alterar operação de investimento", "create_investment_operation": "Registrar operação de investimento", "delete_investment_position": "Excluir posição de investimento", "update_investment_position": "Alterar posição de investimento", "create_investment_position": "Criar posição de investimento", "delete_investment_account": "Excluir conta de investimento", "update_investment_account": "Alterar conta de investimento", "create_investment_account": "Criar conta de investimento", "assign_transaction_category": "Atribuir categoria às transações",
		"set_transaction_inclusion":   "Alterar inclusão nos totais",
		"create_manual_transaction":   "Criar lançamento manual",
		"update_manual_transaction":   "Alterar lançamento manual",
		"delete_manual_transaction":   "Excluir lançamento manual",
		"create_category":             "Criar categoria",
		"update_category":             "Alterar categoria",
		"create_automation_rule":      "Criar regra de automação",
		"update_automation_rule":      "Alterar regra de automação",
		"delete_automation_rule":      "Excluir regra de automação",
		"create_goal":                 "Criar meta",
		"update_goal":                 "Alterar meta",
		"delete_goal":                 "Excluir meta",
		"create_scenario":             "Criar cenário",
		"update_scenario":             "Alterar cenário",
		"delete_scenario":             "Excluir cenário",
		"create_planned_transaction":  "Criar lançamento planejado",
		"update_planned_transaction":  "Alterar lançamento planejado",
		"delete_planned_transaction":  "Excluir lançamento planejado",
		"create_recurring_commitment": "Criar compromisso recorrente",
		"update_recurring_commitment": "Alterar compromisso recorrente",
		"delete_recurring_commitment": "Excluir compromisso recorrente",
	}[a.Preview.Request.Operation]
}

var writePage = template.Must(template.New("write").Parse(`<!doctype html>
<html lang="pt-BR"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Revisar alteração · Julius</title>
<style>body{margin:0;background:#f5f7fa;color:#172b4d;font:16px system-ui,sans-serif}main{max-width:540px;margin:40px auto;padding:24px}h1{font-size:1.6rem}dt,label{font-weight:600}dd{margin:4px 0 16px;overflow-wrap:anywhere}pre{white-space:pre-wrap;overflow-wrap:anywhere}label{display:block;margin:16px 0 6px}input,button{box-sizing:border-box;font:inherit;border-radius:8px;min-height:44px;padding:10px 14px}input{width:100%;border:1px solid #64748b}button{cursor:pointer;margin:16px 8px 0 0;border:1px solid #2457a6;color:#2457a6;background:white}button[value=allow]{background:#2457a6;color:white}:focus-visible{outline:3px solid #2457a6;outline-offset:3px}[role=alert]{color:#c4302b}@media(max-width:480px){main{margin:0;padding:24px 16px}button{width:100%;margin-right:0}}</style>
<main><p>Julius</p><h1>Revisar alteração solicitada</h1>{{if .Message}}<p role="alert">{{.Message}}</p>{{end}}
{{with .Action}}{{if eq .State "pending"}}<p>Confira o efeito desta alteração antes de aprovar.</p>
<dl><dt>Aplicativo (nome informado pelo cliente)</dt><dd>{{.ClientName}}</dd><dt>Identificador do cliente</dt><dd>{{.ClientID}}</dd><dt>Operação</dt><dd>{{$.Operation}}</dd><dt>Quantidade de registros afetados</dt><dd>{{.Preview.AffectedCount}}</dd><dt>Identificadores afetados</dt><dd>{{range .Preview.AffectedIDs}}{{.}}<br>{{else}}Novo registro (identificador gerado ao aplicar).{{end}}</dd></dl>
<p>{{.Preview.Effect}}</p><p>O efeito nos totais será recalculado conforme as regras do Julius.</p>
<h2>Dados solicitados</h2><dl>{{range $.Rows}}<dt>{{.Label}}</dt><dd>{{.Value}}</dd>{{end}}</dl>
{{if .Preview.Before}}<h2>Dados atuais</h2><dl>{{range $.BeforeRows}}<dt>{{.Label}}</dt><dd>{{.Value}}</dd>{{end}}</dl><details><summary>Dados atuais completos e vínculos</summary>{{range .Preview.Before}}<pre>{{printf "%s" .}}</pre>{{end}}</details>{{end}}
<details><summary>Dados da solicitação</summary><pre>{{printf "%s" .Preview.Request.Data}}</pre></details>
<p>A alteração será aplicada uma única vez após sua aprovação. Confirme sua identidade para autorizar.</p>
<form action="/oauth/write" method="post"><input type="hidden" name="request" value="{{$.ID}}"><label for="email">E-mail</label><input id="email" name="email" type="email" autocomplete="username" value="{{$.Email}}" required maxlength="254"><label for="password">Senha</label><input id="password" name="password" type="password" autocomplete="current-password" required><button type="submit" name="decision" value="allow">Aprovar e aplicar alteração</button><button type="submit" name="decision" value="deny" formnovalidate>Negar alteração</button></form>
{{else}}{{if eq .State "applied"}}<p>Alteração aplicada.</p>{{else if eq .State "denied"}}<p>Alteração negada.</p>{{else}}<p role="alert">{{.Error}}</p>{{end}}<p>Retorne ao aplicativo MCP para consultar o resultado.</p>{{end}}{{end}}</main></html>`))
