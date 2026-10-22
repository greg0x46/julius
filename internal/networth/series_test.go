package networth_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/networth"
	"github.com/greg0x46/julius/internal/transactions"
)

var today = time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)

func daysAgo(n int) time.Time { return today.AddDate(0, 0, -n) }

// TestSeriesReconstructsCashWalkingEligibleTransactionsBackward proves the
// core cash reconstruction: today's balance minus every real transaction's
// effect, walked backward day by day — including a transaction the user
// marked "ignored", since ignored only means "excluded from income/expense
// totals" and the money still moved through the real, synced account
// balance that today's figure starts from.
func TestSeriesReconstructsCashWalkingEligibleTransactionsBackward(t *testing.T) {
	f := newFixture(t)
	accountID := f.addAccount("", "1000.00")

	f.addTransaction(accountID, daysAgo(2), "-100.00", "DEBIT") // balance was 100 higher before this happened
	f.addTransaction(accountID, daysAgo(1), "50.00", "CREDIT")  // balance was 50 lower before this happened
	ignored := f.addTransaction(accountID, daysAgo(1), "9999.00", "DEBIT")
	f.setIgnored(ignored) // excluded from totals, but still reversed here — it really happened

	points := f.series(today, nil)
	oneDayAgo, ok := pointOn(t, points, daysAgo(1))
	if !ok {
		t.Fatal("no point for yesterday")
	}
	assertDecimalEqual(t, "yesterday cash", oneDayAgo.Breakdown.CashBalance, "1000")
	twoDaysAgo, ok := pointOn(t, points, daysAgo(2))
	if !ok {
		t.Fatal("no point for two days ago")
	}
	assertDecimalEqual(t, "two days ago cash", twoDaysAgo.Breakdown.CashBalance, "10949")
	assertDecimalEqual(t, "two days ago total", twoDaysAgo.Breakdown.Total, "10949")
	last := points[len(points)-1]
	if !last.Day.Equal(today) {
		t.Fatalf("the series ends on %s, want today", last.Day)
	}
}

// TestSeriesEndsWithTheLiveFigure pins that the last point is Compute, so the
// history and today's number cannot disagree.
func TestSeriesEndsWithTheLiveFigure(t *testing.T) {
	f := newFixture(t)
	accountID := f.addAccount("", "1000.00")
	f.addTransaction(accountID, daysAgo(3), "10.00", "CREDIT")
	f.addInvestment("250")

	points := f.series(time.Now(), nil)
	live, err := networth.Compute(context.Background(), f.conn)
	if err != nil {
		t.Fatal(err)
	}
	last := points[len(points)-1]
	if !last.Breakdown.Total.Equal(live.Total) || !last.Breakdown.InvestmentBalance.Equal(live.InvestmentBalance) {
		t.Fatalf("last point %+v differs from Compute %+v", last.Breakdown, live)
	}
}

func TestSeriesReversesForeignTransactionsInTheirOwnCurrency(t *testing.T) {
	f := newFixture(t)
	brl := f.addAccount("", "200.00")
	usd := f.addAccount("", "100.00")
	f.setCurrency("financial_accounts", usd, "USD")
	f.addTransaction(brl, daysAgo(4), "10.00", "CREDIT") // opens the window before the USD movement
	f.addTransaction(usd, daysAgo(2), "40.00", "CREDIT")

	point, ok := pointOn(t, f.series(today, map[string]string{"USD/BRL": "5"}), daysAgo(3))
	if !ok {
		t.Fatal("no point for three days ago")
	}
	// 200 BRL + (100 - 40) USD * 5
	assertDecimalEqual(t, "cash", point.Breakdown.CashBalance, "500")
}

// TestSeriesMarksDaysBeforeAnIrreversibleForeignMovementUnavailable pins that
// a USD account's transaction with no account-currency amount cannot be
// reversed, so the days before it carry no figures instead of a wrong or zero one.
func TestSeriesMarksDaysBeforeAnIrreversibleForeignMovementUnavailable(t *testing.T) {
	f := newFixture(t)
	brl := f.addAccount("", "200.00")
	usd := f.addAccount("", "100.00")
	f.setCurrency("financial_accounts", usd, "USD")
	f.addTransaction(brl, daysAgo(4), "10.00", "CREDIT")
	id := f.addTransaction(usd, daysAgo(2), "100.00", "CREDIT")
	f.exec(`UPDATE financial_transactions SET amount_in_account_currency = NULL WHERE id = ?`, id)

	points := f.series(today, map[string]string{"USD/BRL": "5"})
	before, _ := pointOn(t, points, daysAgo(3))
	if before.Status != networth.ValuationUnavailable {
		t.Fatalf("status = %q, want unavailable", before.Status)
	}
	assertDecimalEqual(t, "an unavailable day carries no figures", before.Breakdown.Total, "0")
	if after, ok := pointOn(t, points, daysAgo(2)); !ok || after.Status != networth.ValuationComplete {
		t.Fatalf("the day after the movement is still reconstructable: %+v %v", after, ok)
	}
}

// TestSeriesStartsAtTheEarliestKnownMovement proves it never invents a day it
// has no coverage for.
func TestSeriesStartsAtTheEarliestKnownMovement(t *testing.T) {
	f := newFixture(t)
	accountID := f.addAccount("", "500.00")
	f.addTransaction(accountID, daysAgo(3), "10.00", "CREDIT")

	points := f.series(today, nil)
	if _, ok := pointOn(t, points, daysAgo(4)); ok {
		t.Error("found a point before the earliest known movement")
	}
	if _, ok := pointOn(t, points, daysAgo(3)); !ok {
		t.Error("no point for the earliest movement's own day")
	}
	if len(points) != 4 {
		t.Errorf("points = %d, want 3 past days plus today", len(points))
	}
}

func TestSeriesGoesBackFurtherThanNinetyDays(t *testing.T) {
	f := newFixture(t)
	accountID := f.addAccount("", "1000.00")
	f.addTransaction(accountID, daysAgo(200), "10.00", "CREDIT")

	if _, ok := pointOn(t, f.series(today, nil), daysAgo(200)); !ok {
		t.Error("no point 200 days back, want the full history")
	}
}

// TestSeriesWithNoHistoryIsJustToday covers an account connected too recently
// to reconstruct anything: no flat series is invented.
func TestSeriesWithNoHistoryIsJustToday(t *testing.T) {
	f := newFixture(t)
	f.addAccount("", "1000.00")

	points := f.series(today, nil)
	if len(points) != 1 || !points[0].Day.Equal(today) {
		t.Fatalf("points = %+v, want today only", points)
	}
	assertDecimalEqual(t, "today", points[0].Breakdown.Total, "1000")
}

// TestSeriesReversesTransferCategorizedTransactions is the guard rail for the
// transfer rule's blast radius. Categorizing a transaction as a transfer keeps
// it out of income/expense totals, but it must never reach balance
// reconstruction: the money really did leave this account, so today's synced
// balance already reflects it and every day before it has to reverse it out.
func TestSeriesReversesTransferCategorizedTransactions(t *testing.T) {
	f := newFixture(t)
	accountID := f.addAccount("", "1000.00")
	f.addTransaction(accountID, daysAgo(2), "-1.00", "DEBIT") // anchors the older day
	transfer := f.addTransaction(accountID, daysAgo(1), "-300.00", "DEBIT")
	f.setCategory(transfer, categoryTransferencia)

	point, ok := pointOn(t, f.series(today, nil), daysAgo(2))
	if !ok {
		t.Fatal("no point for two days ago")
	}
	assertDecimalEqual(t, "cash before the transfer left", point.Breakdown.CashBalance, "1300")
}

// TestSeriesIgnoresManualTransactionsOnSyncedAccounts: a manual entry on a
// synced account never changed the provider balance today's figure is built
// from, so it must not shift any past day.
func TestSeriesIgnoresManualTransactionsOnSyncedAccounts(t *testing.T) {
	f := newFixture(t)
	accountID := f.addAccount("", "1000.00")
	f.addTransaction(accountID, daysAgo(2), "-1.00", "DEBIT")
	if _, err := transactions.CreateManual(context.Background(), f.conn, transactions.ManualInput{
		AccountID: accountID, Description: "Compra em dinheiro",
		Amount: decimal.RequireFromString("-9999.00"), OccurredAt: daysAgo(1),
	}); err != nil {
		t.Fatalf("CreateManual: %v", err)
	}

	point, ok := pointOn(t, f.series(today, nil), daysAgo(2))
	if !ok {
		t.Fatal("no point for two days ago")
	}
	assertDecimalEqual(t, "cash", point.Breakdown.CashBalance, "1000")
}

func TestSeriesManualAccountMovementsAndAdjustments(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	first := db.FormatTime(now.AddDate(0, 0, -2))
	f.exec(`INSERT INTO financial_accounts(id,origin,name,account_type,currency_code,opening_balance,created_at,updated_at) VALUES('manual','manual','Cash','BANK','BRL','100',?,?)`, first, first)
	if _, err := transactions.CreateManual(context.Background(), f.conn, transactions.ManualInput{AccountID: "manual", Description: "Deposit", Amount: decimal.RequireFromString("10"), OccurredAt: now.AddDate(0, 0, -2)}); err != nil {
		t.Fatal(err)
	}
	f.exec(`INSERT INTO account_balance_adjustments(id,account_id,amount,description,created_at) VALUES('adjust','manual','5','Correction',?)`, db.FormatTime(now.AddDate(0, 0, -1)))
	foreign := f.addAccount("BANK", "10000")
	f.setCurrency("financial_accounts", foreign, "USD")
	f.addTransaction(foreign, now.AddDate(0, 0, -1), "1000", "CREDIT")

	points := f.series(now, map[string]string{"USD/BRL": "2"})
	// The USD account held 9000 before yesterday's 1000 credit: 18000 BRL at 2.
	if p, ok := pointOn(t, points, now.AddDate(0, 0, -2)); !ok || !p.Breakdown.CashBalance.Equal(decimal.RequireFromString("18110")) {
		t.Fatalf("first day: %+v %v", p, ok)
	}
	if p, ok := pointOn(t, points, now.AddDate(0, 0, -1)); !ok || !p.Breakdown.CashBalance.Equal(decimal.RequireFromString("20115")) {
		t.Fatalf("adjustment day: %+v %v", p, ok)
	}
}

func TestSeriesValuesEachPastDayWithItsOwnRate(t *testing.T) {
	f := newFixture(t)
	usd := f.addAccount("BANK", "100")
	f.setCurrency("financial_accounts", usd, "USD")
	f.addTransaction(usd, daysAgo(4), "0.01", "CREDIT") // opens the window
	points := f.series(today, map[string]string{
		"USD/BRL@2024-06-13": "4",
		"USD/BRL@2024-06-12": "6",
		"USD/BRL":            "5",
	})
	for day, want := range map[int]string{1: "500", 2: "400", 3: "600"} {
		p, ok := pointOn(t, points, daysAgo(day))
		if !ok {
			t.Fatalf("no point for %d days ago", day)
		}
		if p.Status != networth.ValuationComplete {
			t.Fatalf("%d days ago: status = %q", day, p.Status)
		}
		assertDecimalEqual(t, fmt.Sprintf("%d days ago", day), p.Breakdown.CashBalance, want)
		if !p.Valuation.ValuedAt.Equal(daysAgo(day)) || p.Valuation.Rates["USD"].Value.IsZero() {
			t.Fatalf("%d days ago valuation: %+v", day, p.Valuation)
		}
	}
}

// TestSeriesLeavesADayWithoutARateUnavailable: a day whose rate is missing has
// no figures, never a 1:1 guess, and the days that do have a rate stay.
func TestSeriesLeavesADayWithoutARateUnavailable(t *testing.T) {
	f := newFixture(t)
	usd := f.addAccount("BANK", "100")
	f.setCurrency("financial_accounts", usd, "USD")
	f.addTransaction(usd, daysAgo(3), "1", "CREDIT")

	points := f.series(today, map[string]string{"USD/BRL@2024-06-14": "5", "USD/BRL@2024-06-15": "5"})
	if p, _ := pointOn(t, points, daysAgo(1)); p.Status != networth.ValuationComplete {
		t.Fatalf("yesterday has a rate: %q", p.Status)
	}
	if p, _ := pointOn(t, points, daysAgo(2)); p.Status != networth.ValuationUnavailable {
		t.Fatalf("a day without a rate: %q", p.Status)
	}
}

// pastOutageRates values today but has a provider that is down for any other day.
type pastOutageRates struct{ calls int }

func (o *pastOutageRates) On(_ context.Context, _, _ string, day time.Time) (exchange.Rate, error) {
	if day.Equal(today) {
		return exchange.Rate{Value: decimal.NewFromInt(5)}, nil
	}
	o.calls++
	return exchange.Rate{}, fmt.Errorf("%w: %w", exchange.ErrUnavailable, exchange.ErrProviderFailure)
}

// TestSeriesStopsAskingAFailedProvider: past foreign days are unavailable and
// the provider is asked once, not once per day.
func TestSeriesStopsAskingAFailedProvider(t *testing.T) {
	f := newFixture(t)
	usd := f.addAccount("", "100")
	f.setCurrency("financial_accounts", usd, "USD")
	f.addTransaction(usd, daysAgo(30), "1", "CREDIT")
	f.addTransaction(usd, daysAgo(10), "1", "CREDIT")

	rates := &pastOutageRates{}
	points, err := networth.SeriesWith(context.Background(), f.conn, today, rates)
	if err != nil {
		t.Fatalf("a past-day outage is not an error: %v", err)
	}
	if rates.calls != 1 {
		t.Fatalf("rate lookups during an outage = %d, want 1 for the 20 foreign days", rates.calls)
	}
	if p, _ := pointOn(t, points, daysAgo(5)); p.Status != networth.ValuationUnavailable {
		t.Fatalf("a day that needed the failed provider: %q", p.Status)
	}
	if p, _ := pointOn(t, points, today); p.Status != networth.ValuationComplete {
		t.Fatalf("today: %q", p.Status)
	}
}

func TestSeriesFailsWhenTodayCannotBeValued(t *testing.T) {
	f := newFixture(t)
	usd := f.addAccount("", "100")
	f.setCurrency("financial_accounts", usd, "USD")
	if _, err := networth.SeriesWith(context.Background(), f.conn, today, f.fx(nil)); err == nil {
		t.Fatal("today without a rate was silently dropped")
	}
}

// TestSeriesIncludesInvestmentsOnPastDays pins the point of deriving on read:
// a past day carries the investments it held, so the history does not jump
// when investments start counting, and a corrected operation moves every day
// after it.
func TestSeriesIncludesInvestmentsOnPastDays(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	bank := f.addAccount("BANK", "5000")
	f.addTransaction(bank, daysAgo(10), "1.00", "CREDIT") // opens the window
	account, err := investments.CreateAccount(ctx, f.conn, investments.AccountInput{Name: "Corretora"})
	if err != nil {
		t.Fatal(err)
	}
	position, err := investments.CreatePosition(ctx, f.conn, investments.PositionInput{AccountID: account.ID, Name: "Ativo", AssetType: "Ações"})
	if err != nil {
		t.Fatal(err)
	}
	create := func(in investments.OperationInput) {
		t.Helper()
		in.AccountID = account.ID
		if _, err := investments.CreateOperation(ctx, f.conn, in); err != nil {
			t.Fatal(err)
		}
	}
	qty := decimal.RequireFromString("10")
	create(investments.OperationInput{Kind: investments.OperationDeposit, OccurredOn: daysAgo(6), Amount: decimal.RequireFromString("1000")})
	create(investments.OperationInput{PositionID: &position.ID, Kind: investments.OperationBuy, OccurredOn: daysAgo(5), Amount: decimal.RequireFromString("1000"), Quantity: &qty})
	create(investments.OperationInput{PositionID: &position.ID, Kind: investments.OperationValuation, OccurredOn: daysAgo(2), Amount: decimal.RequireFromString("1300")})

	points := f.series(today, nil)
	want := map[int]string{7: "0", 6: "1000", 5: "1000", 3: "1000", 2: "1300", 1: "1300", 0: "1300"}
	for ago, invested := range want {
		p, ok := pointOn(t, points, daysAgo(ago))
		if !ok {
			t.Fatalf("no point %d days ago", ago)
		}
		assertDecimalEqual(t, fmt.Sprintf("investments %d days ago", ago), p.Breakdown.InvestmentBalance, invested)
		assertDecimalEqual(t, fmt.Sprintf("cash %d days ago", ago), p.Breakdown.CashBalance, "5000")
	}

	f.exec(`UPDATE investment_operations SET amount = '1500' WHERE kind = 'valuation'`)
	points = f.series(today, nil)
	for ago, invested := range map[int]string{3: "1000", 2: "1500", 0: "1500"} {
		p, _ := pointOn(t, points, daysAgo(ago))
		assertDecimalEqual(t, fmt.Sprintf("corrected investments %d days ago", ago), p.Breakdown.InvestmentBalance, invested)
	}
}

// TestSeriesLeavesADayWithAnUnpricedHoldingUnavailable: a provider holding
// with no price on a day is unknown that day, not zero.
func TestSeriesLeavesADayWithAnUnpricedHoldingUnavailable(t *testing.T) {
	f := newFixture(t)
	f.addAccount("", "100")
	f.addTransaction(f.addAccount("", "0"), daysAgo(3), "1.00", "CREDIT")
	f.addInvestment("500") // today's balance, with no quote history behind it
	f.exec(`UPDATE financial_investments SET quantity = '5' WHERE source_id = ?`, f.sourceID)

	points := f.series(today, nil)
	if p, _ := pointOn(t, points, daysAgo(1)); p.Status != networth.ValuationUnavailable {
		t.Fatalf("an unpriced holding: status = %q", p.Status)
	}
	last := points[len(points)-1]
	assertDecimalEqual(t, "today is the provider balance", last.Breakdown.InvestmentBalance, "500")
}
