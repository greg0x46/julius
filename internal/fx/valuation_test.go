package fx_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/fx"
	"github.com/shopspring/decimal"
)

func TestDatedRatesAreExplicitAndStableWithinAValuation(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "fx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx := t.Context()
	at := time.Date(2020, 1, 2, 12, 0, 0, 0, time.UTC)
	store := func(value string, date time.Time) {
		t.Helper()
		if err := fx.StoreRate(ctx, conn, "USD", fx.Rate{Value: decimal.RequireFromString(value), ObservedAt: date, Source: "test"}); err != nil {
			t.Fatal(err)
		}
	}
	store("4", at.Add(-time.Hour))
	store("5", at)
	store("99", at.Add(time.Hour))
	valuation := fx.New(at)
	value, err := valuation.Convert(ctx, conn, decimal.NewFromInt(100), "USD")
	if err != nil || value.String() != "500" {
		t.Fatalf("value: %s, %v", value, err)
	}
	store("6", at)
	value, err = valuation.Convert(ctx, conn, decimal.NewFromInt(100), "USD")
	if err != nil || value.String() != "500" {
		t.Fatalf("rate changed mid-valuation: %s, %v", value, err)
	}
	if _, err := fx.New(at.Add(-2*time.Hour)).Convert(ctx, conn, decimal.NewFromInt(100), "USD"); !errors.Is(err, fx.ErrMissingRate) {
		t.Fatalf("future rate used historically: %v", err)
	}
	for _, rate := range []fx.Rate{
		{Value: decimal.Zero, ObservedAt: at, Source: "test"},
		{Value: decimal.NewFromInt(-1), ObservedAt: at, Source: "test"},
		{Value: decimal.NewFromInt(5), Source: "test"},
		{Value: decimal.NewFromInt(5), ObservedAt: at},
	} {
		if err := fx.StoreRate(ctx, conn, "USD", rate); err == nil {
			t.Fatalf("invalid rate accepted: %+v", rate)
		}
	}
}
