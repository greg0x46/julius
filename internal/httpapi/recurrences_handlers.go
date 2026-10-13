package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/recurrences"
)

type recurringCommitmentDTO struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Kind        string    `json:"kind"`
	Amount      string    `json:"amount"`
	CategoryID  string    `json:"category_id"`
	AccountID   *string   `json:"account_id"`
	Cadence     string    `json:"cadence"`
	DayOfMonth  int       `json:"day_of_month"`
	MonthOfYear *int      `json:"month_of_year"`
	StartDate   string    `json:"start_date"`
	EndDate     *string   `json:"end_date"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func toRecurringCommitmentDTO(c recurrences.RecurringCommitment) recurringCommitmentDTO {
	var endDate *string
	if c.EndDate != nil {
		s := c.EndDate.Format(dateOnlyLayout)
		endDate = &s
	}
	return recurringCommitmentDTO{
		ID: c.ID, Name: c.Name, Kind: string(c.Kind), Amount: money.CanonicalDecimal(c.Amount),
		CategoryID: c.CategoryID, AccountID: c.AccountID, Cadence: string(c.Cadence),
		DayOfMonth: c.DayOfMonth, MonthOfYear: c.MonthOfYear,
		StartDate: c.StartDate.Format(dateOnlyLayout), EndDate: endDate,
		IsActive:  c.IsActive,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

type recurringCommitmentWriteRequest struct {
	Name        string  `json:"name"`
	Kind        string  `json:"kind"`
	Amount      string  `json:"amount"`
	CategoryID  string  `json:"category_id"`
	AccountID   *string `json:"account_id"`
	Cadence     string  `json:"cadence"`
	DayOfMonth  int     `json:"day_of_month"`
	MonthOfYear *int    `json:"month_of_year"`
	StartDate   string  `json:"start_date"`
	EndDate     *string `json:"end_date"`
	IsActive    bool    `json:"is_active"`
}

func (req recurringCommitmentWriteRequest) toWrite() (recurrences.Write, error) {
	amount, err := decimal.NewFromString(req.Amount)
	if err != nil {
		return recurrences.Write{}, errors.New("invalid amount")
	}
	startDate, err := time.Parse(dateOnlyLayout, req.StartDate)
	if err != nil {
		return recurrences.Write{}, errors.New("invalid start_date")
	}
	var endDate *time.Time
	if req.EndDate != nil && *req.EndDate != "" {
		d, err := time.Parse(dateOnlyLayout, *req.EndDate)
		if err != nil {
			return recurrences.Write{}, errors.New("invalid end_date")
		}
		endDate = &d
	}
	write := recurrences.Write{
		Name: req.Name, Kind: recurrences.Kind(req.Kind), Amount: amount,
		CategoryID: req.CategoryID, AccountID: req.AccountID, Cadence: recurrences.Cadence(req.Cadence),
		DayOfMonth: req.DayOfMonth, MonthOfYear: req.MonthOfYear, StartDate: startDate, EndDate: endDate,
		IsActive: req.IsActive,
	}
	commitment := recurrences.RecurringCommitment{
		Name: write.Name, Kind: write.Kind, Amount: write.Amount, CategoryID: write.CategoryID,
		Cadence: write.Cadence, DayOfMonth: write.DayOfMonth, MonthOfYear: write.MonthOfYear,
		StartDate: write.StartDate, EndDate: write.EndDate,
	}
	if err := commitment.Validate(); err != nil {
		return recurrences.Write{}, err
	}
	return write, nil
}

func invalidRecurringCommitmentProblem(w http.ResponseWriter, detail string) {
	writeProblem(w, 422, "invalid-recurring-commitment", "Compromisso recorrente inválido", detail)
}

func recurringCommitmentUnavailableProblem(w http.ResponseWriter) {
	writeProblem(w, 503, "recurring-commitment-unavailable", "Recorrências temporariamente indisponíveis", "Tente novamente em instantes.")
}

func recurringCommitmentLinkedProblem(w http.ResponseWriter) {
	writeProblem(w, 409, "recurring-commitment-linked", "Compromisso vinculado a uma automação",
		"Remova ou altere a ação de conciliação da automação vinculada antes de excluir este compromisso.")
}

func handleListRecurringCommitments(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		commitments, err := recurrences.List(r.Context(), conn)
		if err != nil {
			recurringCommitmentUnavailableProblem(w)
			return
		}
		dtos := make([]recurringCommitmentDTO, len(commitments))
		for i, c := range commitments {
			dtos[i] = toRecurringCommitmentDTO(c)
		}
		writeJSON(w, http.StatusOK, dtos)
	}
}

func handleCreateRecurringCommitment(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req recurringCommitmentWriteRequest
		if err := decodeStrict(r, &req); err != nil {
			invalidRecurringCommitmentProblem(w, "")
			return
		}
		write, err := req.toWrite()
		if err != nil {
			invalidRecurringCommitmentProblem(w, err.Error())
			return
		}
		commitment, err := recurrences.Create(r.Context(), conn, write)
		if err != nil {
			recurringCommitmentUnavailableProblem(w)
			return
		}
		writeJSON(w, http.StatusCreated, toRecurringCommitmentDTO(commitment))
	}
}

func handleUpdateRecurringCommitment(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req recurringCommitmentWriteRequest
		if err := decodeStrict(r, &req); err != nil {
			invalidRecurringCommitmentProblem(w, "")
			return
		}
		write, err := req.toWrite()
		if err != nil {
			invalidRecurringCommitmentProblem(w, err.Error())
			return
		}
		commitment, err := recurrences.Update(r.Context(), conn, id, write)
		if errors.Is(err, recurrences.ErrNotFound) {
			writeProblem(w, 404, "recurring-commitment-not-found", "Compromisso não encontrado", "")
			return
		}
		if err != nil {
			recurringCommitmentUnavailableProblem(w)
			return
		}
		writeJSON(w, http.StatusOK, toRecurringCommitmentDTO(commitment))
	}
}

func handleSetRecurringCommitmentActive(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req activationRequest
		if err := decodeStrict(r, &req); err != nil {
			invalidRecurringCommitmentProblem(w, "")
			return
		}
		commitment, err := recurrences.SetActive(r.Context(), conn, id, req.IsActive)
		if errors.Is(err, recurrences.ErrNotFound) {
			writeProblem(w, 404, "recurring-commitment-not-found", "Compromisso não encontrado", "")
			return
		}
		if err != nil {
			recurringCommitmentUnavailableProblem(w)
			return
		}
		writeJSON(w, http.StatusOK, toRecurringCommitmentDTO(commitment))
	}
}

func handleDeleteRecurringCommitment(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		err := recurrences.Delete(r.Context(), conn, id)
		if errors.Is(err, recurrences.ErrNotFound) {
			writeProblem(w, 404, "recurring-commitment-not-found", "Compromisso não encontrado", "")
			return
		}
		if errors.Is(err, recurrences.ErrLinkedToAutomationRule) {
			recurringCommitmentLinkedProblem(w)
			return
		}
		if err != nil {
			recurringCommitmentUnavailableProblem(w)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
