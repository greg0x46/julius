package networth

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/transactions"
	"github.com/greg0x46/julius/internal/valuation"
)

// Querier is satisfied by both *sql.DB and *sql.Tx.
type Querier = transactions.Querier

// newValuation values at the given day with the application's exchange service.
func newValuation(q Querier, at time.Time) *valuation.Valuation {
	return valuation.New(exchange.New(q, nil), at)
}

// investmentBalance sums every synced investment's current balance, each valued
// in BRL, plus the manual investments' contribution. Balances are stored as
// TEXT to stay exact, so the summing happens here in Go with decimal.Decimal
// rather than through a float-lossy SQL SUM.
func investmentBalance(ctx context.Context, q Querier, valued *valuation.Valuation) (decimal.Decimal, error) {
	manual, err := investments.ManualNetWorthValued(ctx, q, valued)
	if err != nil {
		return decimal.Zero, err
	}
	rows, err := q.QueryContext(ctx, `SELECT balance, currency_code FROM financial_investments WHERE balance IS NOT NULL`)
	if err != nil {
		return decimal.Decimal{}, err
	}
	defer rows.Close()
	type holding struct {
		amount   decimal.Decimal
		currency string
	}
	var holdings []holding
	for rows.Next() {
		var raw string
		var currency sql.NullString
		if err := rows.Scan(dbtypes.Decimal(&raw), &currency); err != nil {
			return decimal.Decimal{}, err
		}
		amount, err := decimal.NewFromString(raw)
		if err != nil {
			return decimal.Decimal{}, fmt.Errorf("parse investment balance %q: %w", raw, err)
		}
		code := valuation.Currency(nil)
		if currency.Valid {
			code = valuation.Currency(&currency.String)
		}
		holdings = append(holdings, holding{amount, code})
	}
	if err := rows.Err(); err != nil {
		return decimal.Zero, err
	}
	rows.Close()
	total := manual
	for _, h := range holdings {
		value, err := valued.Convert(ctx, h.amount, h.currency)
		if err != nil {
			return decimal.Zero, err
		}
		total = total.Add(value)
	}
	return total, nil
}

// InvestmentValue is today's investments line of net worth in BRL, without
// valuing cash or card balances: a missing rate on unrelated foreign cash
// cannot make it unavailable.
func InvestmentValue(ctx context.Context, q Querier) (decimal.Decimal, error) {
	return investmentBalance(ctx, q, newValuation(q, time.Now()))
}

// Compute is today's wealth from live data, valued in BRL at today's rates:
// cash (non-CREDIT account balances) plus investments.
func Compute(ctx context.Context, q Querier) (Breakdown, error) {
	return ComputeValued(ctx, q, newValuation(q, time.Now()))
}

// ComputeValued is Compute with the valuation (and so the rates it used)
// supplied by the caller.
func ComputeValued(ctx context.Context, q Querier, valued *valuation.Valuation) (Breakdown, error) {
	// Cash is the same transactions.CashOnHandByCurrency the timeline anchors
	// on, so both views start from the same native balances. Credit card
	// accounts are excluded there: a card is credit, not wealth.
	cash, err := transactions.CashOnHandValued(ctx, q, nil, valued)
	if err != nil {
		return Breakdown{}, err
	}
	investments, err := investmentBalance(ctx, q, valued)
	if err != nil {
		return Breakdown{}, err
	}
	return Breakdown{CashBalance: cash, InvestmentBalance: investments}.TotalsFor(), nil
}
