package httpapi

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/networth"
	"github.com/greg0x46/julius/internal/valuation"
)

type netWorthBreakdownDTO struct {
	CashBalance       string `json:"cash_balance"`
	InvestmentBalance string `json:"investment_balance"`
	Total             string `json:"total"`
}

func toNetWorthBreakdownDTO(b networth.Breakdown) netWorthBreakdownDTO {
	return netWorthBreakdownDTO{
		CashBalance:       money.CanonicalDecimal(b.CashBalance),
		InvestmentBalance: money.CanonicalDecimal(b.InvestmentBalance),
		Total:             money.CanonicalDecimal(b.Total),
	}
}

type netWorthPointDTO struct {
	CurrencyCode string         `json:"currency_code"`
	FX           *exchange.Rate `json:"fx,omitempty"`
	Date         time.Time      `json:"date"`
	// ValuationStatus is "complete", "stale" (a stale rate was used) or
	// "unavailable". Valuation records the day and the rates behind the figures.
	ValuationStatus string              `json:"valuation_status"`
	Valuation       *valuation.Metadata `json:"valuation,omitempty"`
	// The figures are absent when the day cannot be valued (status
	// "unavailable"), never zero.
	*netWorthBreakdownDTO
}

type netWorthResponseDTO struct {
	Series []netWorthPointDTO `json:"series"`
	Latest netWorthPointDTO   `json:"latest"`
}

// fxDay is the day a point's figures were valued on, which is the day its FX
// must come from: today's point is valued on the latest day a rate can exist.
func fxDay(p networth.Point) time.Time {
	if p.Valuation != nil {
		return p.Valuation.ValuedAt
	}
	return p.Day
}

func netWorthUnavailableProblem(w http.ResponseWriter) {
	writeProblem(w, 503, "net-worth-unavailable", "Patrimônio temporariamente indisponível", "Tente novamente em instantes.")
}

// handleGetNetWorth returns wealth (cash plus investments) for every day since
// the first cash transaction, derived on this request, with today as "latest".
// Nothing is stored, so the series always agrees with the rest of the app.
func handleGetNetWorth(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		loc, ok := configuredZone(w, r, conn)
		if !ok {
			return
		}
		points, err := networth.Series(ctx, conn, time.Now().In(loc))
		if errors.Is(err, valuation.ErrMissingRate) {
			writeProblem(w, 503, "exchange-unavailable", "Cotação indisponível", "Falta a cotação de uma moeda para valorar o patrimônio em reais. Nenhum saldo foi ignorado.")
			return
		}
		if err != nil {
			log.Printf("net_worth_failed: %v", err)
			netWorthUnavailableProblem(w)
			return
		}
		latest := points[len(points)-1]

		report, ok := reportValuation(w, r, conn, fxDay(latest))
		if !ok {
			return
		}
		fx := exchange.New(conn, nil)
		toDTO := func(p networth.Point, v valuationDTO) netWorthPointDTO {
			dto := netWorthPointDTO{CurrencyCode: v.CurrencyCode, Date: p.Day, ValuationStatus: p.Status, Valuation: p.Valuation}
			if p.Status == networth.ValuationUnavailable {
				return dto
			}
			breakdown := toNetWorthBreakdownDTO(p.Breakdown)
			v.breakdown(&breakdown)
			dto.FX, dto.netWorthBreakdownDTO = &v.FX, &breakdown
			return dto
		}
		series := make([]netWorthPointDTO, len(points))
		for i, p := range points {
			day := valuationDTO{CurrencyCode: report.CurrencyCode}
			if p.Status != networth.ValuationUnavailable {
				if day, ok = valuationForCurrency(w, r, fx, report.CurrencyCode, fxDay(p)); !ok {
					return
				}
			}
			series[i] = toDTO(p, day)
		}
		writeJSON(w, http.StatusOK, netWorthResponseDTO{Series: series, Latest: toDTO(latest, report)})
	}
}
