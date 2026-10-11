// Package reporting converts cash flows without changing transaction money.
package reporting

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/money"
)

const ReferenceCurrency = "BRL"

type Resolver interface {
	On(context.Context, string, string, time.Time) (exchange.Rate, error)
}

type Conversion struct {
	Amount *decimal.Decimal
	Status string
}

func Convert(ctx context.Context, rates Resolver, amount decimal.Decimal, currency string, day *time.Time) Conversion {
	if currency == ReferenceCurrency {
		return Conversion{Amount: &amount, Status: "complete"}
	}
	if day == nil {
		return Conversion{Status: "unavailable"}
	}
	rate, err := rates.On(ctx, currency, ReferenceCurrency, *day)
	if err != nil {
		return Conversion{Status: "unavailable"}
	}
	value := rate.Convert(amount)
	status := "complete"
	if rate.Stale {
		status = "stale"
	}
	return Conversion{Amount: &value, Status: status}
}

type Totals struct {
	CurrencyCode string `json:"currency_code"`
	Inflow       string `json:"inflow"`
	Outflow      string `json:"outflow"`
	Balance      string `json:"balance"`
}

type Summary struct {
	CurrencyCode string  `json:"currency_code"`
	Status       string  `json:"status"`
	Eligible     int     `json:"eligible"`
	Converted    int     `json:"converted"`
	Missing      int     `json:"missing"`
	Stale        int     `json:"stale"`
	Totals       *Totals `json:"totals"`
}

type Accumulator struct {
	eligible, converted, missing, stale int
	inflow, outflow                     decimal.Decimal
}

func (a *Accumulator) Add(c Conversion, direction money.Classification) {
	a.eligible++
	if c.Amount == nil {
		a.missing++
		return
	}
	a.converted++
	if c.Status == "stale" {
		a.stale++
	}
	if direction == money.Inflow {
		a.inflow = a.inflow.Add(c.Amount.Abs())
	} else {
		a.outflow = a.outflow.Add(c.Amount.Abs())
	}
}

func (a Accumulator) Summary() Summary {
	s := Summary{CurrencyCode: ReferenceCurrency, Status: "complete", Eligible: a.eligible, Converted: a.converted, Missing: a.missing, Stale: a.stale}
	if a.missing > 0 {
		s.Status = "incomplete"
		if a.converted == 0 {
			s.Status = "unavailable"
		}
		return s
	}
	if a.stale > 0 {
		s.Status = "stale"
	}
	s.Totals = &Totals{CurrencyCode: ReferenceCurrency, Inflow: money.CanonicalDecimal(a.inflow), Outflow: money.CanonicalDecimal(a.outflow), Balance: money.CanonicalDecimal(a.inflow.Sub(a.outflow))}
	return s
}

type IncompleteError struct {
	Coverage Summary
	Totals   []Totals
}

func (e *IncompleteError) Error() string { return "Faltam cotações históricas para o relatório." }
