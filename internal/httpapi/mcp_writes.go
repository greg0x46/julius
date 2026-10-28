package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/google/jsonschema-go/jsonschema"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/shopspring/decimal"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/greg0x46/julius/internal/automation"
	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/goals"
	"github.com/greg0x46/julius/internal/ledger"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/scenarios"
	"github.com/greg0x46/julius/internal/transactions"
)

// MCPWriteRequest is an allowlisted domain operation, never a URL or SQL query.
type MCPWriteRequest struct {
	Operation      string          `json:"operation"`
	ID             string          `json:"id,omitempty"`
	ScenarioID     string          `json:"scenario_id,omitempty"`
	Data           json.RawMessage `json:"data"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
}

type MCPWritePreview struct {
	Request       MCPWriteRequest   `json:"request"`
	AffectedIDs   []string          `json:"affected_ids"`
	AffectedCount int               `json:"affected_count"`
	Before        []json.RawMessage `json:"before"`
	Effect        string            `json:"effect"`
}

var plainWriteDecimal = regexp.MustCompile(`^-?[0-9]+(?:\.[0-9]+)?$`)

var ErrMCPWriteConflict = errors.New("financial data changed; request a new preview")
var errMCPWriteInput = errors.New("invalid financial write: check operation, UUID identifiers and documented fields")

func decodeMCPWrite(data []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return errMCPWriteInput
	}
	if d.Decode(new(any)) != io.EOF {
		return errMCPWriteInput
	}
	return nil
}
func validWriteID(id string) bool          { _, err := uuid.Parse(id); return err == nil }
func validOptionalWriteID(id *string) bool { return id == nil || validWriteID(*id) }

// PreviewMCPWrite validates without writing and captures the objects the owner reviews.
func PreviewMCPWrite(ctx context.Context, q transactions.Querier, in MCPWriteRequest) (MCPWritePreview, error) {
	p := MCPWritePreview{Request: in, AffectedIDs: []string{}, Before: []json.RawMessage{}}
	if len(in.Data) > 16<<10 || len(in.Data) == 0 {
		return p, errMCPWriteInput
	}
	create := strings.HasPrefix(in.Operation, "create_")
	cleansLinks := false
	if (create && in.ID != "") || (!create && !validWriteID(in.ID)) {
		return p, errMCPWriteInput
	}
	if in.Operation != "create_manual_transaction" && in.IdempotencyKey != "" {
		return p, errMCPWriteInput
	}
	if !strings.HasSuffix(in.Operation, "planned_transaction") && in.ScenarioID != "" {
		return p, errMCPWriteInput
	}
	add := func(v any) error {
		raw, err := json.Marshal(v)
		if err == nil {
			p.Before = append(p.Before, raw)
		}
		return err
	}
	switch in.Operation {
	case "create_manual_transaction", "update_manual_transaction":
		var req manualTransactionRequest
		if err := decodeMCPWrite(in.Data, &req); err != nil {
			return p, err
		}
		// Exact, bounded plain decimals only; JSON numeric amounts are not accepted.
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(in.Data, &fields)
		var amount string
		if json.Unmarshal(fields["amount"], &amount) != nil || len(amount) > 64 || !plainWriteDecimal.MatchString(amount) || !validWriteID(req.AccountID) || !validOptionalWriteID(req.CategoryID) || len(req.Description) > 1000 {
			return p, errMCPWriteInput
		}
		if _, ok := req.toManualInput(); !ok {
			return p, errMCPWriteInput
		}
		if create && !validManualIdempotencyKey(in.IdempotencyKey) {
			return p, errMCPWriteInput
		}
		if req.CategoryID != nil {
			c, err := categories.Get(ctx, q, *req.CategoryID)
			if err != nil {
				return p, err
			}
			if !c.IsActive {
				return p, categories.ErrCategoryInvalid
			}
			_ = add(toCategoryDTO(c))
		}
		var account string
		var accountName, currency sql.NullString
		if err := q.QueryRowContext(ctx, `SELECT id,name,currency_code FROM financial_accounts WHERE id=? AND archived_at IS NULL`, req.AccountID).Scan(dbtypes.ID(&account), &accountName, &currency); err != nil {
			return p, err
		}
		_ = add(map[string]any{"account_id": account, "account_name": accountName.String, "currency_code": currency.String})
		if create {
			rules, err := automation.List(ctx, q)
			if err != nil {
				return p, err
			}
			if err = add(rules); err != nil {
				return p, err
			}
		}
		p.Effect = "Cria ou altera um lançamento manual com valor assinado exato (positivo: entrada; negativo: saída). Contas manuais recalculam o saldo; saldos de contas sincronizadas continuam definidos pelo provedor. Novos lançamentos seguem as regras de categorização e automação do Julius."
	case "delete_manual_transaction":
		cleansLinks = true
		if string(in.Data) != "{}" {
			return p, errMCPWriteInput
		}
		p.Effect = "Exclui o lançamento manual dos relatórios e recalcula o saldo de contas manuais e as faturas. Desvincula pagamentos de pendências, ocorrências recorrentes e transferências entre contas próprias. As contrapartidas são preservadas e passam a seguir suas decisões de categoria e inclusão. O histórico de decisões é preservado."
	case "assign_transaction_category":
		var req transactionCategoryRequest
		if err := decodeMCPWrite(in.Data, &req); err != nil {
			return p, err
		}
		if !validWriteID(req.CategoryID) {
			return p, errMCPWriteInput
		}
		c, err := categories.Get(ctx, q, req.CategoryID)
		if err != nil {
			return p, err
		}
		if !c.IsActive {
			return p, categories.ErrCategoryInvalid
		}
		_ = add(toCategoryDTO(c))
		ids, err := categories.ManualAssignmentIDs(ctx, q, in.ID)
		if err != nil {
			return p, err
		}
		p.AffectedIDs = ids
		p.Effect = "Aplica a categoria manual a todas as parcelas listadas, substituindo decisões anteriores. Categoria de transferência exclui os valores dos totais de receitas e despesas, sem alterar saldos. A decisão pode ensinar a categoria a lançamentos futuros semelhantes."
	case "set_transaction_inclusion":
		var req transactionInclusionRequest
		if err := decodeMCPWrite(in.Data, &req); err != nil {
			return p, err
		}
		if req.State != "considered" && req.State != "ignored" {
			return p, errMCPWriteInput
		}
		cleansLinks = req.State == "ignored"
		p.Effect = "Altera a inclusão nos totais de receitas e despesas. Ignorar não muda o saldo bancário; remove vínculos com pendências, recorrências e investimentos. Também desfaz o vínculo de transferência entre contas próprias; as contrapartidas preservadas passam a seguir suas decisões de categoria e inclusão. Reconsiderar não restaura esses vínculos."
		if !cleansLinks {
			p.Effect = "Marca o lançamento como considerado nos totais, respeitando as demais regras de elegibilidade e categoria. Não altera o saldo bancário nem restaura vínculos anteriormente removidos."
		}
	case "create_category":
		var req categoryCreateRequest
		if err := decodeMCPWrite(in.Data, &req); err != nil {
			return p, err
		}
		if strings.TrimSpace(req.Name) == "" || len(req.Name) > 200 || !validCategoryKinds[req.Kind] || !validCategoryIcons[req.Icon] || !validCategoryColors[req.Color] {
			return p, errMCPWriteInput
		}
		p.Effect = "Cria uma categoria ativa no catálogo. Nenhum lançamento existente é alterado."
	case "update_category":
		var req categoryUpdateRequest
		if err := decodeMCPWrite(in.Data, &req); err != nil {
			return p, err
		}
		if (req.Name == nil && req.IsActive == nil && req.Icon == nil && req.Color == nil) || (req.Name != nil && (strings.TrimSpace(*req.Name) == "" || len(*req.Name) > 200)) || (req.Icon != nil && !validCategoryIcons[*req.Icon]) || (req.Color != nil && !validCategoryColors[*req.Color]) {
			return p, errMCPWriteInput
		}
		c, err := categories.Get(ctx, q, in.ID)
		if err != nil {
			return p, err
		}
		_ = add(toCategoryDTO(c))
		p.Effect = "Atualiza o catálogo, incluindo o nome exibido nos lançamentos já categorizados. Desativar impede novas atribuições; preserva as decisões existentes."
	case "create_automation_rule", "update_automation_rule":
		var req ruleWriteRequest
		if err := decodeMCPWrite(in.Data, &req); err != nil {
			return p, err
		}
		if req.ApplyRetroactively || len(req.Name) > 200 || len(req.Conditions) > 20 || len(req.Actions) > 10 {
			return p, errMCPWriteInput
		}
		write, ok := req.toWrite()
		if !ok {
			return p, errMCPWriteInput
		}
		for _, c := range req.Conditions {
			if len(c.Value) > 1000 {
				return p, errMCPWriteInput
			}
		}
		for _, a := range write.Actions {
			if !validOptionalWriteID(a.CategoryID) || !validOptionalWriteID(a.ScenarioID) {
				return p, errMCPWriteInput
			}
			if a.CategoryID != nil {
				c, err := categories.Get(ctx, q, *a.CategoryID)
				if err != nil {
					return p, err
				}
				if !c.IsActive {
					return p, categories.ErrCategoryInvalid
				}
				_ = add(toCategoryDTO(c))
			}
		}
		p.Effect = "Altera a automação de lançamentos futuros ou atualizados pela sincronização, respeitando decisões manuais. Pode categorizar, ignorar (desvinculando pagamentos e investimentos) ou reconciliar recorrências. Não aplica retroativamente."
	case "delete_automation_rule":
		if string(in.Data) != "{}" {
			return p, errMCPWriteInput
		}
		p.Effect = "Exclui a regra e suas condições e ações. Decisões anteriores de categoria e inclusão permanecem; a reconciliação automática dependente da regra deixa de ocorrer."
	default:
		return previewMCPPlanningWrite(ctx, q, p)
	}
	if !create {
		if len(p.AffectedIDs) == 0 {
			p.AffectedIDs = []string{in.ID}
		}
		if strings.Contains(in.Operation, "transaction") {
			sort.Strings(p.AffectedIDs)
			if len(p.AffectedIDs) > 100 {
				return p, errors.New("installment cascade exceeds 100 transactions; use Julius")
			}
			for _, id := range p.AffectedIDs {
				item, found, err := transactions.GetItem(ctx, q, id)
				if err != nil {
					return p, err
				}
				if !found {
					return p, transactions.ErrTransactionNotFound
				}
				if (in.Operation == "update_manual_transaction" || in.Operation == "delete_manual_transaction") && item.Origin != "manual" {
					return p, transactions.ErrNotManual
				}
				// Manual operations must never gain an implicit installment cascade.
				if in.Operation == "update_manual_transaction" {
					ids, err := categories.ManualAssignmentIDs(ctx, q, id)
					if err != nil {
						return p, err
					}
					if len(ids) != 1 {
						return p, errMCPWriteInput
					}
				}
				if cleansLinks {
					transfer, err := transactions.GetTransferForTransaction(ctx, q, id)
					if err != nil {
						return p, err
					}
					if transfer != nil {
						if err = add(transfer); err != nil {
							return p, err
						}
						participants := []transactions.TransferMovement{transfer.Source, transfer.Destination}
						if transfer.Fee != nil {
							participants = append(participants, *transfer.Fee)
						}
						for _, movement := range participants {
							if movement.TransactionID == id {
								continue
							}
							p.AffectedIDs = append(p.AffectedIDs, movement.TransactionID)
							counterpart, found, err := transactions.GetItem(ctx, q, movement.TransactionID)
							if err != nil {
								return p, err
							}
							if !found {
								return p, transactions.ErrTransactionNotFound
							}
							if err = add(toItemDTO(counterpart)); err != nil {
								return p, err
							}
						}
					}

					links, err := mcpTransactionLinks(ctx, q, id)
					if err != nil {
						return p, err
					}
					if err = add(links); err != nil {
						return p, err
					}
				}
				if err = add(toItemDTO(item)); err != nil {
					return p, err
				}
			}
		}
		if strings.HasSuffix(in.Operation, "automation_rule") {
			rule, err := automation.Get(ctx, q, in.ID)
			if err != nil {
				return p, err
			}
			_ = add(toRuleDTO(rule))
		}
	}
	sort.Strings(p.AffectedIDs)
	p.AffectedCount = len(p.AffectedIDs)
	if create {
		p.AffectedCount = 1
	}
	return p, nil
}

// MCPWriteResult retains the REST Problem/result contract without exposing routing.
type MCPWriteResult struct {
	Status int             `json:"status"`
	Data   json.RawMessage `json:"data,omitempty"`
}

type mcpWriteResponse struct {
	header http.Header
	bytes.Buffer
	status int
}

func (w *mcpWriteResponse) Header() http.Header    { return w.header }
func (w *mcpWriteResponse) WriteHeader(status int) { w.status = status }

func ExecuteMCPWrite(ctx context.Context, conn *sql.DB, approved MCPWritePreview) (MCPWriteResult, error) {
	in := approved.Request
	if in.Operation == "delete_scenario" || in.Operation == "delete_planned_transaction" {
		tx, err := conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			return MCPWriteResult{}, err
		}
		defer tx.Rollback()
		if err = checkMCPPreview(ctx, tx, approved); err != nil {
			return MCPWriteResult{}, err
		}
		if in.Operation == "delete_scenario" {
			err = scenarios.DeleteScenario(ctx, tx, in.ID)
		} else {
			err = scenarios.DeleteScenarioTransaction(ctx, tx, in.ID)
		}
		if err != nil {
			return MCPWriteResult{}, err
		}
		if err = tx.Commit(); err != nil {
			return MCPWriteResult{}, err
		}
		return MCPWriteResult{Status: http.StatusNoContent}, nil
	}
	// Category propagation is a bulk mutation: recheck its entire preview inside
	// the same transaction as all category decisions and events.
	if in.Operation == "assign_transaction_category" {
		tx, err := conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			return MCPWriteResult{}, err
		}
		defer tx.Rollback()
		if err = checkMCPPreview(ctx, tx, approved); err != nil {
			return MCPWriteResult{}, err
		}
		var req transactionCategoryRequest
		_ = json.Unmarshal(in.Data, &req)
		decision, err := categories.AssignManualWithQuerier(ctx, tx, in.ID, req.CategoryID)
		if err != nil {
			return MCPWriteResult{}, err
		}
		if err = tx.Commit(); err != nil {
			return MCPWriteResult{}, err
		}
		raw, _ := json.Marshal(decision)
		return MCPWriteResult{Status: 200, Data: raw}, nil
	}
	if err := checkMCPPreview(ctx, conn, approved); err != nil {
		return MCPWriteResult{}, err
	}
	svc := &ledger.Service{DB: conn, OnIgnored: ledger.OnIgnored}
	handlers := map[string]http.HandlerFunc{
		"create_manual_transaction": handleCreateManualTransaction(svc), "update_manual_transaction": handleUpdateManualTransaction(svc), "delete_manual_transaction": handleDeleteManualTransaction(svc),
		"set_transaction_inclusion": handleSetTransactionInclusion(conn), "create_category": handleCreateCategory(conn), "update_category": handleUpdateCategory(conn),
		"create_automation_rule": handleCreateAutomationRule(conn), "update_automation_rule": handleUpdateAutomationRule(conn), "delete_automation_rule": handleDeleteAutomationRule(conn),
		"create_goal": handleCreateGoal(conn), "update_goal": handleUpdateGoal(conn), "delete_goal": handleDeleteGoal(conn),
		"create_scenario":            handleCreateStandaloneScenario(conn),
		"create_planned_transaction": handleCreateScenarioTransaction(conn), "update_planned_transaction": handleUpdateScenarioTransaction(conn),
		"create_recurring_commitment": handleCreateRecurringCommitment(conn), "update_recurring_commitment": handleUpdateRecurringCommitment(conn), "delete_recurring_commitment": handleDeleteRecurringCommitment(conn),
	}
	if in.Operation == "update_scenario" {
		var req scenarioCreateRequest
		_ = json.Unmarshal(in.Data, &req)
		s, err := scenarios.RenameScenario(ctx, conn, in.ID, req.Name)
		if err != nil {
			return MCPWriteResult{}, err
		}
		raw, err := json.Marshal(scenarioToDTO(s))
		return MCPWriteResult{Status: http.StatusOK, Data: raw}, err
	}
	handler, ok := handlers[in.Operation]
	if !ok {
		return MCPWriteResult{}, errMCPWriteInput
	}
	r, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/", bytes.NewReader(in.Data))
	r.SetPathValue("id", in.ID)
	if in.ScenarioID != "" {
		r.SetPathValue("id", in.ScenarioID)
		r.SetPathValue("transactionId", in.ID)
	}
	r.Header.Set("Content-Type", "application/json")
	if in.IdempotencyKey != "" {
		r.Header.Set("Idempotency-Key", in.IdempotencyKey)
	}
	w := &mcpWriteResponse{header: make(http.Header), status: 200}
	handler(w, r)
	return MCPWriteResult{Status: w.status, Data: append(json.RawMessage(nil), w.Bytes()...)}, nil
}
func checkMCPPreview(ctx context.Context, q transactions.Querier, approved MCPWritePreview) error {
	current, err := PreviewMCPWrite(ctx, q, approved.Request)
	if err != nil {
		return err
	}
	a, _ := json.Marshal(approved)
	b, _ := json.Marshal(current)
	if !bytes.Equal(a, b) {
		return ErrMCPWriteConflict
	}
	return nil
}

// MCPWriteError only exposes domain errors; storage errors never reach clients.
func MCPWriteError(err error) string {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "referenced record not found; check the category or account identifier and request a new preview"
	case errors.Is(err, goals.ErrNotFound), errors.Is(err, goals.ErrInvalidInput), errors.Is(err, scenarios.ErrScenarioNotFound), errors.Is(err, scenarios.ErrTransactionNotFound), errors.Is(err, recurrences.ErrNotFound):
		return err.Error() + "; check the identifier and planning fields, then request a new preview"
	case errors.Is(err, recurrences.ErrLinkedToAutomationRule), errors.Is(err, scenarios.ErrScenarioLinkedToAutomationRule):
		return "recurring commitment is targeted by an automation rule; remove its reconcile action before deleting"
	case errors.Is(err, errMCPWriteInput), errors.Is(err, ErrMCPWriteConflict), errors.Is(err, transactions.ErrNotManual), errors.Is(err, transactions.ErrTransactionNotFound), errors.Is(err, categories.ErrCategoryInvalid), errors.Is(err, categories.ErrNotFound), errors.Is(err, automation.ErrNotFound):
		return err.Error()
	default:
		return "financial write unavailable; request a new preview"
	}
}

// MCPWriteInputSchema publishes the same body fields as the corresponding REST handler.
func MCPWriteInputSchema(operation string) any {
	var typ reflect.Type
	switch operation {
	case "create_manual_transaction", "update_manual_transaction":
		typ = reflect.TypeFor[manualTransactionRequest]()
	case "assign_transaction_category":
		typ = reflect.TypeFor[transactionCategoryRequest]()
	case "set_transaction_inclusion":
		typ = reflect.TypeFor[transactionInclusionRequest]()
	case "create_category":
		typ = reflect.TypeFor[categoryCreateRequest]()
	case "update_category":
		typ = reflect.TypeFor[categoryUpdateRequest]()
	case "create_automation_rule", "update_automation_rule":
		typ = reflect.TypeFor[ruleWriteRequest]()
	case "create_goal", "update_goal":
		typ = reflect.TypeFor[goalRequest]()
	case "create_scenario", "update_scenario":
		typ = reflect.TypeFor[scenarioCreateRequest]()
	case "create_planned_transaction", "update_planned_transaction":
		typ = reflect.TypeFor[scenarioTransactionWriteRequest]()
	case "create_recurring_commitment", "update_recurring_commitment":
		typ = reflect.TypeFor[recurringCommitmentWriteRequest]()
	default:
		typ = reflect.TypeFor[struct{}]()
	}
	schema, err := jsonschema.ForType(typ, &jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{reflect.TypeFor[decimal.Decimal](): {Type: "string", Description: "Signed exact decimal string, nonzero, at most 64 characters; no exponent."}}})
	if err != nil {
		panic(err)
	}
	// REST PATCH fields and nullable optional category selection remain optional.
	if operation == "update_category" {
		schema.Required = nil
	}
	if operation == "create_goal" || operation == "update_goal" {
		schema.Properties["target_amount"] = &jsonschema.Schema{Type: "string", Description: "Positive exact BRL decimal string; at most 64 characters, no exponent."}
		schema.Required = []string{"name", "metric", "target_amount"}
	}
	if operation == "create_recurring_commitment" || operation == "update_recurring_commitment" {
		schema.Required = []string{"name", "kind", "amount", "category_id", "cadence", "day_of_month", "start_date", "is_active"}
	}
	if operation == "create_planned_transaction" || operation == "update_planned_transaction" {
		schema.Required = []string{"description", "amount", "projected_at"}
	}
	if operation == "create_manual_transaction" || operation == "update_manual_transaction" {
		schema.Required = []string{"account_id", "description", "amount", "occurred_at"}
	}
	if strings.HasSuffix(operation, "automation_rule") && !strings.HasPrefix(operation, "delete") {
		schema.Required = []string{"name", "is_active", "logic_operator", "conditions", "actions"}
	}
	var optionalTargets func(*jsonschema.Schema)
	optionalTargets = func(s *jsonschema.Schema) {
		if s == nil {
			return
		}
		if s.Properties["scenario_id"] != nil && s.Properties["type"] != nil {
			s.Required = []string{"type"}
		}
		for _, child := range s.Properties {
			optionalTargets(child)
		}
		for _, child := range s.Defs {
			optionalTargets(child)
		}
		optionalTargets(s.Items)
	}
	optionalTargets(schema)
	properties := map[string]any{"data": schema}
	required := []string{"data"}
	if strings.HasSuffix(operation, "planned_transaction") {
		properties["scenario_id"] = map[string]any{"type": "string", "description": "Parent standalone scenario UUID; the transaction must belong to it."}
		required = append(required, "scenario_id")
	}
	if !strings.HasPrefix(operation, "create_") {
		properties["id"] = map[string]any{"type": "string", "description": "Existing UUID"}
		required = append(required, "id")
	}
	if operation == "create_manual_transaction" {
		properties["idempotency_key"] = map[string]any{"type": "string", "minLength": 8, "maxLength": 128}
		required = append(required, "idempotency_key")
	}
	defs := schema.Defs
	schema.Defs = nil
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false, "$defs": defs}
}

func mcpTransactionLinks(ctx context.Context, q transactions.Querier, id string) ([]map[string]string, error) {
	links := []map[string]string{}
	for _, source := range []struct{ kind, query string }{
		{"payable", `SELECT id,payable_id,linked_amount FROM payable_transaction_links WHERE transaction_id=? ORDER BY id`},
		{"scenario", `SELECT id,scenario_id,COALESCE(linked_amount,allocated_amount,0) FROM scenario_realizations WHERE transaction_id=? ORDER BY id`},
		{"investment", `SELECT id,operation_id,amount FROM investment_reconciliations WHERE financial_transaction_id=? ORDER BY id`},
	} {
		rows, err := q.QueryContext(ctx, source.query, id)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var link, target, amount string
			if err = rows.Scan(dbtypes.ID(&link), dbtypes.ID(&target), dbtypes.Decimal(&amount)); err != nil {
				rows.Close()
				return nil, err
			}
			links = append(links, map[string]string{"kind": source.kind, "link_id": link, "target_id": target, "amount": amount})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return links, nil
}
