package reporting_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/reporting"
)

type resolver struct {
	rate exchange.Rate
	err  error
	day  time.Time
}

func (r *resolver) On(_ context.Context, base, quote string, day time.Time) (exchange.Rate, error) {
	r.day = day
	return r.rate, r.err
}

func TestReportingConversionCoverage(t *testing.T) {
	day := time.Date(2020, 2, 4, 0, 0, 0, 0, time.UTC)
	r := &resolver{rate: exchange.Rate{Value: decimal.RequireFromString("5.123456789")}}
	usd := reporting.Convert(context.Background(), r, decimal.RequireFromString("10.05"), "USD", &day)
	if usd.Amount.String() != "51.49074072945" || !r.day.Equal(day) {
		t.Fatalf("historical conversion = %+v on %v", usd, r.day)
	}
	var a reporting.Accumulator
	a.Add(usd, money.Outflow)
	r.err = errors.New("missing historical rate")
	missing := reporting.Convert(context.Background(), r, decimal.NewFromInt(12), "EUR", &day)
	a.Add(missing, money.Inflow)
	if summary := a.Summary(); summary.Status != "incomplete" || summary.Totals != nil || summary.Missing != 1 || summary.Converted != 1 {
		t.Fatalf("partial = %+v", summary)
	}
	if c := reporting.Convert(context.Background(), nil, decimal.NewFromInt(10), "BRL", nil); c.Amount.String() != "10" || c.Status != "complete" {
		t.Fatalf("BRL = %+v", c)
	}
	if c := reporting.Convert(context.Background(), r, decimal.NewFromInt(1), "EUR", nil); c.Amount != nil {
		t.Fatal("undated foreign value converted")
	}
}

func TestStaleReportingIsDisclosed(t *testing.T) {
	r := &resolver{rate: exchange.Rate{Value: decimal.RequireFromString("0.2"), Inverted: true, Stale: true}}
	day := time.Now()
	converted := reporting.Convert(context.Background(), r, decimal.NewFromInt(10), "USD", &day)
	var a reporting.Accumulator
	a.Add(converted, money.Inflow)
	summary := a.Summary()
	if summary.Status != "stale" || summary.Stale != 1 || summary.Totals.Inflow != "50.000000000000" {
		t.Fatalf("stale inverse = %+v", summary)
	}
}

func TestReportingFailureReasons(t *testing.T) {
	day := time.Date(2024, 5, 18, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		err    error
		reason string
	}{
		{exchange.ErrUnsupported, "unsupported"},
		{exchange.ErrHistorical, "historical_unavailable"},
		{exchange.ErrProviderFailure, "provider_failure"},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			r := &resolver{err: tc.err}
			converted := reporting.Convert(context.Background(), r, decimal.NewFromInt(100), "ARS", &day)
			if converted.Amount != nil || converted.Issue.Reason != tc.reason || converted.Issue.Currency != "ARS" || converted.Issue.Date != "2024-05-18" {
				t.Fatalf("%+v", converted)
			}
			converted.Issue.Account = "Conta sintética"
			var a reporting.Accumulator
			a.Add(converted, money.Inflow)
			a.Add(converted, money.Inflow)
			summary := a.Summary()
			if summary.Totals != nil || summary.Missing != 2 || len(summary.Issues) != 1 {
				t.Fatalf("%+v", summary)
			}
			err := &reporting.IncompleteError{Coverage: summary}
			if err.Error() == "Faltam cotações históricas para o relatório." {
				t.Fatal(err)
			}
		})
	}
}
