package httpapi

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/recurrences/suggestions"
)

type recurrenceSuggestionDTO struct {
	Key            string                          `json:"key"`
	Draft          recurringCommitmentWriteRequest `json:"draft"`
	CurrencyCode   string                          `json:"currency_code"`
	AmountKind     string                          `json:"amount_kind"`
	MinimumAmount  string                          `json:"minimum_amount"`
	MaximumAmount  string                          `json:"maximum_amount"`
	Confidence     string                          `json:"confidence"`
	Reasons        []string                        `json:"reasons"`
	TransactionIDs []string                        `json:"transaction_ids"`
	CardNumber     string                          `json:"card_number"`
}

func handleRecurrenceSuggestions(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		candidates, err := suggestions.List(r.Context(), conn, time.Now().UTC())
		if err != nil {
			recurringCommitmentUnavailableProblem(w)
			return
		}
		dtos := make([]recurrenceSuggestionDTO, len(candidates))
		for i, s := range candidates {
			c := s.Draft
			dtos[i] = recurrenceSuggestionDTO{
				Key: suggestions.Key(s),
				Draft: recurringCommitmentWriteRequest{
					Name: c.Name, Kind: string(c.Kind), Amount: money.CanonicalDecimal(c.Amount), CategoryID: c.CategoryID,
					AccountID: c.AccountID, Cadence: string(c.Cadence), DayOfMonth: c.DayOfMonth, MonthOfYear: c.MonthOfYear,
					StartDate: c.StartDate.Format(dateOnlyLayout), IsActive: c.IsActive,
				},
				CurrencyCode: "BRL", AmountKind: s.AmountKind, MinimumAmount: money.CanonicalDecimal(s.MinimumAmount), MaximumAmount: money.CanonicalDecimal(s.MaximumAmount),
				Confidence: s.Confidence, Reasons: s.Reasons, TransactionIDs: s.TransactionIDs, CardNumber: s.CardNumber,
			}
		}
		writeJSON(w, http.StatusOK, dtos)
	}
}
