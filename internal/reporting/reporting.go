// Package reporting converts cash flows without changing transaction money.
package reporting

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

const ReferenceCurrency = "BRL"

type Resolver interface {
	On(context.Context, string, string, time.Time) (exchange.Rate, error)
}

type Issue struct {
	Currency string `json:"currency"`
	Reason   string `json:"reason"`
	Date     string `json:"date,omitempty"`
	Account  string `json:"account,omitempty"`
}

type Conversion struct {
	Issue  *Issue
	Amount *decimal.Decimal
	Status string
}

func Convert(ctx context.Context, rates Resolver, amount decimal.Decimal, currency string, day *time.Time) Conversion {
	if currency == ReferenceCurrency {
		return Conversion{Amount: &amount, Status: "complete"}
	}
	if day == nil {
		return unavailable(currency, day, "historical_unavailable")
	}
	rate, err := rates.On(ctx, currency, ReferenceCurrency, *day)
	if err != nil {
		reason := "provider_failure"
		if errors.Is(err, exchange.ErrUnsupported) {
			reason = "unsupported"
		} else if errors.Is(err, exchange.ErrHistorical) {
			reason = "historical_unavailable"
		}
		return unavailable(currency, day, reason)
	}
	value := rate.Convert(amount)
	status := "complete"
	if rate.Stale {
		status = "stale"
	}
	return Conversion{Amount: &value, Status: status}
}

func unavailable(currency string, day *time.Time, reason string) Conversion {
	issue := &Issue{Currency: currency, Reason: reason}
	if day != nil {
		issue.Date = day.Format("2006-01-02")
	}
	return Conversion{Status: "unavailable", Issue: issue}
}

type Totals struct {
	CurrencyCode string `json:"currency_code"`
	Inflow       string `json:"inflow"`
	Outflow      string `json:"outflow"`
	Balance      string `json:"balance"`
}

type Summary struct {
	Issues       []Issue `json:"issues,omitempty"`
	CurrencyCode string  `json:"currency_code"`
	Status       string  `json:"status"`
	Eligible     int     `json:"eligible"`
	Converted    int     `json:"converted"`
	Missing      int     `json:"missing"`
	Stale        int     `json:"stale"`
	Totals       *Totals `json:"totals"`
}

type Accumulator struct {
	issues                              []Issue
	eligible, converted, missing, stale int
	inflow, outflow                     decimal.Decimal
}

func (a *Accumulator) Add(c Conversion, direction money.Classification) {
	a.eligible++
	if c.Amount == nil {
		a.missing++
		if c.Issue != nil {
			found := false
			for _, issue := range a.issues {
				if issue == *c.Issue {
					found = true
					break
				}
			}
			if !found {
				a.issues = append(a.issues, *c.Issue)
			}
		}
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
	s := Summary{Issues: append([]Issue(nil), a.issues...), CurrencyCode: ReferenceCurrency, Status: "complete", Eligible: a.eligible, Converted: a.converted, Missing: a.missing, Stale: a.stale}
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
	Detail   string
	Coverage Summary
	Totals   []Totals
}

func (e *IncompleteError) Error() string {
	if len(e.Coverage.Issues) > 0 {
		messages := make([]string, 0, len(e.Coverage.Issues))
		for _, issue := range e.Coverage.Issues {
			label := issue.Currency
			if issue.Account != "" {
				label += " (" + issue.Account + ")"
			}
			switch issue.Reason {
			case "unsupported":
				messages = append(messages, fmt.Sprintf("%s: o provedor não oferece conversão para BRL. Sincronizar não resolve essa limitação.", label))
			case "historical_unavailable":
				messages = append(messages, fmt.Sprintf("%s: não há cotação para %s. Consulte os valores na moeda original ou escolha outro período.", label, issue.Date))
			default:
				messages = append(messages, fmt.Sprintf("%s: não foi possível consultar a cotação agora. Tente novamente mais tarde.", label))
			}
		}
		return strings.Join(messages, " ")
	}
	if e.Detail != "" {
		return e.Detail
	}
	return "Faltam cotações históricas para o relatório."
}
