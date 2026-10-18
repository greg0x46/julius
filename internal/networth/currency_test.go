package networth_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/networth"
	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/timeline"
	"github.com/greg0x46/julius/internal/transactions"
	"github.com/greg0x46/julius/internal/valuation"
)

// stubRates is a provider with fixed answers: "USD/BRL" applies to every day,
// "USD/BRL@2024-06-12" only to that one. Anything else is a provider failure.
type stubRates map[string]string

func (stubRates) Name() string { return "stub" }

func (s stubRates) Rate(_ context.Context, base, quote string, day time.Time) (exchange.ProviderRate, error) {
	pair := base + "/" + quote
	value, ok := s[pair+"@"+day.Format("2006-01-02")]
	if !ok {
		value, ok = s[pair]
	}
	if !ok {
		return exchange.ProviderRate{}, errors.New("no rate for " + pair)
	}
	return exchange.ProviderRate{Value: decimal.RequireFromString(value), EffectiveOn: day}, nil
}

func (f *fixture) fx(rates map[string]string) *exchange.Service {
	return exchange.New(f.conn, stubRates(rates))
}

func (f *fixture) setCurrency(table, id, currency string) {
	f.t.Helper()
	f.exec(fmt.Sprintf(`UPDATE %s SET currency_code = ? WHERE id = ?`, table), currency, id)
}

func TestMixedCurrencyCashInvestmentsAndLiabilities(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	at := time.Now()
	valued := func() *valuation.Valuation {
		return valuation.New(f.fx(map[string]string{"USD/BRL": "5"}), at)
	}

	f.addAccount("BANK", "2000")
	usd := f.addAccount("BANK", "100")
	f.setCurrency("financial_accounts", usd, "USD")
	b, err := networth.ComputeValued(ctx, f.conn, valued())
	if err != nil {
		t.Fatal(err)
	}
	assertDecimalEqual(t, "BRL 2,000 + USD 100 at 5.00", b.CashBalance, "2500")

	// The timeline's cash anchor starts from the same native balances at the
	// same day's rate, so the two views agree.
	day := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
	series, err := timeline.BuildSeries(ctx, f.conn, timeline.BuildParams{From: day, To: day, ReferenceDate: day})
	if err != nil {
		t.Fatal(err)
	}
	assertDecimalEqual(t, "timeline anchor", series.Points[0].Balance, "2500")

	investment := f.addInvestment("20")
	f.setCurrency("financial_investments", investment, "USD")
	card := f.addAccount("CREDIT", "9999")
	f.setCurrency("financial_accounts", card, "USD")
	purchase := f.addCardTransaction(card, at, "-10", "DEBIT")
	f.setCurrency("financial_transactions", purchase, "USD")
	f.addBill(card, "previous", at.AddDate(0, 0, -1), "")
	f.addBill(card, "current", at.AddDate(0, 0, 10), purchase)
	f.addPayable(payables.KindDebt, "30", "30")

	b, err = networth.ComputeValued(ctx, f.conn, valued())
	if err != nil {
		t.Fatal(err)
	}
	assertDecimalEqual(t, "USD holding", b.InvestmentBalance, "100")
	assertDecimalEqual(t, "USD card debt", b.CreditCardBalance, "50")
	assertDecimalEqual(t, "BRL payable", b.PayablesDebt, "30")
	assertDecimalEqual(t, "net worth", b.NetWorth, "2520")

	f.setIgnored(purchase)
	b, err = networth.ComputeValued(ctx, f.conn, valued())
	if err != nil {
		t.Fatal(err)
	}
	assertDecimalEqual(t, "ignored foreign purchase", b.CreditCardBalance, "0")

	native, err := transactions.CashOnHandIn(ctx, f.conn, []string{usd}, "USD")
	if err != nil {
		t.Fatal(err)
	}
	assertDecimalEqual(t, "native USD balance is untouched", native, "100")
}

func TestMissingRatesNeverDropInvestmentsOrCardDebt(t *testing.T) {
	for _, kind := range []string{"investment", "card"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			now := time.Now()
			f.addAccount("BANK", "2000")
			if kind == "investment" {
				id := f.addInvestment("100")
				f.setCurrency("financial_investments", id, "USD")
			} else {
				card := f.addAccount("CREDIT", "0")
				f.setCurrency("financial_accounts", card, "USD")
				tx := f.addCardTransaction(card, now, "-100", "DEBIT")
				f.addBill(card, "previous", now.AddDate(0, 0, -1), "")
				f.addBill(card, "current", now.AddDate(0, 0, 10), tx)
			}
			_, err := networth.SnapshotValued(context.Background(), f.conn, valuation.New(f.fx(nil), now))
			if !errors.Is(err, valuation.ErrMissingRate) {
				t.Fatalf("missing %s rate: %v", kind, err)
			}
			var count int
			if err := f.conn.QueryRow(`SELECT COUNT(*) FROM net_worth_snapshots`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("incomplete live snapshot persisted: %d, %v", count, err)
			}
		})
	}
}

func TestSnapshotRecordsTheRatesItUsed(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	bank := f.addAccount("BANK", "100")
	f.setCurrency("financial_accounts", bank, "USD")
	now := time.Now()

	snapshot, err := networth.SnapshotValued(ctx, f.conn, valuation.New(f.fx(map[string]string{"USD/BRL": "5"}), now))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ValuationVersion != networth.ValuationVersion || snapshot.ValuationStatus != networth.ValuationComplete ||
		snapshot.CurrencyCode != "BRL" || snapshot.Valuation == nil || snapshot.Valuation.RatePolicy != valuation.RatePolicy {
		t.Fatalf("metadata: %+v", snapshot)
	}
	rate, ok := snapshot.Valuation.Rates["USD"]
	if !ok || !rate.Value.Equal(decimal.NewFromInt(5)) || rate.Source != "stub" {
		t.Fatalf("recorded rates: %+v", snapshot.Valuation.Rates)
	}
	assertDecimalEqual(t, "net worth", snapshot.Breakdown.NetWorth, "500")
}

func TestLegacySnapshotsKeepAnExplicitUnqualifiedStatus(t *testing.T) {
	f := newFixture(t)
	f.exec(`INSERT INTO net_worth_snapshots(id,captured_at,total_assets,total_liabilities,net_worth,cash_balance,investment_balance,credit_card_balance,payables_debt,is_backfilled,created_at,updated_at)
		VALUES ('old','2020-01-02','100','0','100','100','0','0','0',FALSE,'2020-01-02T00:00:00Z','2020-01-02T00:00:00Z')`)
	day := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
	row, ok := snapshotFor(t, f, day)
	if !ok {
		t.Fatal("legacy row missing")
	}
	if row.ValuationVersion != 0 || row.ValuationStatus != networth.ValuationLegacy || row.Valuation != nil {
		t.Fatalf("legacy row was relabeled: %+v", row)
	}
	assertDecimalEqual(t, "legacy amounts are untouched", row.Breakdown.NetWorth, "100")
}

func TestBackfillValuesEachPastDayWithItsOwnRate(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	today := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	usd := f.addAccount("BANK", "100")
	f.setCurrency("financial_accounts", usd, "USD")
	f.addCardTransaction(usd, today.AddDate(0, 0, -4), "0.01", "CREDIT") // opens the window
	rates := f.fx(map[string]string{
		"USD/BRL@2024-06-13": "4",
		"USD/BRL@2024-06-12": "6",
		"USD/BRL":            "5",
	})
	if err := networth.BackfillWith(ctx, f.conn, today, rates); err != nil {
		t.Fatal(err)
	}
	for day, want := range map[int]string{-1: "500", -2: "400", -3: "600"} {
		row, ok := snapshotFor(t, f, today.AddDate(0, 0, day))
		if !ok {
			t.Fatalf("no snapshot for %d", day)
		}
		if row.ValuationStatus != networth.ValuationReconstructed || !row.IsBackfilled {
			t.Fatalf("day %d status = %q", day, row.ValuationStatus)
		}
		assertDecimalEqual(t, fmt.Sprintf("day %d cash", day), row.Breakdown.CashBalance, want)
		if !row.Valuation.ValuedAt.Equal(today.AddDate(0, 0, day)) || row.Valuation.Rates["USD"].Value.IsZero() {
			t.Fatalf("day %d valuation: %+v", day, row.Valuation)
		}
	}
}

func TestBackfillRetriesDaysWhoseRateWasMissing(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	today := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	usd := f.addAccount("BANK", "100")
	f.setCurrency("financial_accounts", usd, "USD")
	f.addCardTransaction(usd, today.AddDate(0, 0, -2), "1", "CREDIT")

	if err := networth.BackfillWith(ctx, f.conn, today, f.fx(nil)); err != nil {
		t.Fatalf("a missing rate is not an error: %v", err)
	}
	if _, ok := snapshotFor(t, f, today.AddDate(0, 0, -1)); ok {
		t.Fatal("a day without a rate was stored, so it can never be retried")
	}
	if err := networth.BackfillWith(ctx, f.conn, today, f.fx(map[string]string{"USD/BRL": "5"})); err != nil {
		t.Fatal(err)
	}
	row, ok := snapshotFor(t, f, today.AddDate(0, 0, -1))
	if !ok {
		t.Fatal("day was not retried once the rate was available")
	}
	assertDecimalEqual(t, "cash", row.Breakdown.CashBalance, "500")
}

// outageRates is a resolver whose provider is down.
type outageRates struct{ calls int }

func (o *outageRates) On(context.Context, string, string, time.Time) (exchange.Rate, error) {
	o.calls++
	return exchange.Rate{}, fmt.Errorf("%w: %w", exchange.ErrUnavailable, exchange.ErrProviderFailure)
}

func TestBackfillStopsAtTheFirstProviderFailure(t *testing.T) {
	f := newFixture(t)
	today := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	usd := f.addAccount("BANK", "100")
	f.setCurrency("financial_accounts", usd, "USD")
	f.addCardTransaction(usd, today.AddDate(0, 0, -30), "1", "CREDIT")

	rates := &outageRates{}
	if err := networth.BackfillWith(context.Background(), f.conn, today, rates); err != nil {
		t.Fatalf("a provider outage is not an error: %v", err)
	}
	if rates.calls != 1 {
		t.Fatalf("rate lookups during an outage = %d, want 1 for the 30 missing days", rates.calls)
	}
	if _, ok := snapshotFor(t, f, today.AddDate(0, 0, -1)); ok {
		t.Fatal("a day was stored without a rate")
	}
}
