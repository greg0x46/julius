package valuation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/valuation"
)

type resolver struct {
	calls int
	rate  exchange.Rate
	err   error
}

func (r *resolver) On(_ context.Context, base, quote string, _ time.Time) (exchange.Rate, error) {
	r.calls++
	if r.err != nil {
		return exchange.Rate{}, r.err
	}
	rate := r.rate
	rate.Base, rate.Quote = base, quote
	return rate, nil
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestConvertUsesOneRatePerCurrencyAndRecordsIt(t *testing.T) {
	r := &resolver{rate: exchange.Rate{Value: dec("5"), Source: "test"}}
	v := valuation.New(r, time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC))
	ctx := context.Background()

	for _, amount := range []string{"100", "20"} {
		got, err := v.Convert(ctx, dec(amount), "USD")
		if err != nil || !got.Equal(dec(amount).Mul(dec("5"))) {
			t.Fatalf("Convert(%s) = %s, %v", amount, got, err)
		}
	}
	if r.calls != 1 {
		t.Fatalf("rate resolved %d times, want once per currency", r.calls)
	}
	if rate := v.Rates["USD"]; !rate.Value.Equal(dec("5")) || rate.Source != "test" {
		t.Fatalf("recorded rate: %+v", rate)
	}
}

func TestBRLAndZeroNeedNoRate(t *testing.T) {
	r := &resolver{err: errors.New("must not be asked")}
	v := valuation.New(r, time.Now())
	if got, err := v.Convert(context.Background(), dec("12.34"), "BRL"); err != nil || !got.Equal(dec("12.34")) {
		t.Fatalf("BRL = %s, %v", got, err)
	}
	if got, err := v.Convert(context.Background(), decimal.Zero, "USD"); err != nil || !got.IsZero() {
		t.Fatalf("zero = %s, %v", got, err)
	}
	if r.calls != 0 {
		t.Fatal("asked for a rate it did not need")
	}
}

func TestMissingRateIsAnErrorNotAnEstimate(t *testing.T) {
	v := valuation.New(&resolver{err: exchange.ErrUnavailable}, time.Now())
	if _, err := v.Convert(context.Background(), dec("1"), "USD"); !errors.Is(err, valuation.ErrMissingRate) {
		t.Fatalf("err = %v", err)
	}
	if len(v.Rates) != 0 {
		t.Fatalf("a failed lookup was recorded: %+v", v.Rates)
	}
	other := errors.New("database is down")
	v = valuation.New(&resolver{err: other}, time.Now())
	if _, err := v.Convert(context.Background(), dec("1"), "USD"); !errors.Is(err, other) || errors.Is(err, valuation.ErrMissingRate) {
		t.Fatalf("infrastructure failure was reported as a missing rate: %v", err)
	}
}

func TestInvertedAndStaleRates(t *testing.T) {
	v := valuation.New(&resolver{rate: exchange.Rate{Value: dec("0.2"), Inverted: true, Stale: true}}, time.Now())
	got, err := v.Convert(context.Background(), dec("1"), "USD")
	if err != nil || !got.Equal(dec("5")) {
		t.Fatalf("inverted rate gave %s, %v", got, err)
	}
	if !v.Stale() {
		t.Fatal("stale rate not reported")
	}
}

func TestCurrencyDefaultsToBRL(t *testing.T) {
	usd := " usd "
	if valuation.Currency(nil) != "BRL" || valuation.Currency(&usd) != "USD" {
		t.Fatal("currency convention changed")
	}
}
