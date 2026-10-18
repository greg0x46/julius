package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/greg0x46/julius/internal/goals"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/valuation"
)

// Goals are always expressed in BRL, the base currency of the figures they
// track; they are not re-expressed in the reference currency.
const goalCurrency = "BRL"

type goalDTO struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Metric       string    `json:"metric"`
	TargetAmount string    `json:"target_amount"`
	TargetDate   *string   `json:"target_date"`
	Notes        *string   `json:"notes"`
	CurrentValue string    `json:"current_value"`
	Progress     string    `json:"progress"`
	CurrencyCode string    `json:"currency_code"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func goalToDTO(g goals.Goal) goalDTO {
	return goalDTO{
		ID:           g.ID,
		Name:         g.Name,
		Metric:       string(g.Metric),
		TargetAmount: money.CanonicalDecimal(g.TargetAmount),
		TargetDate:   optionalDay(g.TargetDate),
		Notes:        g.Notes,
		CurrentValue: money.CanonicalDecimal(g.CurrentValue),
		Progress:     money.CanonicalDecimal(g.Progress),
		CurrencyCode: goalCurrency,
		CreatedAt:    g.CreatedAt,
		UpdatedAt:    g.UpdatedAt,
	}
}

type goalRequest struct {
	Name         string            `json:"name"`
	Metric       string            `json:"metric"`
	TargetAmount investmentDecimal `json:"target_amount"`
	TargetDate   *string           `json:"target_date"`
	Notes        *string           `json:"notes"`
}

func (req goalRequest) toInput() (goals.Input, bool) {
	target := req.TargetAmount.pointer()
	if target == nil {
		return goals.Input{}, false
	}
	input := goals.Input{
		Name:         req.Name,
		Metric:       goals.Metric(req.Metric),
		TargetAmount: *target,
		Notes:        req.Notes,
	}
	if req.TargetDate != nil && strings.TrimSpace(*req.TargetDate) != "" {
		day, ok := investmentBodyDate(req.TargetDate)
		if !ok {
			return goals.Input{}, false
		}
		input.TargetDate = &day
	}
	return input, true
}

func writeGoalInvalid(w http.ResponseWriter) {
	writeProblem(w, http.StatusBadRequest, "invalid-goal-input", "Dados da meta inválidos",
		"Informe um nome, a métrica, um valor alvo maior que zero e, se houver, a data no formato AAAA-MM-DD.")
}

func writeGoalProblem(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, goals.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "goal-not-found", "Meta não encontrada", "A meta informada não existe.")
	case errors.Is(err, goals.ErrInvalidInput):
		writeGoalInvalid(w)
	case errors.Is(err, valuation.ErrMissingRate):
		writeProblem(w, http.StatusServiceUnavailable, "exchange-unavailable", "Cotação indisponível",
			"Falta a cotação de uma moeda para valorar os investimentos em reais. Nenhum saldo foi ignorado.")
	default:
		writeProblem(w, http.StatusServiceUnavailable, "goals-unavailable", "Metas temporariamente indisponíveis", "Tente novamente em instantes.")
	}
}

func decodeGoalRequest(w http.ResponseWriter, r *http.Request) (goals.Input, bool) {
	var req goalRequest
	if err := decodeStrict(r, &req); err != nil {
		writeGoalInvalid(w)
		return goals.Input{}, false
	}
	input, ok := req.toInput()
	if !ok {
		writeGoalInvalid(w)
	}
	return input, ok
}

func handleListGoals(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := goals.List(r.Context(), conn)
		if err != nil {
			writeGoalProblem(w, err)
			return
		}
		items := make([]goalDTO, 0, len(list))
		for _, g := range list {
			items = append(items, goalToDTO(g))
		}
		writeInvestmentItems(w, items)
	}
}

func handleCreateGoal(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		input, ok := decodeGoalRequest(w, r)
		if !ok {
			return
		}
		goal, err := goals.Create(r.Context(), conn, input)
		if err != nil {
			writeGoalProblem(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, goalToDTO(goal))
	}
}

func handleUpdateGoal(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		input, ok := decodeGoalRequest(w, r)
		if !ok {
			return
		}
		goal, err := goals.Update(r.Context(), conn, r.PathValue("id"), input)
		if err != nil {
			writeGoalProblem(w, err)
			return
		}
		writeJSON(w, http.StatusOK, goalToDTO(goal))
	}
}

func handleDeleteGoal(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := goals.Delete(r.Context(), conn, r.PathValue("id")); err != nil {
			writeGoalProblem(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
