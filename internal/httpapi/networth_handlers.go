package httpapi

import (
	"database/sql"
	"errors"
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
	CurrencyCode   string         `json:"currency_code"`
	FX             *exchange.Rate `json:"fx,omitempty"`
	StoredCurrency string         `json:"stored_currency"`
	CapturedAt     time.Time      `json:"captured_at"`
	// ValuationVersion 0 marks a snapshot from before foreign balances were
	// valued: it covers BRL accounts and holdings only. Valuation records the
	// day and rates behind every other row.
	ValuationVersion int                 `json:"valuation_version"`
	ValuationStatus  string              `json:"valuation_status"`
	Valuation        *valuation.Metadata `json:"valuation,omitempty"`
	// The figures are absent when the day cannot be valued (status
	// "unavailable"), never zero.
	*netWorthBreakdownDTO
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
		if errors.Is(err, valuation.ErrMissingRate) {
			writeProblem(w, 503, "exchange-unavailable", "Cotação indisponível", "Falta a cotação de uma moeda para valorar o patrimônio líquido em reais. Nenhum saldo foi ignorado.")
			return
		}
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

		report, ok := reportValuation(w, r, conn, latest.CapturedAt)
		if !ok {
			return
		}
		fx := exchange.New(conn, nil)
		toDTO := func(row networth.SnapshotRow, v valuationDTO) netWorthSnapshotDTO {
			dto := netWorthSnapshotDTO{
				CurrencyCode: v.CurrencyCode, StoredCurrency: row.CurrencyCode, CapturedAt: row.CapturedAt,
				ValuationVersion: row.ValuationVersion, ValuationStatus: row.ValuationStatus, Valuation: row.Valuation,
				IsBackfilled: row.IsBackfilled,
			}
			if row.ValuationStatus == networth.ValuationUnavailable {
				return dto
			}
			breakdown := toNetWorthBreakdownDTO(row.Breakdown)
			v.breakdown(&breakdown)
			dto.FX, dto.netWorthBreakdownDTO = &v.FX, &breakdown
			return dto
		}
		series := make([]netWorthSnapshotDTO, len(rows))
		for i, row := range rows {
			day := valuationDTO{CurrencyCode: report.CurrencyCode}
			if row.ValuationStatus != networth.ValuationUnavailable {
				if day, ok = valuationForCurrency(w, r, fx, report.CurrencyCode, row.CapturedAt); !ok {
					return
				}
			}
			series[i] = toDTO(row, day)
		}
		writeJSON(w, http.StatusOK, netWorthResponseDTO{Series: series, Latest: toDTO(latest, report)})
	}
}
