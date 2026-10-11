// Package fx values native amounts using explicitly stored, dated rates.
package fx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/greg0x46/julius/internal/db"
	"github.com/shopspring/decimal"
)

const ReferenceCurrency = "BRL"
const RatePolicy = "latest_observed_at_or_before_v1"

var ErrMissingRate = errors.New("FX rate unavailable")

type Querier interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type Rate struct {
	Value decimal.Decimal `json:"value"`
	ObservedAt time.Time `json:"observed_at"`
	Source string `json:"source"`
}

type Metadata struct {
	ReferenceCurrency string `json:"reference_currency"`
	ValuedAt time.Time `json:"valued_at"`
	RatePolicy string `json:"rate_policy"`
	Rates map[string]Rate `json:"rates"`
}

type Valuation struct {
	Metadata
}

func New(at time.Time) *Valuation {
	return &Valuation{Metadata: Metadata{ReferenceCurrency: ReferenceCurrency, ValuedAt: at.UTC(), RatePolicy: RatePolicy, Rates: map[string]Rate{}}}
}

// Currency preserves the existing convention that an unspecified currency is BRL.
func Currency(code *string) string {
	if code == nil || strings.TrimSpace(*code) == "" {
		return ReferenceCurrency
	}
	return strings.ToUpper(strings.TrimSpace(*code))
}

// StoreRate records reference-currency units per native unit. It never changes balances.
func StoreRate(ctx context.Context, q Querier, currency string, rate Rate) error {
	currency = Currency(&currency)
	if len(currency) != 3 || currency == ReferenceCurrency || !rate.Value.IsPositive() || rate.ObservedAt.IsZero() || strings.TrimSpace(rate.Source) == "" {
		return fmt.Errorf("invalid FX rate")
	}
	_, err := q.ExecContext(ctx, `INSERT INTO fx_rates (currency_code, reference_currency, observed_at, rate, source)
		VALUES (?, ?, ?, ?, ?) ON CONFLICT (currency_code, reference_currency, observed_at)
		DO UPDATE SET rate = excluded.rate, source = excluded.source`, currency, ReferenceCurrency, db.FormatTime(rate.ObservedAt), rate.Value.String(), rate.Source)
	return err
}

func (v *Valuation) Convert(ctx context.Context, q Querier, amount decimal.Decimal, currency string) (decimal.Decimal, error) {
	currency = Currency(&currency)
	if currency == v.ReferenceCurrency || amount.IsZero() {
		return amount, nil
	}
	rate, ok := v.Rates[currency]
	if !ok {
		var raw, observed string
		err := q.QueryRowContext(ctx, `SELECT rate, observed_at, source FROM fx_rates
			WHERE currency_code = ? AND reference_currency = ? AND observed_at <= ?
			ORDER BY observed_at DESC LIMIT 1`, currency, v.ReferenceCurrency, db.FormatTime(v.ValuedAt)).Scan(&raw, &observed, &rate.Source)
		if errors.Is(err, sql.ErrNoRows) {
			return decimal.Zero, fmt.Errorf("%w: %s/%s at %s", ErrMissingRate, currency, v.ReferenceCurrency, v.ValuedAt.Format(time.RFC3339))
		}
		if err != nil { return decimal.Zero, err }
		rate.Value, err = decimal.NewFromString(raw)
		if err != nil { return decimal.Zero, err }
		if !rate.Value.IsPositive() { return decimal.Zero, fmt.Errorf("invalid FX rate for %s", currency) }
		rate.ObservedAt, err = db.ParseTime(observed)
		if err != nil { return decimal.Zero, err }
		v.Rates[currency] = rate
	}
	return amount.Mul(rate.Value), nil
}
