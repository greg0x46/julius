package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/greg0x46/julius/internal/db/dbtest"
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

type fixedCurrencyCoverage struct{ coverage exchange.Coverage }

func (p fixedCurrencyCoverage) Currencies(context.Context) exchange.Coverage { return p.coverage }

func TestAccountCurrencyOptionsMatchWriteValidation(t *testing.T) {
	provider := fixedCurrencyCoverage{exchange.Coverage{
		Currencies: []exchange.Currency{
			{Code: "CNH"}, {Code: "BRL"}, {Code: "CMD"}, {Code: "GGP"},
			{Code: "ARS", StartDate: "1998-07-07", EndDate: "2026-10-14"},
			{Code: "IMP"}, {Code: "XXX"}, {Code: "usd"},
		},
		Status: "complete", Source: "synthetic",
	}}
	originalCurrencies := append([]exchange.Currency(nil), provider.coverage.Currencies...)
	w := httptest.NewRecorder()
	handleAccountCurrencies(provider)(w, httptest.NewRequest("GET", "/api/accounts/currencies", nil))
	var got exchange.Coverage
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(got.Currencies) != 3 || got.Currencies[0].Code != "CNH" || got.Currencies[1].Code != "BRL" || got.Currencies[2].Code != "ARS" {
		t.Fatalf("selectable currencies: %d %+v", w.Code, got)
	}
	if got.Status != "complete" || got.Source != "synthetic" || got.Currencies[2] != provider.coverage.Currencies[4] {
		t.Fatalf("coverage metadata changed: %+v", got)
	}
	if !reflect.DeepEqual(provider.coverage.Currencies, originalCurrencies) {
		t.Fatalf("raw provider coverage changed: %+v", provider.coverage)
	}
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		for _, c := range provider.coverage.Currencies {
			t.Run(c.Code, func(t *testing.T) {
				body, err := json.Marshal(manualAccountRequest{Name: "Synthetic " + c.Code, CurrencyCode: c.Code, OpeningBalance: "100"})
				if err != nil {
					t.Fatal(err)
				}
				w := httptest.NewRecorder()
				handleWriteManualAccount(conn, true)(w, httptest.NewRequest("POST", "/api/accounts", bytes.NewReader(body)))
				want := 422
				for _, option := range got.Currencies {
					if option.Code == c.Code {
						want = 201
					}
				}
				if w.Code != want {
					t.Fatalf("write %s: %d %s, want %d", c.Code, w.Code, w.Body.String(), want)
				}
			})
		}
	})
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
