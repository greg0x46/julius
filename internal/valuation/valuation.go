// Package valuation values native amounts in the BRL base using dated
// exchange rates, recording which rates were used. Native values are never
// changed; a missing rate is an error, never a 1:1 estimate.
package valuation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/money"
)

// BaseCurrency is the stored valuation currency; display currencies are
// applied on top of it when a report is read.
const BaseCurrency = money.BRL

// RatePolicy names how a rate is chosen, so a stored valuation stays
// interpretable if the policy ever changes.
const RatePolicy = "exchange_on_valued_day_v1"

// ErrMissingRate means a currency could not be valued on the valuation day.
var ErrMissingRate = errors.New("exchange rate unavailable for valuation")

// Resolver is the part of exchange.Service the valuation needs.
type Resolver interface {
	On(ctx context.Context, base, quote string, t time.Time) (exchange.Rate, error)
}

// Rate is one rate that took part in a valuation.
type Rate struct {
	Value       decimal.Decimal `json:"value"`
	Inverted    bool            `json:"inverted"`
	EffectiveOn time.Time       `json:"effective_on"`
	Source      string          `json:"source"`
	Stale       bool            `json:"stale"`
}

// Metadata is what a stored valuation keeps about how it was computed.
type Metadata struct {
	BaseCurrency string          `json:"base_currency"`
	ValuedAt     time.Time       `json:"valued_at"`
	RatePolicy   string          `json:"rate_policy"`
	Rates        map[string]Rate `json:"rates"`
}

// Valuation converts amounts at one valuation instant and remembers the
// rates it used. It is not safe for concurrent use.
type Valuation struct {
	Metadata
	rates Resolver
}

// New values at the calendar day of at.
func New(rates Resolver, at time.Time) *Valuation {
	return &Valuation{
		Metadata: Metadata{BaseCurrency: BaseCurrency, ValuedAt: at.UTC(), RatePolicy: RatePolicy, Rates: map[string]Rate{}},
		rates:    rates,
	}
}

// Currency applies the convention that an unspecified currency is BRL.
func Currency(code *string) string {
	if code == nil || strings.TrimSpace(*code) == "" {
		return BaseCurrency
	}
	return strings.ToUpper(strings.TrimSpace(*code))
}

// Convert values amount of currency in the base currency.
func (v *Valuation) Convert(ctx context.Context, amount decimal.Decimal, currency string) (decimal.Decimal, error) {
	currency = strings.ToUpper(currency)
	if currency == v.BaseCurrency || amount.IsZero() {
		return amount, nil
	}
	rate, ok := v.Rates[currency]
	if !ok {
		got, err := v.rates.On(ctx, currency, v.BaseCurrency, v.ValuedAt)
		if err != nil {
			if errors.Is(err, exchange.ErrUnavailable) || errors.Is(err, exchange.ErrInvalidCurrency) {
				return decimal.Zero, fmt.Errorf("%w: %s/%s on %s: %w", ErrMissingRate, currency, v.BaseCurrency, v.ValuedAt.Format(time.DateOnly), err)
			}
			return decimal.Zero, err
		}
		rate = Rate{Value: got.Value, Inverted: got.Inverted, EffectiveOn: got.EffectiveOn, Source: got.Source, Stale: got.Stale}
		v.Rates[currency] = rate
	}
	return exchange.Rate{Value: rate.Value, Inverted: rate.Inverted}.Convert(amount), nil
}

// Stale reports whether any rate used was stale.
func (v *Valuation) Stale() bool {
	for _, rate := range v.Rates {
		if rate.Stale {
			return true
		}
	}
	return false
}
