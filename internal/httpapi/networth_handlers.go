package httpapi

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/networth"
)

type netWorthBreakdownDTO struct {
	CashBalance       string `json:"cash_balance"`
	InvestmentBalance string `json:"investment_balance"`
	CreditCardBalance string `json:"credit_card_balance"`
	PayablesDebt      string `json:"payables_debt"`
	TotalAssets       string `json:"total_assets"`
	TotalLiabilities  string `json:"total_liabilities"`
	NetWorth          string `json:"net_worth"`
}

func toNetWorthBreakdownDTO(b networth.Breakdown) netWorthBreakdownDTO {
	return netWorthBreakdownDTO{
		CashBalance:       money.CanonicalDecimal(b.CashBalance),
		InvestmentBalance: money.CanonicalDecimal(b.InvestmentBalance),
		CreditCardBalance: money.CanonicalDecimal(b.CreditCardBalance),
		PayablesDebt:      money.CanonicalDecimal(b.PayablesDebt),
		TotalAssets:       money.CanonicalDecimal(b.TotalAssets),
		TotalLiabilities:  money.CanonicalDecimal(b.TotalLiabilities),
		NetWorth:          money.CanonicalDecimal(b.NetWorth),
	}
}

type netWorthSnapshotDTO struct {
	valuationDTO
	StoredCurrency string    `json:"stored_currency"`
	CapturedAt     time.Time `json:"captured_at"`
	netWorthBreakdownDTO
	// IsBackfilled mirrors SnapshotRow.IsBackfilled: true when this row was
	// reconstructed by networth.Backfill rather than captured live, in
	// which case InvestmentBalance is always "0" — see Backfill's doc
	// comment for why investment history can't be reconstructed.
	IsBackfilled bool `json:"is_backfilled"`
}

type netWorthResponseDTO struct {
	Series []netWorthSnapshotDTO `json:"series"`
	Latest netWorthSnapshotDTO   `json:"latest"`
}

func netWorthUnavailableProblem(w http.ResponseWriter) {
	writeProblem(w, 503, "net-worth-unavailable", "Patrimônio líquido temporariamente indisponível", "Tente novamente em instantes.")
}

// handleGetNetWorth snapshots today's net worth (idempotent per calendar
// day — see networth.Snapshot), fills in any missing recent days via
// networth.Backfill, and then returns the full stored series alongside that
// just-written snapshot as "latest", so the frontend never needs a second
// request to know the current breakdown. Backfill is cheap to call on every
// request: once a day's row exists (live or backfilled) it's never
// recomputed, so this only does real work the first few times the table is
// mostly empty.
func handleGetNetWorth(conn *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		latest, err := networth.Snapshot(ctx, conn)
		if err != nil {
			netWorthUnavailableProblem(w)
			return
		}
		if err := networth.Backfill(ctx, conn, time.Now()); err != nil {
			netWorthUnavailableProblem(w)
			return
		}

		from := latest.CapturedAt.AddDate(-100, 0, 0) // effectively "since the beginning" — the table never holds more than a few years of daily rows
		rows, err := networth.List(ctx, conn, from, latest.CapturedAt)
		if err != nil {
			netWorthUnavailableProblem(w)
			return
		}

		series := make([]netWorthSnapshotDTO, len(rows))
		for i, row := range rows {
			valuation, ok := reportValuation(w, r, conn, row.CapturedAt)
			if !ok {
				return
			}
			breakdown := toNetWorthBreakdownDTO(row.Breakdown)
			valuation.breakdown(&breakdown)
			series[i] = netWorthSnapshotDTO{valuationDTO: valuation, StoredCurrency: row.CurrencyCode, CapturedAt: row.CapturedAt, netWorthBreakdownDTO: breakdown, IsBackfilled: row.IsBackfilled}
		}
		valuation, ok := reportValuation(w, r, conn, latest.CapturedAt)
		if !ok {
			return
		}
		breakdown := toNetWorthBreakdownDTO(latest.Breakdown)
		valuation.breakdown(&breakdown)
		writeJSON(w, http.StatusOK, netWorthResponseDTO{
			Series: series,
			Latest: netWorthSnapshotDTO{valuationDTO: valuation, StoredCurrency: latest.CurrencyCode, CapturedAt: latest.CapturedAt, netWorthBreakdownDTO: breakdown, IsBackfilled: latest.IsBackfilled},
		})
	}
}
