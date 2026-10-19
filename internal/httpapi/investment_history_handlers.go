package httpapi

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/investments"
)

// History samples the portfolio at month ends, with a separate start-day
// valuation. Each flow covers only its stated inclusive period.
func handleGetInvestmentHistory(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		from, ok := investmentQueryDate(r, "from")
		if !ok || from == nil {
			writeInvestmentInvalid(w, "from deve estar no formato AAAA-MM-DD.")
			return
		}
		to, ok := investmentQueryDate(r, "to")
		if !ok || to == nil {
			writeInvestmentInvalid(w, "to deve estar no formato AAAA-MM-DD.")
			return
		}
		today := investments.ProviderDay(time.Now())
		if to.After(today) {
			to = &today
		}
		if from.After(*to) || to.Year()*12+int(to.Month())-(from.Year()*12+int(from.Month())) >= 120 {
			writeInvestmentInvalid(w, "Escolha um período de até 120 meses, com início até hoje.")
			return
		}
		reference, ok := reportValuation(w, r, conn, *to)
		if !ok {
			return
		}
		build := func(start, end time.Time) (investmentPeriodDTO, bool) {
			p, err := investments.BuildPortfolio(r.Context(), conn, investments.PortfolioQuery{From: &start, To: end, GroupBy: investments.PortfolioGroupNone})
			if err != nil {
				writeInvestmentProblem(w, err)
				return investmentPeriodDTO{}, false
			}
			day, ok := valuationForCurrency(w, r, exchange.New(conn, nil), reference.CurrencyCode, end)
			if !ok {
				return investmentPeriodDTO{}, false
			}
			return investmentPeriodToDTO(p, day), true
		}
		baseline, ok := build(*from, *from)
		if !ok {
			return
		}
		periods := []investmentPeriodDTO{}
		for start := *from; !start.After(*to); {
			end := time.Date(start.Year(), start.Month(), dates.DaysInMonth(start.Year(), start.Month()), 0, 0, 0, 0, time.UTC)
			if end.After(*to) {
				end = *to
			}
			period, ok := build(start, end)
			if !ok {
				return
			}
			periods = append(periods, period)
			start = end.AddDate(0, 0, 1)
		}
		writeJSON(w, http.StatusOK, struct {
			Baseline investmentPeriodDTO   `json:"baseline"`
			Periods  []investmentPeriodDTO `json:"periods"`
		}{baseline, periods})
	}
}
