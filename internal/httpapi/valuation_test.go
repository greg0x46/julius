package httpapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/reporting"
	"github.com/shopspring/decimal"
)

type unavailableFX struct{}

func (unavailableFX) Name() string { return "synthetic" }
func (unavailableFX) Rate(context.Context, string, string, time.Time) (exchange.ProviderRate, error) {
	return exchange.ProviderRate{}, errors.New("offline")
}

func TestReferenceValuationRequiresFX(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fx := exchange.New(conn, unavailableFX{})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/timeline", nil)
	_, ok := valuationForCurrency(recorder, request, fx, "USD", time.Now().UTC())
	if ok || recorder.Code != 503 || !strings.Contains(recorder.Body.String(), "exchange-unavailable") || !strings.Contains(recorder.Body.String(), "BRL para USD") {
		t.Fatalf("missing rate response: %d %s", recorder.Code, recorder.Body.String())
	}
	valuation, ok := valuationForCurrency(httptest.NewRecorder(), request, fx, "BRL", time.Now().UTC())
	if !ok || valuation.FX.Source != exchange.SourceIdentity {
		t.Fatalf("BRL regression = %+v", valuation)
	}
}

func TestReferenceSummaryConvertsConsolidatedTotalsOnly(t *testing.T) {
	summary := reporting.Summary{CurrencyCode: "BRL", Status: "complete", Totals: &reporting.Totals{CurrencyCode: "BRL", Inflow: "150", Outflow: "50", Balance: "100"}}
	valuation := valuationDTO{CurrencyCode: "USD", FX: exchange.Rate{Value: decimal.RequireFromString("0.2")}}
	valuation.summary(&summary)
	totals := summary.Totals
	if summary.CurrencyCode != "USD" || totals.CurrencyCode != "USD" || !decimal.RequireFromString(totals.Inflow).Equal(decimal.NewFromInt(30)) || !decimal.RequireFromString(totals.Outflow).Equal(decimal.NewFromInt(10)) || !decimal.RequireFromString(totals.Balance).Equal(decimal.NewFromInt(20)) {
		t.Fatalf("reference summary = %+v %+v", summary, totals)
	}
	unavailable := reporting.Summary{CurrencyCode: "BRL", Status: "unavailable"}
	valuation.summary(&unavailable)
	if unavailable.Totals != nil || unavailable.CurrencyCode != "USD" {
		t.Fatalf("unavailable summary gained totals: %+v", unavailable)
	}
}
