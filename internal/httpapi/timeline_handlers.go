package httpapi

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/timeline"
)

func timelineUnavailableProblem(w http.ResponseWriter) {
	writeProblem(w, 503, "timeline-unavailable", "Linha do tempo temporariamente indisponível", "Tente novamente em instantes.")
}

func invalidTimelineProblem(w http.ResponseWriter, detail string) {
	writeProblem(w, 422, "invalid-timeline-request", "Parâmetros da linha do tempo inválidos", detail)
}

type timelineEntryDTO struct {
	Date             string  `json:"date"`
	Description      string  `json:"description"`
	Amount           string  `json:"amount"`
	ReportableAmount string  `json:"reportable_amount"`
	CategoryID       *string `json:"category_id"`
	CategoryName     string  `json:"category_name"`
	Tier             string  `json:"tier"`
	Source           string  `json:"source"`
	SourceRefID      string  `json:"source_ref_id"`
	ScenarioID       *string `json:"scenario_id"`
}

func timelineEntryToDTO(e timeline.Entry) timelineEntryDTO {
	reportable := e.Amount
	if e.ReportableAmount != nil {
		reportable = *e.ReportableAmount
	}
	return timelineEntryDTO{
		Date:             e.Date.Format(dateOnlyLayout),
		Description:      e.Description,
		Amount:           money.CanonicalDecimal(e.Amount),
		ReportableAmount: money.CanonicalDecimal(reportable),
		CategoryID:       e.CategoryID,
		CategoryName:     e.CategoryName,
		Tier:             string(e.Tier),
		Source:           string(e.Source),
		SourceRefID:      e.SourceRefID,
		ScenarioID:       e.ScenarioID,
	}
}

type timelineDayPointDTO struct {
	Date       string `json:"date"`
	Balance    string `json:"balance"`
	Inflow     string `json:"inflow"`
	Outflow    string `json:"outflow"`
	LowestTier string `json:"lowest_tier"`
}

func timelineDayPointToDTO(p timeline.DayPoint) timelineDayPointDTO {
	return timelineDayPointDTO{
		Date:       p.Date.Format(dateOnlyLayout),
		Balance:    money.CanonicalDecimal(p.Balance),
		Inflow:     money.CanonicalDecimal(p.Inflow),
		Outflow:    money.CanonicalDecimal(p.Outflow),
		LowestTier: string(p.LowestTier),
	}
}

type timelineSeriesDTO struct {
	Points          []timelineDayPointDTO `json:"points"`
	Entries         []timelineEntryDTO    `json:"entries"`
	StartingBalance string                `json:"starting_balance"`
	LowestBalance   timelineDayPointDTO   `json:"lowest_balance"`
	FirstNegative   *string               `json:"first_negative"`
}

func timelineSeriesToDTO(s timeline.Series) timelineSeriesDTO {
	points := make([]timelineDayPointDTO, len(s.Points))
	for i, p := range s.Points {
		points[i] = timelineDayPointToDTO(p)
	}
	entries := make([]timelineEntryDTO, len(s.Entries))
	for i, e := range s.Entries {
		entries[i] = timelineEntryToDTO(e)
	}
	var firstNegative *string
	if s.FirstNegative != nil {
		formatted := s.FirstNegative.Format(dateOnlyLayout)
		firstNegative = &formatted
	}
	return timelineSeriesDTO{
		Points:          points,
		Entries:         entries,
		StartingBalance: money.CanonicalDecimal(s.StartingBalance),
		LowestBalance:   timelineDayPointToDTO(s.LowestBalance),
		FirstNegative:   firstNegative,
	}
}

type periodTotalsDTO struct {
	Income  string `json:"income"`
	Expense string `json:"expense"`
	Result  string `json:"result"`
}

func periodTotalsToDTO(t timeline.PeriodTotals) periodTotalsDTO {
	return periodTotalsDTO{
		Income:  money.CanonicalDecimal(t.Income),
		Expense: money.CanonicalDecimal(t.Expense),
		Result:  money.CanonicalDecimal(t.Result),
	}
}

type scenarioImpactDTO struct {
	ScenarioID   string `json:"scenario_id"`
	ScenarioName string `json:"scenario_name"`
	Delta        string `json:"delta"`
}

func scenarioImpactToDTO(i timeline.Impact) scenarioImpactDTO {
	return scenarioImpactDTO{ScenarioID: i.ScenarioID, ScenarioName: i.ScenarioName, Delta: money.CanonicalDecimal(i.Delta)}
}

// timelineResponseDTO is the full payload: the canonical Series plus the
// totals every presentation layer reads instead of recomputing locally
// (see the umbrella spec's reconciliation principle). Simulation/
// ScenarioImpacts populate only when scenario_ids was non-empty.
// PeriodTotals is always computed (free, folds over series once) so a
// window rarely aligned to whole calendar months — like the Home
// dashboard's — gets an income/expense total for exactly the requested
// range.
type timelineResponseDTO struct {
	valuationDTO
	Base            timelineSeriesDTO   `json:"base"`
	PeriodTotals    periodTotalsDTO     `json:"period_totals"`
	Simulation      *timelineSeriesDTO  `json:"simulation"`
	ScenarioImpacts []scenarioImpactDTO `json:"scenario_impacts"`
}

func splitCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// handleGetTimeline serves GET /api/timeline?reference_date=...&from=...&to=...
// &account_ids=...&category_ids=...&card_numbers=...&scenario_ids=...
// scenario_ids empty (the default) returns {base, simulation: null,
// scenario_impacts: []}; non-empty adds Simulation (Base + those scenarios)
// and one Impact per scenario, each isolated against Base — never all
// standalone scenarios that exist, only the ones actually selected.
func handleGetTimeline(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		referenceRaw := query.Get("reference_date")
		fromRaw := query.Get("from")
		toRaw := query.Get("to")
		if referenceRaw == "" || fromRaw == "" || toRaw == "" {
			invalidTimelineProblem(w, "Informe reference_date, from e to.")
			return
		}
		reference, err := time.Parse(dateOnlyLayout, referenceRaw)
		if err != nil {
			invalidTimelineProblem(w, "reference_date inválida.")
			return
		}
		from, err := time.Parse(dateOnlyLayout, fromRaw)
		if err != nil {
			invalidTimelineProblem(w, "from inválida.")
			return
		}
		to, err := time.Parse(dateOnlyLayout, toRaw)
		if err != nil {
			invalidTimelineProblem(w, "to inválida.")
			return
		}
		if to.Before(from) {
			invalidTimelineProblem(w, "to não pode ser anterior a from.")
			return
		}
		baseParams := timeline.BuildParams{
			From: from, To: to, ReferenceDate: reference,
			AccountIDs:  splitCSV(query.Get("account_ids")),
			CategoryIDs: splitCSV(query.Get("category_ids")),
			CardNumbers: splitCSV(query.Get("card_numbers")),
		}
		scenarioIDs := splitCSV(query.Get("scenario_ids"))

		series, err := timeline.BuildSeries(r.Context(), conn, baseParams)
		if err != nil {
			timelineUnavailableProblem(w)
			return
		}

		response := timelineResponseDTO{
			Base:            timelineSeriesToDTO(series),
			PeriodTotals:    periodTotalsToDTO(timeline.TotalsForPeriod(series)),
			ScenarioImpacts: []scenarioImpactDTO{},
		}

		if len(scenarioIDs) > 0 {
			simulationParams := baseParams
			simulationParams.ScenarioIDs = scenarioIDs
			simulationSeries, err := timeline.BuildSeries(r.Context(), conn, simulationParams)
			if err != nil {
				timelineUnavailableProblem(w)
				return
			}
			simulationDTO := timelineSeriesToDTO(simulationSeries)
			response.Simulation = &simulationDTO

			// baseParams carries no ScenarioIDs and `series` was built from
			// it, which is exactly ScenarioImpact's contract — so each
			// impact costs one BuildSeries instead of rebuilding the Base
			// alongside it.
			for _, scenarioID := range scenarioIDs {
				impact, err := timeline.ScenarioImpact(r.Context(), conn, baseParams, series, scenarioID)
				if err != nil {
					timelineUnavailableProblem(w)
					return
				}
				response.ScenarioImpacts = append(response.ScenarioImpacts, scenarioImpactToDTO(impact))
			}
		}

		valuation, ok := reportValuation(w, r, conn, time.Now())
		if !ok {
			return
		}
		response.valuationDTO = valuation
		valuation.series(&response.Base)
		valuation.fields(&response.PeriodTotals.Income, &response.PeriodTotals.Expense, &response.PeriodTotals.Result)
		if response.Simulation != nil {
			valuation.series(response.Simulation)
		}
		for i := range response.ScenarioImpacts {
			valuation.fields(&response.ScenarioImpacts[i].Delta)
		}
		writeJSON(w, http.StatusOK, response)
	}
}

type timelineRangeDTO struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// handleGetTimelineDataRange serves GET /api/timeline/range — the widest
// window the balance curve can meaningfully cover. It exists so a client
// offering a "todo o período" option doesn't have to guess a start date, or
// ask for a decade of days to be safe.
func handleGetTimelineDataRange(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dataRange, err := timeline.LoadDataRange(r.Context(), conn, time.Now().UTC())
		if err != nil {
			timelineUnavailableProblem(w)
			return
		}
		writeJSON(w, http.StatusOK, timelineRangeDTO{
			From: dataRange.From.Format(dateOnlyLayout),
			To:   dataRange.To.Format(dateOnlyLayout),
		})
	}
}
