package httpapi

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/greg0x46/julius/internal/ledger"
	"github.com/greg0x46/julius/internal/transactions"
)

func transferProblem(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, transactions.ErrTransferLinked), errors.Is(err, transactions.ErrInvestmentLinked):
		writeProblem(w, 409, "transfer-conflict", "Movimentação já vinculada", "Desfaça o vínculo anterior antes de vincular esta transferência.")
	case errors.Is(err, transactions.ErrInvalidTransfer):
		writeProblem(w, 422, "invalid-transfer", "Transferência inválida", "Selecione uma saída e uma entrada em contas próprias distintas, nas moedas das contas. Na mesma moeda, os valores devem ser iguais. A tarifa deve ser uma despesa separada em uma dessas contas. "+invalidAmountDetail+" "+invalidDateDetail)
	case errors.Is(err, transactions.ErrTransferNotFound), errors.Is(err, transactions.ErrTransactionNotFound), errors.Is(err, transactions.ErrAccountNotFound):
		writeProblem(w, 404, "transfer-not-found", "Movimentação não encontrada", "")
	case errors.Is(err, transactions.ErrAccountArchived):
		writeProblem(w, 409, "account-archived", "Conta arquivada", "Reative a conta antes de registrar movimentações.")
	default:
		writeProblem(w, 503, "transfer-unavailable", "Transferência temporariamente indisponível", "Tente novamente em instantes.")
	}
}

func handleCreateTransfer(svc *ledger.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SourceID      string                    `json:"source_transaction_id"`
			DestinationID string                    `json:"destination_transaction_id"`
			FeeID         string                    `json:"fee_transaction_id"`
			Source        *manualTransactionRequest `json:"source"`
			Destination   *manualTransactionRequest `json:"destination"`
			Fee           *manualTransactionRequest `json:"fee"`
		}
		if err := decodeStrict(r, &req); err != nil {
			transferProblem(w, transactions.ErrInvalidTransfer)
			return
		}
		var result *transactions.AccountTransfer
		var err error
		if req.Source != nil || req.Destination != nil || req.Fee != nil {
			if req.Source == nil || req.Destination == nil || req.SourceID != "" || req.DestinationID != "" || req.FeeID != "" || req.Source.CategoryID != nil || req.Destination.CategoryID != nil {
				transferProblem(w, transactions.ErrInvalidTransfer)
				return
			}
			source, ok1 := req.Source.toManualInput()
			destination, ok2 := req.Destination.toManualInput()
			if !ok1 || !ok2 {
				transferProblem(w, transactions.ErrInvalidTransfer)
				return
			}
			var fee *transactions.ManualInput
			if req.Fee != nil {
				input, ok := req.Fee.toManualInput()
				if !ok || req.Fee.CategoryID != nil {
					transferProblem(w, transactions.ErrInvalidTransfer)
					return
				}
				fee = &input
			}
			result, err = svc.CreateManualTransfer(r.Context(), source, destination, fee)
		} else {
			result, err = svc.LinkTransfer(r.Context(), transactions.TransferInput{SourceID: req.SourceID, DestinationID: req.DestinationID, FeeID: req.FeeID})
		}
		if err != nil {
			transferProblem(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, result)
	}
}

func handleGetTransactionTransfer(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := transactions.GetTransferForTransaction(r.Context(), conn, r.PathValue("id"))
		if err != nil {
			transferProblem(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func handleDeleteTransfer(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := transactions.DeleteTransfer(r.Context(), conn, r.PathValue("id")); err != nil {
			transferProblem(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
