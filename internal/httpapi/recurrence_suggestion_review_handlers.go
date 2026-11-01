package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/greg0x46/julius/internal/httplog"
	"github.com/greg0x46/julius/internal/recurrences/suggestions"
)

type suggestionReviewRequest struct {
	Key         string                          `json:"key"`
	Draft       recurringCommitmentWriteRequest `json:"draft"`
	Conditions  []conditionDTO                  `json:"conditions"`
	Token       string                          `json:"token"`
	Acknowledge bool                            `json:"acknowledge"`
}

func suggestionError(err error) (int, string) {
	switch {
	case errors.Is(err, suggestions.ErrDuplicate):
		return 409, "Já existe uma recorrência equivalente. Ajuste os dados propostos ou use a recorrência existente."
	case errors.Is(err, suggestions.ErrStale):
		return 409, "Os dados desta sugestão mudaram. Atualize a descoberta e revise novamente."
	case errors.Is(err, suggestions.ErrInvalid):
		return 422, "Selecione uma categoria ativa e confira os valores, as datas e as condições."
	case errors.Is(err, suggestions.ErrReview):
		return 422, "Revise as correspondências ambíguas. Condições que atravessam meses não são suportadas."
	default:
		return 503, "Não foi possível salvar a sugestão. Tente novamente."
	}
}

func writeSuggestionError(w http.ResponseWriter, err error) {
	status, detail := suggestionError(err)
	writeProblem(w, status, "recurrence-suggestion", "Não foi possível concluir a sugestão", detail)
}

func handleReviewRecurrenceSuggestion(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req suggestionReviewRequest
		if decodeStrict(r, &req) != nil {
			invalidRecurringCommitmentProblem(w, "Confira os dados da sugestão.")
			return
		}
		write, err := req.Draft.toWrite()
		if err != nil {
			writeSuggestionError(w, suggestions.ErrInvalid)
			return
		}
		conditions, ok := conditionsFromDTOs(req.Conditions, allowedAutomationCondition(true))
		if !ok {
			writeSuggestionError(w, suggestions.ErrInvalid)
			return
		}
		// Preserve omission to request generated conditions.
		if req.Conditions == nil {
			conditions = nil
		}
		tx, err := conn.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
		if err != nil {
			recurringCommitmentUnavailableProblem(w)
			return
		}
		defer tx.Rollback()
		review, err := suggestions.ReviewCandidate(r.Context(), tx, time.Now().UTC(), req.Key, write, conditions)
		if err != nil {
			writeSuggestionError(w, err)
			return
		}
		type historyDTO struct {
			ID               string         `json:"id"`
			Date             string         `json:"date"`
			Description      string         `json:"description"`
			Amount           string         `json:"amount"`
			CurrencyCode     string         `json:"currency_code"`
			Evidence         bool           `json:"evidence"`
			Status           string         `json:"status"`
			Reasons          []string       `json:"reasons"`
			FailedConditions []conditionDTO `json:"failed_conditions"`
			RuleIDs          []string       `json:"rule_ids"`
			ScenarioIDs      []string       `json:"scenario_ids"`
		}
		evidence := map[string]bool{}
		for _, o := range review.Draft.Evidence {
			evidence[o.Transaction.ID] = true
		}
		history := make([]historyDTO, 0, len(review.History))
		for i, item := range review.History {
			row := historyDTO{ID: item.ID, Date: item.OccurredAt.Format(dateOnlyLayout), Evidence: evidence[item.ID], Status: review.Preview.Transactions[i].Status, Reasons: review.Preview.Transactions[i].Reasons, FailedConditions: toConditionDTOs(review.Preview.Transactions[i].FailedConditions), RuleIDs: review.Preview.Transactions[i].RuleIDs, ScenarioIDs: review.Preview.Transactions[i].ScenarioIDs}
			if item.Description != nil {
				row.Description = *item.Description
			}
			if item.EffectiveMoney != nil {
				row.Amount = item.EffectiveMoney.Value
				row.CurrencyCode = item.EffectiveMoney.CurrencyCode
			} else {
				row.Amount = "0"
			}
			history = append(history, row)
		}
		writeJSON(w, http.StatusOK, struct {
			Token       string         `json:"token"`
			Conditions  []conditionDTO `json:"conditions"`
			Warnings    []string       `json:"warnings"`
			NeedsReview bool           `json:"needs_review"`
			History     []historyDTO   `json:"history"`
		}{review.Token, toConditionDTOs(review.Draft.Rule.Conditions), review.Preview.Warnings, review.Preview.NeedsReview, history})
	}
}

func handleAcceptRecurrenceSuggestions(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Items []suggestionReviewRequest `json:"items"`
		}
		if decodeStrict(r, &req) != nil || len(req.Items) == 0 || len(req.Items) > 100 {
			invalidRecurringCommitmentProblem(w, "Selecione entre 1 e 100 sugestões.")
			return
		}
		type result struct {
			Key        string `json:"key"`
			ScenarioID string `json:"scenario_id"`
			Status     int    `json:"status"`
			Error      string `json:"error"`
		}
		results := make([]result, 0, len(req.Items))
		for _, item := range req.Items {
			row := result{Key: item.Key, Status: 201}
			write, err := item.Draft.toWrite()
			conditions, ok := conditionsFromDTOs(item.Conditions, allowedAutomationCondition(true))
			if err != nil || !ok || len(conditions) == 0 {
				err = suggestions.ErrInvalid
			} else {
				row.ScenarioID, err = suggestions.Accept(r.Context(), conn, time.Now().UTC(), item.Key, write, conditions, item.Token, item.Acknowledge)
			}
			if err != nil {
				httplog.SetError(r.Context(), "recurrence-suggestion")
				row.Status, row.Error = suggestionError(err)
			}
			results = append(results, row)
		}
		writeJSON(w, http.StatusOK, results)
	}
}

func handleDismissRecurrenceSuggestion(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Key string `json:"key"`
		}
		if decodeStrict(r, &req) != nil || req.Key == "" {
			invalidRecurringCommitmentProblem(w, "Selecione uma sugestão.")
			return
		}
		if err := suggestions.Dismiss(r.Context(), conn, time.Now().UTC(), req.Key); err != nil {
			writeSuggestionError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
