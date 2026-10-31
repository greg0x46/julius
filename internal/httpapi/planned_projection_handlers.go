package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/projections"
	"github.com/greg0x46/julius/internal/scenarios"
)

type plannedTransactionDTO struct {
	ScenarioID        string  `json:"scenario_id"`
	EventKey          string  `json:"event_key"`
	ScenarioKind      string  `json:"scenario_kind"`
	Date              string  `json:"date"`
	Description       string  `json:"description"`
	Amount            string  `json:"amount"`
	CategoryID        *string `json:"category_id"`
	CategoryName      string  `json:"category_name"`
	Tier              string  `json:"tier"`
	Source            string  `json:"source"`
	PayableID         *string `json:"payable_id"`
	Realized          bool    `json:"realized"`
	RealizationOrigin string  `json:"realization_origin"`
	Detached          bool    `json:"detached"`
}

func plannedTransactionToDTO(event projections.PlannedTransaction) plannedTransactionDTO {
	return plannedTransactionDTO{
		ScenarioID: event.ScenarioID, EventKey: event.EventKey, ScenarioKind: string(event.ScenarioKind),
		Date: event.Date.Format(dateOnlyLayout), Description: event.Description,
		Amount: money.CanonicalDecimal(event.Amount), CategoryID: event.CategoryID,
		CategoryName: event.CategoryName, Tier: string(event.Tier), Source: string(event.Source),
		PayableID: event.PayableID, Realized: event.Realized,
		RealizationOrigin: event.RealizationOrigin, Detached: event.Detached,
	}
}

func plannedWindow(r *http.Request) (time.Time, time.Time, error) {
	today := dates.Day(time.Now().UTC())
	from, to := today, dates.AddMonths(today, 12)
	var err error
	if raw := r.URL.Query().Get("from"); raw != "" {
		from, err = time.Parse(dateOnlyLayout, raw)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		to, err = time.Parse(dateOnlyLayout, raw)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
	}
	if dates.ExceedsMaxSpan(from, to) {
		return time.Time{}, time.Time{}, errRangeTooLong
	}
	if to.Before(from) {
		return time.Time{}, time.Time{}, errors.New("to não pode ser anterior a from")
	}
	return from, to, nil
}

func handleListScenarioPlannedTransactions(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		from, to, err := plannedWindow(r)
		if errors.Is(err, errRangeTooLong) {
			invalidScenarioProblem(w, rangeTooLongDetail)
			return
		}
		if err != nil {
			invalidScenarioProblem(w, "Período inválido.")
			return
		}
		id := r.PathValue("id")
		if _, err := scenarios.GetScenario(r.Context(), conn, id); errors.Is(err, scenarios.ErrScenarioNotFound) {
			scenarioNotFoundProblem(w)
			return
		} else if err != nil {
			scenariosUnavailableProblem(w)
			return
		}
		events, err := projections.List(r.Context(), conn, projections.ProjectionQuery{
			From: from, To: to, Selection: projections.SelectionExplicit, IDs: []string{id},
		})
		if err != nil {
			scenariosUnavailableProblem(w)
			return
		}
		dtos := make([]plannedTransactionDTO, 0, len(events))
		for _, event := range events {
			if event.ScenarioID == id {
				dtos = append(dtos, plannedTransactionToDTO(event))
			}
		}
		writeJSON(w, http.StatusOK, dtos)
	}
}

type scenarioActivationRequest struct {
	IsActive *bool `json:"is_active"`
}

func handlePatchScenario(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request scenarioActivationRequest
		if err := decodeStrict(r, &request); err != nil || request.IsActive == nil {
			invalidScenarioProblem(w, "Informe is_active como booleano.")
			return
		}
		scenario, err := scenarios.SetActive(r.Context(), conn, r.PathValue("id"), *request.IsActive)
		if errors.Is(err, scenarios.ErrScenarioNotFound) {
			scenarioNotFoundProblem(w)
			return
		}
		if err != nil {
			scenariosUnavailableProblem(w)
			return
		}
		writeJSON(w, http.StatusOK, scenarioToDTO(scenario))
	}
}

type plannedRealizationRequest struct {
	State           string           `json:"state"`
	TransactionID   *string          `json:"transaction_id"`
	AllocatedAmount *decimal.Decimal `json:"allocated_amount"`
}

type genericRealizationDTO struct {
	ID                    string    `json:"id"`
	ScenarioID            string    `json:"scenario_id"`
	ScenarioTransactionID *string   `json:"scenario_transaction_id"`
	OccurrenceDate        *string   `json:"occurrence_date"`
	TransactionID         *string   `json:"transaction_id"`
	RelationType          string    `json:"relation_type"`
	State                 string    `json:"state"`
	Origin                string    `json:"origin"`
	AllocatedAmount       *string   `json:"allocated_amount"`
	LinkedAmount          *string   `json:"linked_amount"`
	CreatedAt             time.Time `json:"created_at"`
}

func genericRealizationToDTO(value scenarios.GenericRealization) genericRealizationDTO {
	var date *string
	if value.OccurrenceDate != nil {
		formatted := value.OccurrenceDate.Format(dateOnlyLayout)
		date = &formatted
	}
	return genericRealizationDTO{
		ID: value.ID, ScenarioID: value.ScenarioID, ScenarioTransactionID: value.ScenarioTransactionID,
		OccurrenceDate: date, TransactionID: value.TransactionID, RelationType: string(value.RelationType),
		State: value.State, Origin: value.Origin, AllocatedAmount: decimalString(value.AllocatedAmount),
		LinkedAmount: decimalString(value.LinkedAmount), CreatedAt: value.CreatedAt,
	}
}

func decimalString(value *decimal.Decimal) *string {
	if value == nil {
		return nil
	}
	formatted := money.CanonicalDecimal(*value)
	return &formatted
}

func handlePutScenarioPlannedRealization(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request plannedRealizationRequest
		if err := decodeStrict(r, &request); err != nil {
			invalidScenarioProblem(w, "Revise os campos da realização.")
			return
		}
		state := strings.TrimSpace(request.State)
		if state == "" {
			if request.TransactionID != nil {
				state = scenarios.RealizationStateLinked
			} else {
				state = scenarios.RealizationStateDetached
			}
		}
		amount := decimal.Zero
		if request.AllocatedAmount != nil {
			amount = *request.AllocatedAmount
		}
		if !validMoneyAmount(amount) {
			invalidScenarioProblem(w, "Valor alocado inválido. "+invalidAmountDetail)
			return
		}
		value, err := scenarios.RealizeEvent(r.Context(), conn, r.PathValue("id"), r.PathValue("eventKey"), scenarios.RealizationWrite{
			State: state, TransactionID: request.TransactionID, AllocatedAmount: amount, Origin: "manual",
		})
		switch {
		case errors.Is(err, scenarios.ErrScenarioNotFound):
			scenarioNotFoundProblem(w)
		case errors.Is(err, scenarios.ErrTransactionUnavailable):
			writeProblem(w, 404, "transaction-not-found", "Transação não encontrada", "")
		case errors.Is(err, scenarios.ErrTransactionAlreadyRealized):
			writeProblem(w, http.StatusConflict, "transaction-already-realized", "Transação já utilizada", "Uma transação real não pode satisfazer dois eventos completos.")
		case errors.Is(err, scenarios.ErrInvalidEventKey), errors.Is(err, scenarios.ErrInvalidRealization):
			invalidScenarioProblem(w, "O evento previsto ou a realização é inválido.")
		case err != nil:
			if db.IsUniqueViolationOn(err, db.ConstraintOneCompleteEventTransactionSQLite, db.ConstraintOneCompleteEventTransactionPostgres) {
				writeProblem(w, http.StatusConflict, "transaction-already-realized", "Transação já utilizada", "Uma transação real não pode satisfazer dois eventos completos.")
			} else {
				scenariosUnavailableProblem(w)
			}
		default:
			writeJSON(w, http.StatusOK, genericRealizationToDTO(value))
		}
	}
}

func handleDeleteScenarioPlannedRealization(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := scenarios.DeletePlannedEventRealization(r.Context(), conn, r.PathValue("id"), r.PathValue("eventKey"))
		if errors.Is(err, scenarios.ErrScenarioNotFound) {
			scenarioNotFoundProblem(w)
			return
		}
		if errors.Is(err, scenarios.ErrPlannedRealizationNotFound) {
			realizationNotFoundProblem(w)
			return
		}
		if err != nil {
			scenariosUnavailableProblem(w)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleListScenarioRealizations keeps the old scenario-level endpoint alive
// while returning the same generic relation model used by the new event
// endpoints.
func handleListScenarioRealizations(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if _, err := scenarios.GetScenario(r.Context(), conn, id); errors.Is(err, scenarios.ErrScenarioNotFound) {
			scenarioNotFoundProblem(w)
			return
		} else if err != nil {
			scenariosUnavailableProblem(w)
			return
		}
		list, err := scenarios.ListRealizations(r.Context(), conn, id)
		if err != nil {
			scenariosUnavailableProblem(w)
			return
		}
		dtos := make([]genericRealizationDTO, len(list))
		for i, value := range list {
			dtos[i] = genericRealizationToDTO(value)
		}
		writeJSON(w, http.StatusOK, dtos)
	}
}
