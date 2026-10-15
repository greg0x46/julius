package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/greg0x46/julius/internal/exchange"
)

type currencyCoverage struct{ status string }

func (p currencyCoverage) Currencies(context.Context) exchange.Coverage {
	currencies := []exchange.Currency{{Code: "BRL"}}
	if p.status != "unavailable" {
		currencies = append(currencies, exchange.Currency{Code: "ARS", StartDate: "1998-07-07", EndDate: "2026-10-14"})
	}
	return exchange.Coverage{Currencies: currencies, Status: p.status, Source: "synthetic"}
}

func TestAccountCurrenciesProviderCoverage(t *testing.T) {
	for _, status := range []string{"complete", "stale", "unavailable"} {
		t.Run(status, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handleAccountCurrencies(currencyCoverage{status})(recorder, httptest.NewRequest("GET", "/api/accounts/currencies", nil))
			var got exchange.Coverage
			if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != 200 || got.Status != status || got.Source != "synthetic" || got.Currencies[0].Code != "BRL" {
				t.Fatalf("%+v", got)
			}
			if status != "unavailable" && got.Currencies[1].Code != "ARS" {
				t.Fatalf("%+v", got)
			}
		})
	}
}
