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

func TestTransactionReferenceTotalsPreserveNativeAndExcludeForeign(t *testing.T) {
	native := []currencyTotalsDTO{
		{CurrencyCode: "USD", Inflow: "100", Outflow: "5", Balance: "95"},
		{CurrencyCode: "BRL", Inflow: "150", Outflow: "50", Balance: "100"},
	}
	valuation := valuationDTO{CurrencyCode: "USD", FX: exchange.Rate{Value: decimal.RequireFromString("0.2")}}
	got := valuation.transactionTotals(native)
	if got.CurrencyCode != "USD" || !decimal.RequireFromString(got.Inflow).Equal(decimal.NewFromInt(30)) || !decimal.RequireFromString(got.Outflow).Equal(decimal.NewFromInt(10)) || !decimal.RequireFromString(got.Balance).Equal(decimal.NewFromInt(20)) {
		t.Fatalf("reference totals = %+v", got)
	}
	if native[0].Balance != "95" || native[1].Balance != "100" {
		t.Fatalf("native totals changed: %+v", native)
	}
	if got := valuation.transactionTotals(native[:1]); !decimal.RequireFromString(got.Balance).IsZero() || !decimal.RequireFromString(got.Inflow).IsZero() || !decimal.RequireFromString(got.Outflow).IsZero() {
		t.Fatalf("foreign totals entered BRL-base report: %+v", got)
	}
}
