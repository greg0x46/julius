package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/greg0x46/julius/internal/settings"
)

type preferencesDTO struct {
	TransactionsPeriodBasis string `json:"transactions_period_basis"`
	ReferenceCurrency       string `json:"reference_currency"`
}

func handleGetPreferences(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		basis, err := settings.GetTransactionsPeriodBasis(r.Context(), db)
		if err != nil {
			writeProblem(w, 503, "preferences-unavailable", "Preferências temporariamente indisponíveis", "Tente novamente em instantes.")
			return
		}
		code, err := settings.ReferenceCurrency(r.Context(), db)
		if err != nil {
			writeProblem(w, 503, "preferences-unavailable", "Preferências temporariamente indisponíveis", "")
			return
		}
		writeJSON(w, http.StatusOK, preferencesDTO{TransactionsPeriodBasis: basis, ReferenceCurrency: code})
	}
}

func handleUpdatePreferences(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			TransactionsPeriodBasis string          `json:"transactions_period_basis"`
			ReferenceCurrency       json.RawMessage `json:"reference_currency"`
		}
		if err := decodeStrict(r, &input); err != nil {
			writeProblem(w, 422, "invalid-preferences", "Preferências inválidas", "Revise os campos enviados.")
			return
		}
		req := preferencesDTO{TransactionsPeriodBasis: input.TransactionsPeriodBasis}
		if len(input.ReferenceCurrency) > 0 {
			if err := json.Unmarshal(input.ReferenceCurrency, &req.ReferenceCurrency); err != nil {
				writeProblem(w, 422, "invalid-preferences", "Preferências inválidas", "Escolha uma moeda de referência válida.")
				return
			}
		}
		if req.TransactionsPeriodBasis != settings.PeriodBasisOccurredAt && req.TransactionsPeriodBasis != settings.PeriodBasisPaidAt {
			writeProblem(w, 422, "invalid-preferences", "Preferências inválidas",
				"transactions_period_basis deve ser \"occurred_at\" ou \"paid_at\".")
			return
		}
		// Older clients omit the currency; preserve the saved preference.
		if len(input.ReferenceCurrency) == 0 {
			code, err := settings.ReferenceCurrency(r.Context(), db)
			if err != nil {
				writeProblem(w, 503, "preferences-unavailable", "Preferências temporariamente indisponíveis", "")
				return
			}
			req.ReferenceCurrency = code
		}
		if err := settings.ValidateReferenceCurrency(req.ReferenceCurrency); err != nil {
			writeProblem(w, 422, "invalid-preferences", "Preferências inválidas", err.Error())
			return
		}
		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			writeProblem(w, 503, "preferences-unavailable", "Preferências temporariamente indisponíveis", "")
			return
		}
		defer tx.Rollback()
		err = settings.Set(r.Context(), tx, settings.KeyTransactionsPeriodBasis, req.TransactionsPeriodBasis, false, nil)
		if err == nil {
			err = settings.Set(r.Context(), tx, settings.KeyReferenceCurrency, req.ReferenceCurrency, false, nil)
		}
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			writeProblem(w, 503, "preferences-unavailable", "Preferências temporariamente indisponíveis", "Tente novamente em instantes.")
			return
		}
		writeJSON(w, http.StatusOK, req)
	}
}
