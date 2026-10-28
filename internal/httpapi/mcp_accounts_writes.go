package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/greg0x46/julius/internal/automation"
	"github.com/greg0x46/julius/internal/ledger"
	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/scenarios"
	"github.com/greg0x46/julius/internal/transactions"
	"github.com/shopspring/decimal"
)

type manualAccountAdjustmentRequest struct {
	Amount      string `json:"amount"`
	Description string `json:"description"`
}

type transferWriteRequest struct {
	SourceID      string                    `json:"source_transaction_id,omitempty"`
	DestinationID string                    `json:"destination_transaction_id,omitempty"`
	FeeID         string                    `json:"fee_transaction_id,omitempty"`
	Source        *manualTransactionRequest `json:"source,omitempty"`
	Destination   *manualTransactionRequest `json:"destination,omitempty"`
	Fee           *manualTransactionRequest `json:"fee,omitempty"`
}

type payableLinkWriteRequest struct {
	PayableID     string `json:"payable_id"`
	TransactionID string `json:"transaction_id"`
}

type payableUnlinkWriteRequest struct {
	PayableID string `json:"payable_id"`
}

func isMCPAccountsWrite(op string) bool {
	switch op {
	case "create_manual_account", "update_manual_account", "adjust_manual_account", "create_transfer", "delete_transfer", "create_payable", "update_payable", "delete_payable", "create_payable_link", "delete_payable_link":
		return true
	}
	return false
}

func mcpAccount(ctx context.Context, q transactions.Querier, id string) (accountDTO, error) {
	d, err := scanAccount(q.QueryRowContext(ctx, `SELECT `+accountSelectColumns+accountSelectFrom+` WHERE fa.id=?`, id))
	if err != nil {
		return d, err
	}
	d.Balance, err = transactions.AccountBalance(ctx, q, id)
	return d, err
}

func mcpManualAccountInput(data json.RawMessage) (transactions.ManualAccountInput, error) {
	var req manualAccountRequest
	if err := decodeMCPWrite(data, &req); err != nil {
		return transactions.ManualAccountInput{}, err
	}
	if req.CurrencyCode == "" {
		req.CurrencyCode = "BRL"
	}
	if req.OpeningBalance == "" {
		req.OpeningBalance = "0"
	}
	opening, err := decimal.NewFromString(req.OpeningBalance)
	if err != nil || len(req.OpeningBalance) > 64 || !plainWriteDecimal.MatchString(req.OpeningBalance) || strings.TrimSpace(req.Name) == "" || len(req.Name) > 200 || !validManualAccountCurrency(req.CurrencyCode) {
		return transactions.ManualAccountInput{}, errMCPWriteInput
	}
	return transactions.ManualAccountInput{Name: strings.TrimSpace(req.Name), CurrencyCode: req.CurrencyCode, OpeningBalance: opening, Archived: req.Archived}, nil
}

func previewMCPAccountsWrite(ctx context.Context, q transactions.Querier, p MCPWritePreview) (MCPWritePreview, error) {
	in := p.Request
	create := strings.HasPrefix(in.Operation, "create_")
	if strings.HasPrefix(in.Operation, "delete_") && in.Operation != "delete_payable_link" && string(in.Data) != "{}" {
		return p, errMCPWriteInput
	}
	add := func(v any) error {
		raw, err := json.Marshal(v)
		if err == nil {
			p.Before = append(p.Before, raw)
		}
		return err
	}
	account := func(id string, manual, active bool) (accountDTO, error) {
		if !validWriteID(id) {
			return accountDTO{}, errMCPWriteInput
		}
		d, err := mcpAccount(ctx, q, id)
		if err != nil {
			return d, err
		}
		if manual && d.Origin != "manual" {
			return d, transactions.ErrAccountNotManual
		}
		if active && d.ArchivedAt != nil {
			return d, transactions.ErrAccountArchived
		}
		if err = add(d); err != nil {
			return d, err
		}
		p.AffectedIDs = append(p.AffectedIDs, id)
		return d, nil
	}
	movement := func(id string) error {
		if !validWriteID(id) {
			return errMCPWriteInput
		}
		item, found, err := transactions.GetItem(ctx, q, id)
		if err != nil {
			return err
		}
		if !found {
			return transactions.ErrTransactionNotFound
		}
		if err = add(toItemDTO(item)); err != nil {
			return err
		}
		links, err := mcpTransactionLinks(ctx, q, id)
		if err != nil {
			return err
		}
		if err = add(links); err != nil {
			return err
		}
		p.AffectedIDs = append(p.AffectedIDs, id)
		return nil
	}
	snapshotPayable := func(id string) (payables.Payable, error) {
		if !validWriteID(id) {
			return payables.Payable{}, errMCPWriteInput
		}
		item, err := payables.Get(ctx, q, id)
		if err != nil {
			return item, err
		}
		summary, err := payables.Summarize(ctx, q, item)
		if err != nil {
			return item, err
		}
		if err = add(payableToDTO(item, summary)); err != nil {
			return item, err
		}
		if err = add(summary); err != nil {
			return item, err
		}
		// Plans and allocations can be removed by deletion/unlinking.
		plans, err := scenarios.ListScenariosByPayable(ctx, q, id)
		if err != nil {
			return item, err
		}
		for _, plan := range plans {
			if err = add(plan); err != nil {
				return item, err
			}
			p.AffectedIDs = append(p.AffectedIDs, plan.ID)
			entries, err := scenarios.ListScenarioTransactions(ctx, q, plan.ID)
			if err != nil {
				return item, err
			}
			if err = add(entries); err != nil {
				return item, err
			}
			for _, entry := range entries {
				p.AffectedIDs = append(p.AffectedIDs, entry.ID)
			}
			links, err := scenarios.ListRealizations(ctx, q, plan.ID)
			if err != nil {
				return item, err
			}
			if err = add(links); err != nil {
				return item, err
			}
		}
		p.AffectedIDs = append(p.AffectedIDs, id)
		for _, link := range summary.Links {
			p.AffectedIDs = append(p.AffectedIDs, link.Link.TransactionID)
		}
		return item, nil
	}
	switch in.Operation {
	case "create_manual_account", "update_manual_account":
		input, err := mcpManualAccountInput(in.Data)
		if err != nil {
			return p, err
		}
		if !create {
			d, err := account(in.ID, true, false)
			if err != nil {
				return p, err
			}
			if d.HasMovements && (d.CurrencyCode == nil || *d.CurrencyCode != input.CurrencyCode || d.OpeningBalance == nil || !decimal.RequireFromString(*d.OpeningBalance).Equal(input.OpeningBalance)) {
				return p, transactions.ErrAccountHasMovements
			}
		}
		p.Effect = "Cria ou altera uma conta manual na moeda original. Após movimentações, moeda e saldo inicial não podem mudar; use um ajuste. Arquivar impede novas movimentações. Contas sincronizadas continuam definidas pelo provedor."
	case "adjust_manual_account":
		var req manualAccountAdjustmentRequest
		if err := decodeMCPWrite(in.Data, &req); err != nil {
			return p, err
		}
		amount, err := decimal.NewFromString(req.Amount)
		if err != nil || !planningWriteAmount(in.Data, "amount") || amount.IsZero() || strings.TrimSpace(req.Description) == "" || len(req.Description) > 200 {
			return p, errMCPWriteInput
		}
		if _, err = account(in.ID, true, true); err != nil {
			return p, err
		}
		p.Effect = "O ajuste altera o saldo da conta manual na moeda da conta e não entra nas receitas e despesas. O histórico do ajuste é preservado."
	case "create_transfer":
		var req transferWriteRequest
		if err := decodeMCPWrite(in.Data, &req); err != nil {
			return p, err
		}
		if req.Source != nil || req.Destination != nil || req.Fee != nil {
			if req.Source == nil || req.Destination == nil || req.SourceID != "" || req.DestinationID != "" || req.FeeID != "" {
				return p, errMCPWriteInput
			}
			inputs := []*manualTransactionRequest{req.Source, req.Destination}
			if req.Fee != nil {
				inputs = append(inputs, req.Fee)
			}
			currencies := []string{}
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(in.Data, &fields)
			for i, input := range inputs {
				raw := fields[[]string{"source", "destination", "fee"}[i]]
				if _, ok := input.toManualInput(); !ok || !planningWriteAmount(raw, "amount") || input.CategoryID != nil || len(input.Description) > 1000 {
					return p, errMCPWriteInput
				}
				d, err := account(input.AccountID, true, true)
				if err != nil {
					return p, err
				}
				if d.AccountType == nil || *d.AccountType != "BANK" || d.CurrencyCode == nil {
					return p, transactions.ErrInvalidTransfer
				}
				currencies = append(currencies, *d.CurrencyCode)
			}
			sent, received := req.Source.Amount, req.Destination.Amount
			if req.Source.AccountID == req.Destination.AccountID || !sent.IsNegative() || !received.IsPositive() || (currencies[0] == currencies[1] && !sent.Abs().Equal(received)) {
				return p, transactions.ErrInvalidTransfer
			}
			if req.Fee != nil {
				fee := req.Fee.Amount
				if !fee.IsNegative() || (req.Fee.AccountID != req.Source.AccountID && req.Fee.AccountID != req.Destination.AccountID) {
					return p, transactions.ErrInvalidTransfer
				}
			}
			rules, err := automation.List(ctx, q)
			if err != nil {
				return p, err
			}
			if err = add(rules); err != nil {
				return p, err
			}
		} else {
			if !validWriteID(req.SourceID) || !validWriteID(req.DestinationID) || (req.FeeID != "" && !validWriteID(req.FeeID)) {
				return p, errMCPWriteInput
			}
			if err := transactions.ValidateTransfer(ctx, q, transactions.TransferInput{SourceID: req.SourceID, DestinationID: req.DestinationID, FeeID: req.FeeID}); err != nil {
				return p, err
			}
			for _, id := range []string{req.SourceID, req.DestinationID, req.FeeID} {
				if id != "" {
					if err := movement(id); err != nil {
						return p, err
					}
				}
			}
		}
		p.Effect = "O principal da transferência fica fora das receitas e despesas. Os movimentos mantêm seu efeito nos saldos das contas e suas moedas originais; a tarifa é uma despesa separada. Novos movimentos são criados juntos somente em contas manuais; movimentos existentes não são alterados."
	case "delete_transfer":
		transfer, err := transactions.GetTransfer(ctx, q, in.ID)
		if err != nil {
			return p, err
		}
		if err = add(transfer); err != nil {
			return p, err
		}
		ids := []string{transfer.Source.TransactionID, transfer.Destination.TransactionID}
		if transfer.Fee != nil {
			ids = append(ids, transfer.Fee.TransactionID)
		}
		for _, id := range ids {
			if err = movement(id); err != nil {
				return p, err
			}
		}
		p.Effect = "Desvincula a transferência. Os lançamentos e seus efeitos nos saldos serão mantidos e voltarão a seguir suas decisões de categoria e inclusão nos totais."
	case "create_payable", "update_payable", "delete_payable":
		if in.Operation != "delete_payable" {
			var req payableCreateRequest
			if in.Operation == "create_payable" {
				if err := decodeMCPWrite(in.Data, &req); err != nil {
					return p, err
				}
				if req.Kind != "debt" && req.Kind != "receivable" {
					return p, errMCPWriteInput
				}
			} else {
				var update payableUpdateRequest
				if err := decodeMCPWrite(in.Data, &update); err != nil {
					return p, err
				}
				req.Name, req.TotalAmount = update.Name, update.TotalAmount
			}
			if !planningWriteAmount(in.Data, "total_amount") || !req.TotalAmount.IsPositive() || strings.TrimSpace(req.Name) == "" || len(req.Name) > 200 {
				return p, errMCPWriteInput
			}
			if req.InitialRemainingAmount != nil && (!planningWriteAmount(in.Data, "initial_remaining_amount") || req.InitialRemainingAmount.IsNegative() || req.InitialRemainingAmount.GreaterThan(req.TotalAmount)) {
				return p, errMCPWriteInput
			}
		}
		if !create {
			if _, err := snapshotPayable(in.ID); err != nil {
				return p, err
			}
		}
		p.Effect = "Cria ou altera uma dívida ou valor a receber em BRL, sem movimentar dinheiro. Valores quitados e restantes são recalculados a partir dos vínculos. Excluir remove a pendência, seus planos e vínculos, preservando os lançamentos reais."
	case "create_payable_link", "delete_payable_link":
		var payableID, transactionID string
		if create {
			var req payableLinkWriteRequest
			if err := decodeMCPWrite(in.Data, &req); err != nil {
				return p, err
			}
			payableID, transactionID = req.PayableID, req.TransactionID
		} else {
			var req payableUnlinkWriteRequest
			if err := decodeMCPWrite(in.Data, &req); err != nil {
				return p, err
			}
			payableID = req.PayableID
			link, err := payables.GetLink(ctx, q, in.ID)
			if err != nil {
				return p, err
			}
			if link.PayableID != payableID {
				return p, payables.ErrLinkNotFound
			}
			transactionID = link.TransactionID
			if err = add(link); err != nil {
				return p, err
			}
		}
		item, err := snapshotPayable(payableID)
		if err != nil {
			return p, err
		}
		if err = movement(transactionID); err != nil {
			return p, err
		}
		if create {
			result, _, err := payables.ValidateLink(ctx, q, item.Kind, payableID, transactionID)
			if err != nil {
				return p, err
			}
			if result.Status != payables.StatusCreated {
				return p, errMCPWriteInput
			}
		}
		p.Effect = "Vincula ou desvincula um lançamento real elegível em BRL: saída para dívida, entrada para valor a receber. O saldo quitado e o saldo restante serão recalculados conforme os vínculos. Desvincular também remove quitações e alocações dependentes; o lançamento real e o saldo bancário são preservados."
	default:
		return p, errMCPWriteInput
	}
	if !create {
		p.AffectedIDs = append(p.AffectedIDs, in.ID)
	}
	sort.Strings(p.AffectedIDs)
	p.AffectedIDs = compactWriteIDs(p.AffectedIDs)
	if len(p.AffectedIDs) > 100 {
		return p, errors.New("preview exceeds 100 affected records; use Julius")
	}
	p.AffectedCount = len(p.AffectedIDs)
	if create {
		p.AffectedCount++
	}
	return p, nil
}

func compactWriteIDs(ids []string) []string {
	out := ids[:0]
	for _, id := range ids {
		if len(out) == 0 || out[len(out)-1] != id {
			out = append(out, id)
		}
	}
	return out
}

func executeMCPAccountsWrite(ctx context.Context, conn *sql.DB, approved MCPWritePreview) (MCPWriteResult, error) {
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
	var value any
	switch in.Operation {
	case "create_manual_account", "update_manual_account", "adjust_manual_account":
		id := in.ID
		if in.Operation == "adjust_manual_account" {
			var req manualAccountAdjustmentRequest
			_ = json.Unmarshal(in.Data, &req)
			err = transactions.AdjustManualAccount(ctx, tx, id, decimal.RequireFromString(req.Amount), strings.TrimSpace(req.Description))
		} else {
			input, _ := mcpManualAccountInput(in.Data)
			id, err = transactions.WriteManualAccount(ctx, tx, id, input, in.Operation == "create_manual_account")
			if in.Operation == "create_manual_account" {
				result.Status = http.StatusCreated
			}
		}
		if err == nil {
			value, err = mcpAccount(ctx, tx, id)
		}
	case "create_transfer":
		var req transferWriteRequest
		_ = json.Unmarshal(in.Data, &req)
		if req.Source != nil {
			source, _ := req.Source.toManualInput()
			destination, _ := req.Destination.toManualInput()
			var fee *transactions.ManualInput
			if req.Fee != nil {
				f, _ := req.Fee.toManualInput()
				fee = &f
			}
			value, err = ledger.CreateManualTransferWithQuerier(ctx, tx, source, destination, fee, ledger.OnIgnored)
		} else {
			var id string
			id, err = transactions.CreateTransfer(ctx, tx, transactions.TransferInput{SourceID: req.SourceID, DestinationID: req.DestinationID, FeeID: req.FeeID})
			if err == nil {
				value, err = transactions.GetTransfer(ctx, tx, id)
			}
		}
		result.Status = http.StatusCreated
	case "delete_transfer":
		err = transactions.DeleteTransfer(ctx, tx, in.ID)
		result.Status = http.StatusNoContent
	case "create_payable", "update_payable":
		var item payables.Payable
		if in.Operation == "create_payable" {
			var req payableCreateRequest
			_ = json.Unmarshal(in.Data, &req)
			initial := req.TotalAmount
			if req.InitialRemainingAmount != nil {
				initial = *req.InitialRemainingAmount
			}
			item, err = payables.Create(ctx, tx, payables.Kind(req.Kind), req.Name, req.TotalAmount, initial)
			result.Status = http.StatusCreated
		} else {
			var req payableUpdateRequest
			_ = json.Unmarshal(in.Data, &req)
			item, err = payables.Update(ctx, tx, in.ID, req.Name, req.TotalAmount)
		}
		if err == nil {
			var summary payables.Summary
			summary, err = payables.Summarize(ctx, tx, item)
			if err == nil {
				value = payableToDTO(item, summary)
			}
		}
	case "delete_payable":
		err = payables.Delete(ctx, tx, in.ID)
		result.Status = http.StatusNoContent
	case "create_payable_link":
		var req payableLinkWriteRequest
		_ = json.Unmarshal(in.Data, &req)
		var item payables.Payable
		item, err = payables.Get(ctx, tx, req.PayableID)
		if err == nil {
			var link payables.CreateLinkResult
			link, err = payables.CreateLinkWithQuerier(ctx, tx, item.Kind, req.PayableID, req.TransactionID)
			if err == nil && link.Status != payables.StatusCreated {
				err = errMCPWriteInput
			}
			if err == nil {
				value = payableLinkDTO{ID: link.Link.ID, TransactionID: link.Link.TransactionID, LinkedAmount: link.Link.LinkedAmount.String(), LinkedAt: link.Link.LinkedAt}
			}
		}
		result.Status = http.StatusCreated
	case "delete_payable_link":
		var req payableUnlinkWriteRequest
		_ = json.Unmarshal(in.Data, &req)
		var found bool
		found, err = payables.DeleteLinkWithQuerier(ctx, tx, req.PayableID, in.ID)
		if err == nil && !found {
			err = payables.ErrLinkNotFound
		}
		result.Status = http.StatusNoContent
	default:
		return result, errMCPWriteInput
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
