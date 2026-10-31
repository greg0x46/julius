package httpapi

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/greg0x46/julius/internal/automation"
	"github.com/greg0x46/julius/internal/categories"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/goals"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/scenarios"
	"github.com/greg0x46/julius/internal/settings"
	"github.com/greg0x46/julius/internal/transactions"
)

func planningWriteAmount(data json.RawMessage, field string) bool {
	var fields map[string]json.RawMessage
	var amount string
	return json.Unmarshal(data, &fields) == nil && json.Unmarshal(fields[field], &amount) == nil && len(amount) <= 64 && plainWriteDecimal.MatchString(amount)
}

func previewMCPPlanningWrite(ctx context.Context, q transactions.Querier, p MCPWritePreview) (MCPWritePreview, error) {
	in := p.Request
	create := strings.HasPrefix(in.Operation, "create_")
	deleting := strings.HasPrefix(in.Operation, "delete_")
	if deleting && string(in.Data) != "{}" {
		return p, errMCPWriteInput
	}
	add := func(value any) error {
		raw, err := json.Marshal(value)
		if err == nil {
			p.Before = append(p.Before, raw)
		}
		return err
	}
	category := func(id string) error {
		if !validWriteID(id) {
			return errMCPWriteInput
		}
		c, err := categories.Get(ctx, q, id)
		if err != nil {
			return err
		}
		return add(toCategoryDTO(c))
	}
	switch in.Operation {
	case "create_goal", "update_goal", "delete_goal":
		if !deleting {
			if !planningWriteAmount(in.Data, "target_amount") {
				return p, goals.ErrInvalidInput
			}
			var req goalRequest
			if err := decodeMCPWrite(in.Data, &req); err != nil {
				return p, err
			}
			input, ok := req.toInput()
			if !ok || strings.TrimSpace(req.Name) == "" || len(req.Name) > 200 || (req.Notes != nil && len(*req.Notes) > 4000) || input.Metric != goals.MetricInvestedValue || !input.TargetAmount.IsPositive() {
				return p, goals.ErrInvalidInput
			}
		}
		if !create {
			loc, err := settings.Location(ctx, q)
			if err != nil {
				return p, err
			}
			g, err := goals.Get(ctx, q, in.ID, loc)
			if err != nil {
				return p, err
			}
			// Only stored decisions are approved; progress remains derived on read.
			if err = add(map[string]any{"id": g.ID, "name": g.Name, "metric": g.Metric, "target_amount": g.TargetAmount.String(), "target_date": goalToDTO(g).TargetDate, "notes": g.Notes, "updated_at": g.UpdatedAt}); err != nil {
				return p, err
			}
		}
		p.Effect = "Cria, altera ou exclui apenas a meta em BRL. O progresso e a contribuição estimada são recalculados pelos investimentos reais; nenhum dinheiro ou investimento é movimentado."
	case "create_recurring_commitment", "update_recurring_commitment", "delete_recurring_commitment":
		if !deleting {
			var req recurringCommitmentWriteRequest
			if err := decodeMCPWrite(in.Data, &req); err != nil {
				return p, err
			}
			if !planningWriteAmount(in.Data, "amount") || strings.TrimSpace(req.Name) == "" || len(req.Name) > 200 || !validOptionalWriteID(req.AccountID) {
				return p, errMCPWriteInput
			}
			if _, err := req.toWrite(); err != nil {
				return p, errMCPWriteInput
			}
			if err := category(req.CategoryID); err != nil {
				return p, err
			}
			if req.AccountID != nil {
				var id string
				if err := q.QueryRowContext(ctx, `SELECT id FROM financial_accounts WHERE id=? AND archived_at IS NULL`, *req.AccountID).Scan(dbtypes.ID(&id)); err != nil {
					return p, err
				}
				if err := add(map[string]string{"account_id": id}); err != nil {
					return p, err
				}
			}
		}
		if !create {
			c, err := recurrences.Get(ctx, q, in.ID)
			if err != nil {
				return p, err
			}
			if err = add(toRecurringCommitmentDTO(c)); err != nil {
				return p, err
			}
			links, err := scenarios.ListRealizations(ctx, q, in.ID)
			if err != nil {
				return p, err
			}
			if err = add(map[string]any{"realizations": links}); err != nil {
				return p, err
			}
			rules, err := automation.List(ctx, q)
			if err != nil {
				return p, err
			}
			for _, rule := range rules {
				for _, action := range rule.Actions {
					if action.ScenarioID != nil && *action.ScenarioID == in.ID {
						if deleting {
							return p, recurrences.ErrLinkedToAutomationRule
						}
						if err = add(toRuleDTO(rule)); err != nil {
							return p, err
						}
						break
					}
				}
			}
		}
		p.Effect = "Altera compromissos previstos em BRL. Quando ativo, o compromisso entra na projeção base; ocorrências conciliadas não são contadas novamente. Alterar o calendário remove decisões manuais de datas que deixam de ser ocorrências. Excluir remove a agenda e suas decisões de conciliação. Os lançamentos reais são preservados."
	case "create_scenario", "update_scenario", "delete_scenario", "create_planned_transaction", "update_planned_transaction", "delete_planned_transaction":
		child := strings.HasSuffix(in.Operation, "planned_transaction")
		scenarioID := in.ID
		if child {
			if !validWriteID(in.ScenarioID) {
				return p, errMCPWriteInput
			}
			scenarioID = in.ScenarioID
			if !deleting {
				var req scenarioTransactionWriteRequest
				if !planningWriteAmount(in.Data, "amount") {
					return p, errMCPWriteInput
				}
				if err := decodeMCPWrite(in.Data, &req); err != nil {
					return p, err
				}
				if _, ok := req.validate(); !ok || strings.TrimSpace(req.Description) == "" || len(req.Description) > 1000 {
					return p, errMCPWriteInput
				}
				if req.Category != nil {
					if err := category(*req.Category); err != nil {
						return p, err
					}
				}
			}
		} else if !deleting {
			var req scenarioCreateRequest
			if err := decodeMCPWrite(in.Data, &req); err != nil {
				return p, err
			}
			if strings.TrimSpace(req.Name) == "" || len(req.Name) > 200 {
				return p, errMCPWriteInput
			}
		}
		if !create || child {
			s, err := scenarios.GetScenario(ctx, q, scenarioID)
			if err != nil {
				return p, err
			}
			if s.Kind != scenarios.KindStandalone {
				return p, errMCPWriteInput
			}
			if err = add(scenarioToDTO(s)); err != nil {
				return p, err
			}
			if child && !create {
				st, err := scenarios.GetScenarioTransaction(ctx, q, in.ID)
				if err != nil {
					return p, err
				}
				if st.ScenarioID != scenarioID {
					return p, scenarios.ErrTransactionNotFound
				}
				if err = add(map[string]any{"id": st.ID, "scenario_id": st.ScenarioID, "description": st.Description, "amount": st.Amount.String(), "projected_at": st.ProjectedAt.Format(dateOnlyLayout), "category": st.Category, "updated_at": st.UpdatedAt}); err != nil {
					return p, err
				}
			}
			if deleting && !child {
				list, err := scenarios.ListScenarioTransactions(ctx, q, scenarioID)
				if err != nil {
					return p, err
				}
				for _, st := range list {
					p.AffectedIDs = append(p.AffectedIDs, st.ID)
					if err = add(map[string]any{"id": st.ID, "scenario_id": st.ScenarioID, "description": st.Description, "amount": st.Amount.String(), "projected_at": st.ProjectedAt.Format(dateOnlyLayout), "category": st.Category, "updated_at": st.UpdatedAt}); err != nil {
						return p, err
					}
				}
			}
			links, err := scenarios.ListRealizations(ctx, q, scenarioID)
			if err != nil {
				return p, err
			}
			if err = add(map[string]any{"realizations": links}); err != nil {
				return p, err
			}
		}
		p.Effect = "Altera somente um cenário hipotético ou seus lançamentos planejados em BRL (negativo: saída; positivo: entrada). No MCP, o cenário afeta a simulação apenas quando seu ID é selecionado explicitamente em scenario_ids. Excluir remove os dados planejados e seus vínculos; não exclui nem altera lançamentos reais, saldos ou totais realizados."
	default:
		return p, errMCPWriteInput
	}
	if !create {
		p.AffectedIDs = append(p.AffectedIDs, in.ID)
	}
	p.AffectedCount = len(p.AffectedIDs)
	if create {
		p.AffectedCount = 1
	}
	return p, nil
}
