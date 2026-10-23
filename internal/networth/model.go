// Package networth computes the user's wealth ("patrimônio"): what has
// already been realized, cash plus investments, valued in BRL. It counts no
// liability and nothing in the future: credit cards, debts and receivables
// are not wealth. The package keeps its historical name.
//
// Nothing is stored. Today's figure and every past day are derived on read
// (Compute, Series), so a corrected transaction, a late quote or an edited
// operation shows up in the whole history at once. Every figure is in BRL:
// balances and holdings in other currencies are converted with the rate of the
// day being valued, and a currency without a rate makes that day unavailable
// rather than being dropped.
package networth

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/valuation"
)

// Breakdown is a point-in-time decomposition of wealth: Total = CashBalance +
// InvestmentBalance.
type Breakdown struct {
	CashBalance       decimal.Decimal
	InvestmentBalance decimal.Decimal
	Total             decimal.Decimal
}

// TotalsFor derives Total from the categories.
func (b Breakdown) TotalsFor() Breakdown {
	b.Total = b.CashBalance.Add(b.InvestmentBalance)
	return b
}

// Point is the wealth at the end of one calendar day.
type Point struct {
	Day       time.Time
	Status    string
	Breakdown Breakdown
	// Valuation records the day and the rates the figures were valued with.
	Valuation *valuation.Metadata
}

// Valuation statuses of a point.
const (
	ValuationComplete    = "complete"    // every currency valued
	ValuationStale       = "stale"       // valued using at least one stale rate
	ValuationUnavailable = "unavailable" // the day cannot be valued; carries no figures
)
