package httpapi

import (
	"database/sql"
	"net/http"

	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/quotes"
)

// investmentMovementRequest records a movement the provider did not report
// for one of its holdings. Amount is the money in reais, costs included; it
// defaults to quantity × unit price.
type investmentMovementRequest struct {
	InvestmentID string            `json:"investment_id"`
	Kind         string            `json:"kind"`
	TradeDate    *string           `json:"trade_date"`
	Quantity     investmentDecimal `json:"quantity"`
	UnitPrice    investmentDecimal `json:"unit_price"`
	Amount       investmentDecimal `json:"amount"`
	Notes        *string           `json:"notes"`
}

func (req investmentMovementRequest) toInput() (investments.ManualMovementInput, bool) {
	day, ok := investmentBodyDate(req.TradeDate)
	if !ok {
		return investments.ManualMovementInput{}, false
	}
	amount := req.Amount.orZero()
	if !req.Amount.present && req.Quantity.present && req.UnitPrice.present {
		amount = req.Quantity.value.Mul(req.UnitPrice.value)
	}
	return investments.ManualMovementInput{
		InvestmentID: req.InvestmentID,
		Kind:         investments.ManualMovementKind(req.Kind),
		TradeDate:    day,
		Quantity:     req.Quantity.pointer(),
		UnitPrice:    req.UnitPrice.pointer(),
		Amount:       amount,
		Notes:        req.Notes,
	}, true
}

func manualMovementToTransactionDTO(m investments.ManualMovement) investmentTransactionDTO {
	kind, direction := string(m.Kind), m.Direction
	day := m.TradeDate
	dto := investmentTransactionDTO{
		ID: m.ID, ExternalID: m.ID, Origin: "manual", MovementType: &kind, Direction: &direction,
		TradeDate: &day, Notes: m.Notes,
	}
	amount := money.CanonicalDecimal(m.Amount)
	dto.Amount = &amount
	if m.Quantity != nil {
		quantity := money.CanonicalDecimal(*m.Quantity)
		dto.Quantity = &quantity
	}
	if m.UnitPrice != nil {
		price := money.CanonicalDecimal(*m.UnitPrice)
		dto.Value = &price
	}
	return dto
}

func handleCreateInvestmentMovement(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req investmentMovementRequest
		if err := decodeStrict(r, &req); err != nil {
			writeInvestmentInvalid(w, "Não foi possível ler os dados enviados.")
			return
		}
		input, ok := req.toInput()
		if !ok {
			writeInvestmentInvalid(w, "Informe a data da movimentação no formato AAAA-MM-DD.")
			return
		}
		movement, err := investments.CreateManualMovement(r.Context(), conn, input)
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		quotes.RequestBackfill()
		writeJSON(w, http.StatusCreated, manualMovementToTransactionDTO(movement))
	}
}

func handleUpdateInvestmentMovement(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req investmentMovementRequest
		if err := decodeStrict(r, &req); err != nil {
			writeInvestmentInvalid(w, "Não foi possível ler os dados enviados.")
			return
		}
		input, ok := req.toInput()
		if !ok {
			writeInvestmentInvalid(w, "Informe a data da movimentação no formato AAAA-MM-DD.")
			return
		}
		movement, err := investments.UpdateManualMovement(r.Context(), conn, r.PathValue("id"), input)
		if err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		quotes.RequestBackfill()
		writeJSON(w, http.StatusOK, manualMovementToTransactionDTO(movement))
	}
}

func handleDeleteInvestmentMovement(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := investments.DeleteManualMovement(r.Context(), conn, r.PathValue("id")); err != nil {
			writeInvestmentProblem(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
