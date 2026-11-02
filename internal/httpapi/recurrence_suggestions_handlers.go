package httpapi

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/recurrences/suggestions"
	"github.com/greg0x46/julius/internal/transactions"
)

type suggestionEvidenceDTO struct {
	ID          string `json:"id"`
	Date        string `json:"date"`
	Description string `json:"description"`
	Amount      string `json:"amount"`
}

type recurrenceSuggestionDTO struct {
	Evidence       []suggestionEvidenceDTO         `json:"evidence"`
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
		history, err := transactions.History(r.Context(), conn)
		if err != nil {
			recurringCommitmentUnavailableProblem(w)
			return
		}
		byID := map[string]transactions.Item{}
		for _, item := range history {
			byID[item.ID] = item
		}
		dtos := make([]recurrenceSuggestionDTO, len(candidates))
		for i, s := range candidates {
			evidence := make([]suggestionEvidenceDTO, 0, len(s.TransactionIDs))
			for _, id := range s.TransactionIDs {
				item, ok := byID[id]
				if !ok || item.OccurredAt == nil || item.EffectiveMoney == nil {
					continue
				}
				description := ""
				if item.Description != nil {
					description = *item.Description
				}
				evidence = append(evidence, suggestionEvidenceDTO{ID: id, Date: item.OccurredAt.Format(dateOnlyLayout), Description: description, Amount: item.EffectiveMoney.Value})
			}
			c := s.Draft
			dtos[i] = recurrenceSuggestionDTO{
				Key: suggestions.Key(s), Evidence: evidence,
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
