package networth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/fx"
	"github.com/greg0x46/julius/internal/networth"
	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/timeline"
	"github.com/greg0x46/julius/internal/transactions"
)

func (f *fixture) rate(currency, value string, at time.Time) {
	f.t.Helper()
	if err := fx.StoreRate(context.Background(), f.conn, currency, fx.Rate{Value: mustDecimal(f.t, value), ObservedAt: at, Source: "test"}); err != nil {
		f.t.Fatal(err)
	}
}

func TestMixedCurrencyCashInvestmentsAndLiabilities(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	at := time.Now()
	f.addAccount("BANK", "2000")
	usd := f.addAccount("BANK", "100")
	f.exec(`UPDATE financial_accounts SET currency_code='USD' WHERE id=?`, usd)
	if _, err := networth.Compute(ctx, f.conn); !errors.Is(err, fx.ErrMissingRate) {
		t.Fatalf("missing cash rate: %v", err)
	}
	f.rate("USD", "5", at.Add(-time.Hour))
	f.rate("USD", "99", at.Add(time.Hour))
	b, err := networth.ComputeValued(ctx, f.conn, fx.New(at))
	if err != nil {
		t.Fatal(err)
	}
	assertDecimalEqual(t, "mixed cash", b.CashBalance, "2500")
	cash, err := transactions.CashOnHand(ctx, f.conn, []string{usd})
	if err != nil {
		t.Fatal(err)
	}
	assertDecimalEqual(t, "scoped foreign cash", cash, "500")
	day := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
	series, err := timeline.BuildSeries(ctx, f.conn, timeline.BuildParams{From: day, To: day, ReferenceDate: day})
	if err != nil {
		t.Fatal(err)
	}
	assertDecimalEqual(t, "timeline anchor", series.Points[0].Balance, "2500")

	foreignInvestment := f.addInvestment("20")
	f.exec(`UPDATE financial_investments SET currency_code='USD' WHERE id=?`, foreignInvestment)
	card := f.addAccount("CREDIT", "9999")
	f.exec(`UPDATE financial_accounts SET currency_code='USD' WHERE id=?`, card)
	purchase := f.addCardTransaction(card, at, "-10", "DEBIT")
	f.exec(`UPDATE financial_transactions SET currency_code='USD' WHERE id=?`, purchase)
	f.addBill(card, "previous", at.AddDate(0, 0, -1), "")
	f.addBill(card, "current", at.AddDate(0, 0, 10), purchase)
	f.addPayable(payables.KindDebt, "30", "30")
	b, err = networth.ComputeValued(ctx, f.conn, fx.New(at))
	if err != nil {
		t.Fatal(err)
	}
	assertDecimalEqual(t, "investments", b.InvestmentBalance, "100")
	assertDecimalEqual(t, "foreign card debt", b.CreditCardBalance, "50")
	assertDecimalEqual(t, "BRL payable", b.PayablesDebt, "30")
	assertDecimalEqual(t, "net worth", b.NetWorth, "2520")
	f.setIgnored(purchase)
	b, err = networth.Compute(ctx, f.conn)
	if err != nil {
		t.Fatal(err)
	}
	assertDecimalEqual(t, "ignored foreign purchase", b.CreditCardBalance, "0")
}

func TestMissingRatesNeverDropInvestmentsOrCardDebt(t *testing.T) {
	for _, kind := range []string{"investment", "card"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			now := time.Now()
			f.addAccount("BANK", "2000")
			if kind == "investment" {
				id := f.addInvestment("100")
				f.exec(`UPDATE financial_investments SET currency_code='USD' WHERE id=?`, id)
			} else {
				card := f.addAccount("CREDIT", "0")
				f.exec(`UPDATE financial_accounts SET currency_code='USD' WHERE id=?`, card)
				tx := f.addCardTransaction(card, now, "-100", "DEBIT")
				f.addBill(card, "previous", now.AddDate(0, 0, -1), "")
				f.addBill(card, "current", now.AddDate(0, 0, 10), tx)
			}
			if _, err := networth.Snapshot(context.Background(), f.conn); !errors.Is(err, fx.ErrMissingRate) {
				t.Fatalf("missing %s rate: %v", kind, err)
			}
			var count int
			if err := f.conn.QueryRow(`SELECT COUNT(*) FROM net_worth_snapshots`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("incomplete live snapshot persisted: %d, %v", count, err)
			}
		})
	}
}

func TestSnapshotPersistsExactRatesWithoutRelabelingLegacyRows(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	bank := f.addAccount("BANK", "100")
	f.exec(`UPDATE financial_accounts SET currency_code='USD' WHERE id=?`, bank)
	at := time.Now().Add(-time.Hour)
	f.rate("USD", "5", at)
	snapshot, err := networth.Snapshot(ctx, f.conn)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ValuationVersion != 1 || snapshot.ValuationStatus != networth.ValuationComplete || snapshot.Valuation.ReferenceCurrency != "BRL" || snapshot.Valuation.RatePolicy != fx.RatePolicy || snapshot.Valuation.ValuedAt.IsZero() {
		t.Fatalf("metadata: %+v", snapshot)
	}
	f.rate("USD", "6", at)
	stored, ok := snapshotFor(t, f, snapshot.CapturedAt)
	if !ok || stored.Valuation.Rates["USD"].Value.String() != "5" || !stored.Valuation.Rates["USD"].ObservedAt.Equal(at) || stored.Valuation.Rates["USD"].Source != "test" {
		t.Fatalf("stored rate changed: %+v", stored)
	}
	assertDecimalEqual(t, "stored cash", stored.Breakdown.CashBalance, "500")
	f.exec(`INSERT INTO net_worth_snapshots (id,captured_at,total_assets,total_liabilities,net_worth,cash_balance,investment_balance,credit_card_balance,payables_debt,created_at,updated_at)
	 VALUES ('legacy','2020-01-01','2100','0','2100','2100','0','0','0',?,?)`, db.FormatTime(at), db.FormatTime(at))
	legacy, ok := snapshotFor(t, f, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	if !ok || legacy.ValuationVersion != 0 || legacy.ValuationStatus != "legacy_unknown" || legacy.Valuation != nil {
		t.Fatalf("legacy relabeled: %+v", legacy)
	}
	assertDecimalEqual(t, "legacy amount", legacy.Breakdown.CashBalance, "2100")
}

func TestBackfillUsesHistoricalRatesAndMarksMissingHistory(t *testing.T) {
	for _, history := range []bool{true, false} {
		t.Run(map[bool]string{true: "available", false: "missing"}[history], func(t *testing.T) {
			f := newFixture(t)
			ctx := context.Background()
			day := func(n int) time.Time { return time.Date(2020, 1, n, 12, 0, 0, 0, time.UTC) }
			bank := f.addAccount("BANK", "100")
			f.exec(`UPDATE financial_accounts SET currency_code='USD' WHERE id=?`, bank)
			first := f.addCardTransaction(bank, day(1), "80", "CREDIT")
			second := f.addCardTransaction(bank, day(2), "20", "CREDIT")
			f.exec(`UPDATE financial_transactions SET currency_code='USD' WHERE id IN (?,?)`, first, second)
			if history {
				f.rate("USD", "4", day(1))
			}
			f.rate("USD", "5", day(2))
			f.rate("USD", "99", day(3))
			if err := networth.Backfill(ctx, f.conn, day(3)); err != nil {
				t.Fatal(err)
			}
			one, _ := snapshotFor(t, f, day(1))
			two, _ := snapshotFor(t, f, day(2))
			if two.ValuationStatus != networth.ValuationReconstructed || !two.IsBackfilled || two.Valuation.Rates["USD"].Value.String() != "5" {
				t.Fatalf("day 2: %+v", two)
			}
			assertDecimalEqual(t, "day 2 cash", two.Breakdown.CashBalance, "500")
			if history {
				assertDecimalEqual(t, "native reconstruction then historical FX", one.Breakdown.CashBalance, "320")
				if one.Valuation.Rates["USD"].Value.String() != "4" {
					t.Fatal(one.Valuation)
				}
			} else if one.ValuationStatus != networth.ValuationUnavailable {
				t.Fatalf("missing history labeled usable: %+v", one)
			}
			f.rate("USD", "7", day(2))
			if err := networth.Backfill(ctx, f.conn, day(3)); err != nil {
				t.Fatal(err)
			}
			two, _ = snapshotFor(t, f, day(2))
			assertDecimalEqual(t, "backfill does not rewrite observations", two.Breakdown.CashBalance, "500")
		})
	}
}
