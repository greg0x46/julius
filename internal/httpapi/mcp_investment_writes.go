package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/transactions"
)

func investmentWrite(operation string) bool {
	switch operation {
	case "create_investment_account", "update_investment_account", "delete_investment_account",
		"create_investment_position", "update_investment_position", "delete_investment_position",
		"create_investment_operation", "update_investment_operation", "delete_investment_operation":
		return true
	}
	return false
}

func validCustodyID(id string) bool { return validWriteID(strings.TrimPrefix(id, "integrated:")) }

func previewMCPInvestmentWrite(ctx context.Context, q transactions.Querier, p MCPWritePreview) (MCPWritePreview, error) {
	in := p.Request
	create, deleting := strings.HasPrefix(in.Operation, "create_"), strings.HasPrefix(in.Operation, "delete_")
	if !investmentWrite(in.Operation) {
		return p, errMCPWriteInput
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(in.Data, &fields) != nil || fields == nil {
		return p, errMCPWriteInput
	}
	if deleting && len(fields) != 0 {
		return p, errMCPWriteInput
	}
	for _, key := range []string{"amount", "quantity", "unit_price", "fees", "taxes", "initial_quantity", "initial_unit_cost", "initial_value"} {
		if raw, ok := fields[key]; ok && string(raw) != "null" {
			var value string
			if json.Unmarshal(raw, &value) != nil || len(value) > 64 || !plainWriteDecimal.MatchString(value) {
				return p, errMCPWriteInput
			}
		}
	}
	for _, key := range []string{"name", "notes", "ticker", "asset_type"} {
		if raw, ok := fields[key]; ok && string(raw) != "null" {
			var value string
			if json.Unmarshal(raw, &value) != nil || len(value) > 1000 {
				return p, errMCPWriteInput
			}
		}
	}
	add := func(value any) error {
		raw, err := json.Marshal(value)
		if err == nil {
			p.Before = append(p.Before, raw)
		}
		return err
	}
	accounts := map[string]bool{}
	var accountIDs []string
	account := func(id string) error {
		if !validCustodyID(id) {
			return errMCPWriteInput
		}
		if !accounts[id] {
			accounts[id] = true
			accountIDs = append(accountIDs, id)
		}
		return nil
	}
	position := func(id string) error {
		if !validWriteID(id) {
			return errMCPWriteInput
		}
		pos, err := investments.GetPosition(ctx, q, id)
		if err != nil {
			return err
		}
		if pos.Source != investments.PositionSourceManual {
			return investments.ErrIntegratedReadOnly
		}
		if err = add(investmentPositionToDTO(pos)); err != nil {
			return err
		}
		return account(pos.AccountID)
	}
	switch {
	case strings.HasSuffix(in.Operation, "_account"):
		if !create {
			if err := account(in.ID); err != nil {
				return p, err
			}
		}
		if !deleting {
			if create {
				var req investmentAccountCreateRequest
				if err := decodeMCPWrite(in.Data, &req); err != nil {
					return p, err
				}
				if strings.TrimSpace(req.Name) == "" || (req.Kind != nil && *req.Kind != "manual") || (req.CurrencyCode != nil && *req.CurrencyCode != "BRL") || req.SourceID != nil || !validOptionalWriteID(req.FinancialAccountID) {
					return p, errMCPWriteInput
				}
			} else {
				var req investmentAccountUpdateRequest
				if err := decodeMCPWrite(in.Data, &req); err != nil {
					return p, err
				}
				if len(fields) == 0 || (req.Name != nil && strings.TrimSpace(*req.Name) == "") || !validOptionalWriteID(req.FinancialAccountID.value) {
					return p, errMCPWriteInput
				}
			}
			if raw := fields["financial_account_id"]; len(raw) > 0 && string(raw) != "null" {
				var id string
				_ = json.Unmarshal(raw, &id)
				balance, err := transactions.AccountBalance(ctx, q, id)
				if err != nil {
					return p, err
				}
				if err = add(map[string]any{"financial_account_id": id, "balance": balance}); err != nil {
					return p, err
				}
			}
		}
		p.Effect = "Altera a conta de custódia e seu vínculo bancário. O vínculo define o saldo disponível e evita contá-lo duas vezes no patrimônio. Contas integradas preservam os dados do provedor. Excluir exige uma conta manual sem posições nem operações."
	case strings.HasSuffix(in.Operation, "_position"):
		if !create {
			if err := position(in.ID); err != nil {
				return p, err
			}
		}
		if !deleting {
			if create {
				var req investmentPositionCreateRequest
				if err := decodeMCPWrite(in.Data, &req); err != nil {
					return p, err
				}
				if err := account(req.AccountID); err != nil {
					return p, err
				}
				if !validOptionalWriteID(req.AssetID) || (req.AssetID == nil && strings.TrimSpace(req.Name) == "") {
					return p, errMCPWriteInput
				}
				if _, ok := investmentBodyDate(req.OccurredOn); !ok {
					return p, errMCPWriteInput
				}
				if req.AssetID != nil {
					asset, err := investments.GetAsset(ctx, q, *req.AssetID)
					if err != nil {
						return p, err
					}
					if err = add(investmentAssetToDTO(asset)); err != nil {
						return p, err
					}
				}
			} else {
				var req investmentPositionUpdateRequest
				if err := decodeMCPWrite(in.Data, &req); err != nil {
					return p, err
				}
				if len(fields) == 0 {
					return p, errMCPWriteInput
				}
				// Instrument descriptions are shared across all local holdings of the asset.
				current, err := investments.GetPosition(ctx, q, in.ID)
				if err != nil {
					return p, err
				}
				positions, err := investments.ListPositions(ctx, q, investments.PositionFilter{IncludeClosed: true})
				if err != nil {
					return p, err
				}
				for _, pos := range positions {
					if current.AssetID != nil && pos.AssetID != nil && *pos.AssetID == *current.AssetID {
						p.AffectedIDs = append(p.AffectedIDs, pos.ID)
						if err = account(pos.AccountID); err != nil {
							return p, err
						}
					}
				}
			}
		}
		p.Effect = "Altera uma posição manual em BRL, inclusive dentro de uma custódia integrada. A descrição do ativo é compartilhada pelas posições listadas. O saldo inicial entra no custo de aquisição; compras, vendas e cotações recalculam o resultado. Excluir exige uma posição sem operações. Posições do provedor são preservadas."
	case strings.HasSuffix(in.Operation, "_operation"):
		if !create {
			op, err := investments.GetOperation(ctx, q, in.ID)
			if err != nil {
				return p, err
			}
			if op.Source != "manual" {
				return p, investments.ErrNotManual
			}
			if op.TransferID != nil {
				return p, investments.ErrTransferAtomic
			}
			if err = add(investmentOperationToDTO(op)); err != nil {
				return p, err
			}
			if err = account(op.AccountID); err != nil {
				return p, err
			}
		}
		if !deleting {
			var req investmentOperationRequest
			if err := decodeMCPWrite(in.Data, &req); err != nil {
				return p, err
			}
			input, ok := req.toInput()
			if !ok || (!input.Kind.Valid() || input.Kind == investments.OperationTransferIn || input.Kind == investments.OperationTransferOut) {
				return p, errMCPWriteInput
			}
			if err := account(req.AccountID); err != nil {
				return p, err
			}
			if req.PositionID != nil {
				if err := position(*req.PositionID); err != nil {
					return p, err
				}
			}
		}
		p.Effect = "Registra, corrige ou exclui uma operação manual em BRL. Compras e vendas recalculam quantidade, custo de aquisição e rendimento, incluindo taxas e impostos uma única vez. Nas custódias manuais, movimenta o saldo disponível conforme o tipo da operação; nas integradas, preserva o saldo do provedor. Uma correção que deixe saldo ou posição negativos é recusada por inteiro. Operações conciliadas não podem ser excluídas."
	}
	// Capture the ledger, holdings, quotes and reconciliations which determine the approved effect.
	for _, id := range accountIDs {
		a, err := investments.GetAccount(ctx, q, id)
		if err != nil {
			return p, err
		}
		if err = add(investmentAccountToDTO(a)); err != nil {
			return p, err
		}
		positions, err := investments.ListPositions(ctx, q, investments.PositionFilter{AccountID: &id, IncludeClosed: true})
		if err != nil {
			return p, err
		}
		ops, err := investments.ListOperations(ctx, q, investments.OperationFilter{AccountID: &id})
		if err != nil {
			return p, err
		}
		if len(positions)+len(ops) > 500 {
			return p, errMCPWriteInput
		}
		for _, pos := range positions {
			if err = add(investmentPositionToDTO(pos)); err != nil {
				return p, err
			}
		}
		for _, op := range ops {
			if err = add(investmentOperationToDTO(op)); err != nil {
				return p, err
			}
		}
		for _, op := range ops {
			links, err := investments.ListReconciliations(ctx, q, investments.ReconciliationFilter{OperationID: &op.ID})
			if err != nil {
				return p, err
			}
			if err = add(links); err != nil {
				return p, err
			}
		}
	}
	if !create && len(p.AffectedIDs) == 0 {
		p.AffectedIDs = append(p.AffectedIDs, in.ID)
	}
	p.AffectedCount = len(p.AffectedIDs)
	if create {
		p.AffectedCount = 1
	}
	return p, nil
}

func executeMCPInvestmentWrite(ctx context.Context, conn *sql.DB, approved MCPWritePreview) (MCPWriteResult, error) {
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return MCPWriteResult{}, err
	}
	defer tx.Rollback()
	if err = checkMCPPreview(ctx, tx, approved); err != nil {
		return MCPWriteResult{}, err
	}
	in := approved.Request
	result := MCPWriteResult{Status: http.StatusOK}
	if strings.HasPrefix(in.Operation, "create_") {
		result.Status = http.StatusCreated
	}
	if strings.HasPrefix(in.Operation, "delete_") {
		result.Status = http.StatusNoContent
	}
	var value any
	switch in.Operation {
	case "create_investment_account":
		var req investmentAccountCreateRequest
		_ = json.Unmarshal(in.Data, &req)
		var a investments.Account
		a, err = investments.CreateAccount(ctx, tx, investments.AccountInput{Name: req.Name, FinancialAccountID: req.FinancialAccountID})
		value = investmentAccountToDTO(a)
	case "update_investment_account":
		var req investmentAccountUpdateRequest
		_ = json.Unmarshal(in.Data, &req)
		var a investments.Account
		a, err = investments.GetAccount(ctx, tx, in.ID)
		if err == nil {
			name := a.Name
			if req.Name != nil {
				name = *req.Name
			}
			a, err = investments.UpdateAccount(ctx, tx, in.ID, investments.AccountInput{Name: name, Active: req.Active, FinancialAccountID: req.FinancialAccountID.orCurrent(a.FinancialAccountID)})
		}
		value = investmentAccountToDTO(a)
	case "delete_investment_account":
		err = investments.DeleteAccount(ctx, tx, in.ID)
	case "create_investment_position":
		var req investmentPositionCreateRequest
		_ = json.Unmarshal(in.Data, &req)
		day, _ := investmentBodyDate(req.OccurredOn)
		input := investments.PositionInput{AccountID: req.AccountID, AssetID: req.AssetID, Name: req.Name, Ticker: req.Ticker, AssetType: defaultInvestmentAssetType, InitialQuantity: req.InitialQuantity.orZero(), InitialUnitCost: req.InitialUnitCost.orZero(), InitialValue: req.InitialValue.pointer(), OccurredOn: day, Notes: req.Notes}
		if req.AssetType != nil {
			input.AssetType = *req.AssetType
		}
		var pos investments.Position
		pos, err = investments.CreatePositionWithQuerier(ctx, tx, input)
		value = investmentPositionToDTO(pos)
	case "update_investment_position":
		var req investmentPositionUpdateRequest
		_ = json.Unmarshal(in.Data, &req)
		var pos investments.Position
		pos, err = investments.GetPosition(ctx, tx, in.ID)
		if err == nil {
			input := investments.PositionUpdate{Name: pos.Name, Ticker: req.Ticker.orCurrent(pos.Ticker), AssetType: pos.AssetType, Notes: req.Notes.orCurrent(pos.Notes)}
			if req.Name != nil {
				input.Name = *req.Name
			}
			if req.AssetType != nil {
				input.AssetType = *req.AssetType
			}
			pos, err = investments.UpdatePositionWithQuerier(ctx, tx, in.ID, input)
		}
		value = investmentPositionToDTO(pos)
	case "delete_investment_position":
		err = investments.DeletePositionWithQuerier(ctx, tx, in.ID)
	case "create_investment_operation", "update_investment_operation":
		var req investmentOperationRequest
		_ = json.Unmarshal(in.Data, &req)
		input, _ := req.toInput()
		var op investments.Operation
		if in.Operation == "create_investment_operation" {
			var ops []investments.Operation
			ops, err = investments.CreateOperationsWithQuerier(ctx, tx, []investments.OperationInput{input}, nil)
			if err == nil {
				op = ops[0]
			}
		} else {
			op, err = investments.UpdateOperationWithQuerier(ctx, tx, in.ID, input)
		}
		value = investmentOperationToDTO(op)
	case "delete_investment_operation":
		err = investments.DeleteOperationWithQuerier(ctx, tx, in.ID)
	}
	if err != nil {
		return MCPWriteResult{}, err
	}
	if value != nil {
		result.Data, err = json.Marshal(value)
		if err != nil {
			return MCPWriteResult{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return MCPWriteResult{}, err
	}
	return result, nil
}
