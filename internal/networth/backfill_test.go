package networth_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/networth"
	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/transactions"
)

func (f *fixture) setIgnored(transactionID string) {
	f.t.Helper()
	if _, err := transactions.SetInclusion(context.Background(), f.conn, transactionID, "ignored", transactions.InclusionOriginManual, nil, nil, nil); err != nil {
		f.t.Fatalf("SetInclusion: %v", err)
	}
}

// categoryTransferencia is the seeded "Transferência entre Contas Próprias"
// category (00004_categories.sql), the one kind that gates totals.
const categoryTransferencia = "533d9187-99b6-542b-a2f3-6eb9cbb299ce"

func (f *fixture) setCategory(transactionID, categoryID string) {
	f.t.Helper()
	if _, err := categories.AssignManual(context.Background(), f.conn, transactionID, categoryID); err != nil {
		f.t.Fatalf("AssignManual: %v", err)
	}
}

func snapshotFor(t *testing.T, f *fixture, day time.Time) (networth.SnapshotRow, bool) {
	t.Helper()
	rows, err := networth.List(context.Background(), f.conn, day, day)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) == 0 {
		return networth.SnapshotRow{}, false
	}
	return rows[0], true
}

// TestBackfillReconstructsCashWalkingEligibleTransactionsBackward proves the
// core cash reconstruction: today's balance minus every real transaction's
// effect, walked backward day by day — including a transaction the user
// marked "ignored", since ignored only means "excluded from income/expense
// totals" and the money still moved through the real, synced account
// balance that today's figure starts from.
func TestBackfillReconstructsCashWalkingEligibleTransactionsBackward(t *testing.T) {
	f := newFixture(t)
	accountID := f.addAccount("", "1000.00")

	today := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	twoDaysAgo := today.AddDate(0, 0, -2)
	oneDayAgo := today.AddDate(0, 0, -1)

	f.addCardTransaction(accountID, twoDaysAgo, "-100.00", "DEBIT") // outflow: balance was 100 lower before this happened
	f.addCardTransaction(accountID, oneDayAgo, "50.00", "CREDIT")   // inflow: balance was 50 lower before this happened

	ignoredID := f.addCardTransaction(accountID, oneDayAgo, "9999.00", "DEBIT")
	f.setIgnored(ignoredID) // excluded from totals, but still reversed here — it really happened

	if err := networth.Backfill(context.Background(), f.conn, today); err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	snapOneDayAgo, ok := snapshotFor(t, f, oneDayAgo)
	if !ok {
		t.Fatalf("no snapshot for oneDayAgo")
	}
	assertDecimalEqual(t, "oneDayAgo.CashBalance", snapOneDayAgo.Breakdown.CashBalance, "1000")
	if !snapOneDayAgo.IsBackfilled {
		t.Errorf("oneDayAgo.IsBackfilled = false, want true")
	}

	snapTwoDaysAgo, ok := snapshotFor(t, f, twoDaysAgo)
	if !ok {
		t.Fatalf("no snapshot for twoDaysAgo")
	}
	assertDecimalEqual(t, "twoDaysAgo.CashBalance", snapTwoDaysAgo.Breakdown.CashBalance, "10949")

	// Investments are never reconstructed — see Backfill's doc comment.
	assertDecimalEqual(t, "twoDaysAgo.InvestmentBalance", snapTwoDaysAgo.Breakdown.InvestmentBalance, "0")
	assertDecimalEqual(t, "twoDaysAgo.TotalAssets", snapTwoDaysAgo.Breakdown.TotalAssets, "10949")
}

// TestBackfillDoesNotReverseNonBRLTransactions pins that a USD inflow is not
// subtracted from the BRL cash anchor when reconstructing earlier days.
func TestBackfillDoesNotReverseNonBRLTransactions(t *testing.T) {
	f := newFixture(t)
	brl := f.addAccount("", "200.00")
	usd := f.addAccount("", "100.00")
	f.exec(`UPDATE financial_accounts SET currency_code = 'USD' WHERE id = ?`, usd)

	today := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	fourDaysAgo := today.AddDate(0, 0, -4)
	threeDaysAgo := today.AddDate(0, 0, -3)
	twoDaysAgo := today.AddDate(0, 0, -2)
	f.addCardTransaction(brl, fourDaysAgo, "10.00", "CREDIT") // opens the backfill window before the USD movement
	f.addCardTransaction(usd, twoDaysAgo, "100.00", "CREDIT")

	if err := networth.Backfill(context.Background(), f.conn, today); err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	snap, ok := snapshotFor(t, f, threeDaysAgo)
	if !ok {
		t.Fatalf("no snapshot for threeDaysAgo")
	}
	assertDecimalEqual(t, "threeDaysAgo.CashBalance", snap.Breakdown.CashBalance, "200")
}

// TestBackfillDoesNotReverseForeignAccountWithoutAccountAmount pins that a
// USD account's transaction with no account-currency amount (so it falls back
// to its BRL transaction amount) is not subtracted from the BRL cash anchor.
func TestBackfillDoesNotReverseForeignAccountWithoutAccountAmount(t *testing.T) {
	f := newFixture(t)
	brl := f.addAccount("", "200.00")
	usd := f.addAccount("", "100.00")
	f.exec(`UPDATE financial_accounts SET currency_code = 'USD' WHERE id = ?`, usd)

	today := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	fourDaysAgo := today.AddDate(0, 0, -4)
	threeDaysAgo := today.AddDate(0, 0, -3)
	twoDaysAgo := today.AddDate(0, 0, -2)
	f.addCardTransaction(brl, fourDaysAgo, "10.00", "CREDIT") // opens the backfill window before the USD movement
	id := f.addCardTransaction(usd, twoDaysAgo, "100.00", "CREDIT")
	f.exec(`UPDATE financial_transactions SET amount_in_account_currency = NULL WHERE id = ?`, id)

	if err := networth.Backfill(context.Background(), f.conn, today); err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	snap, ok := snapshotFor(t, f, threeDaysAgo)
	if !ok {
		t.Fatalf("no snapshot for threeDaysAgo")
	}
	assertDecimalEqual(t, "threeDaysAgo.CashBalance", snap.Breakdown.CashBalance, "200")
}

// TestBackfillSkipsDaysBeforeTheEarliestKnownTransaction proves Backfill
// never invents a day it has no transaction coverage for: with a single
// cash transaction three days back, only that day and days after it (up to
// yesterday) get a snapshot.
func TestBackfillSkipsDaysBeforeTheEarliestKnownTransaction(t *testing.T) {
	f := newFixture(t)
	accountID := f.addAccount("", "500.00")

	today := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	threeDaysAgo := today.AddDate(0, 0, -3)
	f.addCardTransaction(accountID, threeDaysAgo, "10.00", "CREDIT")

	if err := networth.Backfill(context.Background(), f.conn, today); err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	if _, ok := snapshotFor(t, f, threeDaysAgo.AddDate(0, 0, -1)); ok {
		t.Errorf("found a snapshot for a day before the earliest known transaction, want none")
	}
	if _, ok := snapshotFor(t, f, threeDaysAgo); !ok {
		t.Errorf("no snapshot for the earliest known transaction's own day")
	}
	if _, ok := snapshotFor(t, f, today.AddDate(0, 0, -1)); !ok {
		t.Errorf("no snapshot for yesterday")
	}
}

// TestBackfillGoesBackFurtherThanNinetyDays proves there is no arbitrary cap
// on how far Backfill reconstructs: it goes as deep as real transaction
// history allows, per the user's explicit request to backfill everything
// available rather than stop at a fixed window.
func TestBackfillGoesBackFurtherThanNinetyDays(t *testing.T) {
	f := newFixture(t)
	accountID := f.addAccount("", "1000.00")

	today := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	farBack := today.AddDate(0, 0, -200)
	f.addCardTransaction(accountID, farBack, "10.00", "CREDIT")

	if err := networth.Backfill(context.Background(), f.conn, today); err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	if _, ok := snapshotFor(t, f, farBack); !ok {
		t.Errorf("no snapshot for a day 200 days back, want the full transaction history reconstructed")
	}
}

// TestBackfillWithNoTransactionsAtAllProducesNoSnapshots covers the "account
// connected too recently to reconstruct anything" case: no non-CREDIT
// transaction ever recorded means cash can't be reconstructed for any past
// day, so Backfill must be a complete no-op rather than invent a flat series.
func TestBackfillWithNoTransactionsAtAllProducesNoSnapshots(t *testing.T) {
	f := newFixture(t)
	f.addAccount("", "1000.00")

	today := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	if err := networth.Backfill(context.Background(), f.conn, today); err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	rows, err := networth.List(context.Background(), f.conn, today.AddDate(0, 0, -90), today)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("len(rows) = %d, want 0", len(rows))
	}
}

// TestBackfillNeverOverwritesAnExistingSnapshot proves point 1 of the plan:
// a day already computed live (via Snapshot) keeps its own numbers, even if
// Backfill's reconstruction would have produced something different.
func TestBackfillNeverOverwritesAnExistingSnapshot(t *testing.T) {
	f := newFixture(t)
	accountID := f.addAccount("", "1000.00")

	today := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	yesterday := today.AddDate(0, 0, -1)
	f.addCardTransaction(accountID, yesterday, "10.00", "CREDIT")

	now := db.FormatTime(time.Now())
	f.exec(`INSERT INTO net_worth_snapshots (
			id, captured_at, total_assets, total_liabilities, net_worth,
			cash_balance, investment_balance, credit_card_balance, payables_debt,
			is_backfilled, created_at, updated_at
		) VALUES (?, ?, '1', '0', '1', '1', '0', '0', '0', 0, ?, ?)`,
		"existing-row-id", yesterday.Format("2006-01-02"), now, now)

	if err := networth.Backfill(context.Background(), f.conn, today); err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	snap, ok := snapshotFor(t, f, yesterday)
	if !ok {
		t.Fatalf("no snapshot for yesterday")
	}
	assertDecimalEqual(t, "yesterday.Breakdown.CashBalance", snap.Breakdown.CashBalance, "1")
	if snap.IsBackfilled {
		t.Errorf("IsBackfilled = true, want the pre-existing live row untouched")
	}
}

// TestBackfillReconstructsCreditCardDebtFromBillCycle reuses
// transactions.CreditCardTransactionTotalAt's arbitrary-reference-date
// support: a card debit assigned to a past bill counts toward that day's
// CreditCardBalance the same way it counts toward today's.
func TestBackfillReconstructsCreditCardDebtFromBillCycle(t *testing.T) {
	f := newFixture(t)
	loc := time.Local
	today := time.Date(2024, 6, 15, 0, 0, 0, 0, loc)
	targetDay := time.Date(2024, 6, 5, 0, 0, 0, 0, loc)
	lastClosing := time.Date(2024, 6, 2, 0, 0, 0, 0, loc)
	nextClosing := time.Date(2024, 7, 2, 0, 0, 0, 0, loc)
	earliestCash := time.Date(2024, 5, 1, 0, 0, 0, 0, loc)

	cashAccountID := f.addAccount("", "1000.00")
	f.addCardTransaction(cashAccountID, earliestCash, "1.00", "CREDIT") // only to satisfy the cash-history gate

	cardAccountID := f.addAccount("CREDIT", "0.00")
	txID := f.addCardTransaction(cardAccountID, targetDay, "-100.00", "DEBIT")
	f.addBill(cardAccountID, "bill-history", lastClosing, "")
	f.addBill(cardAccountID, "bill-current", nextClosing, txID)

	if err := networth.Backfill(context.Background(), f.conn, today); err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	snap, ok := snapshotFor(t, f, targetDay)
	if !ok {
		t.Fatalf("no snapshot for targetDay")
	}
	assertDecimalEqual(t, "targetDay.CreditCardBalance", snap.Breakdown.CreditCardBalance, "100")
}

// TestBackfillReconstructsPayablesDebtFromLinkedTransactionDate proves a
// debt's remaining amount as of a past day only reflects links whose
// transaction had already occurred by then — a payment made the next day
// hasn't happened yet as of the day before.
func TestBackfillReconstructsPayablesDebtFromLinkedTransactionDate(t *testing.T) {
	f := newFixture(t)
	today := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	earliestCash := today.AddDate(0, 0, -10)
	paymentDay := today.AddDate(0, 0, -5)
	dayBeforePayment := paymentDay.AddDate(0, 0, -1)

	cashAccountID := f.addAccount("", "1000.00")
	f.addCardTransaction(cashAccountID, earliestCash, "1.00", "CREDIT") // satisfies the cash-history gate

	payableID := f.addPayable(payables.KindDebt, "300.00", "300.00")
	f.backdatePayable(payableID, earliestCash)
	paymentTxID := f.addCardTransaction(cashAccountID, paymentDay, "-150.00", "DEBIT")
	if result, err := payables.CreateLink(context.Background(), f.conn, payables.KindDebt, payableID, paymentTxID); err != nil || result.Status != payables.StatusCreated {
		t.Fatalf("CreateLink: result=%+v err=%v", result, err)
	}

	if err := networth.Backfill(context.Background(), f.conn, today); err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	before, ok := snapshotFor(t, f, dayBeforePayment)
	if !ok {
		t.Fatalf("no snapshot for dayBeforePayment")
	}
	assertDecimalEqual(t, "dayBeforePayment.PayablesDebt", before.Breakdown.PayablesDebt, "300")

	onPaymentDay, ok := snapshotFor(t, f, paymentDay)
	if !ok {
		t.Fatalf("no snapshot for paymentDay")
	}
	assertDecimalEqual(t, "paymentDay.PayablesDebt", onPaymentDay.Breakdown.PayablesDebt, "150")
}

// TestBackfillReversesTransferCategorizedTransactions is the guard rail for
// the transfer rule's blast radius. Categorizing a transaction as a transfer
// keeps it out of income/expense totals, but it must never reach balance
// reconstruction: the money really did leave this account — possibly to an
// account that isn't tracked at all — so today's synced balance already
// reflects it, and every day before it has to reverse it out. If the rule
// ever leaks into networth, each day before a transfer ends up off by its
// full amount.
func TestBackfillReversesTransferCategorizedTransactions(t *testing.T) {
	f := newFixture(t)
	accountID := f.addAccount("", "1000.00")

	today := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	twoDaysAgo := today.AddDate(0, 0, -2)
	oneDayAgo := today.AddDate(0, 0, -1)

	// An anchor on the older day, so twoDaysAgo is within backfill coverage.
	// A day's snapshot only reverses what happened *after* it, so the
	// transfer has to sit on the later day to be reversed out of this one.
	f.addCardTransaction(accountID, twoDaysAgo, "-1.00", "DEBIT")

	transferID := f.addCardTransaction(accountID, oneDayAgo, "-300.00", "DEBIT")
	f.setCategory(transferID, categoryTransferencia)

	if err := networth.Backfill(context.Background(), f.conn, today); err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	snap, ok := snapshotFor(t, f, twoDaysAgo)
	if !ok {
		t.Fatalf("no snapshot for twoDaysAgo")
	}
	// 1000 today + 300 reversed back out = 1300 before the transfer left.
	assertDecimalEqual(t, "twoDaysAgo.CashBalance", snap.Breakdown.CashBalance, "1300")
}

// TestBackfillIgnoresManualTransactions guards the other blast radius: a
// lançamento manual describes money that moved outside what the provider
// tracks, so financial_accounts.balance — what today's figure is built from
// — never included it. If it leaked into cashDeltasDescending, every day
// before it would be off by its full amount, exactly like an un-excluded
// transfer would be.
func TestBackfillIgnoresManualTransactions(t *testing.T) {
	f := newFixture(t)
	accountID := f.addAccount("", "1000.00")

	today := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	twoDaysAgo := today.AddDate(0, 0, -2)
	oneDayAgo := today.AddDate(0, 0, -1)

	// An anchor on the older day, so twoDaysAgo is within backfill coverage
	// (same technique as TestBackfillReversesTransferCategorizedTransactions)
	// — a day's snapshot only reverses what happened *after* it.
	f.addCardTransaction(accountID, twoDaysAgo, "-1.00", "DEBIT")

	if _, err := transactions.CreateManual(context.Background(), f.conn, transactions.ManualInput{
		AccountID: accountID, Description: "Compra em dinheiro",
		Amount: decimal.RequireFromString("-9999.00"), OccurredAt: oneDayAgo,
	}); err != nil {
		t.Fatalf("CreateManual: %v", err)
	}

	if err := networth.Backfill(context.Background(), f.conn, today); err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	snap, ok := snapshotFor(t, f, twoDaysAgo)
	if !ok {
		t.Fatalf("no snapshot for twoDaysAgo")
	}
	// If the manual -9999.00 leaked into reconstruction, this would read
	// 10999 (1000 + 9999 reversed back out). It must not shift at all: the
	// manual entry never touched the real balance to begin with.
	assertDecimalEqual(t, "twoDaysAgo.CashBalance", snap.Breakdown.CashBalance, "1000")
}

func TestBackfillManualAccountMovementsAndAdjustments(t *testing.T) {
	f := newFixture(t)
	today := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	first := db.FormatTime(today.AddDate(0, 0, -2))
	f.exec(`INSERT INTO financial_accounts(id,origin,name,account_type,currency_code,opening_balance,created_at,updated_at) VALUES('manual','manual','Cash','BANK','BRL','100',?,?)`, first, first)
	_, err := transactions.CreateManual(context.Background(), f.conn, transactions.ManualInput{AccountID: "manual", Description: "Deposit", Amount: decimal.RequireFromString("10"), OccurredAt: today.AddDate(0, 0, -2)})
	if err != nil {
		t.Fatal(err)
	}
	f.exec(`INSERT INTO account_balance_adjustments(id,account_id,amount,description,created_at) VALUES('adjust','manual','5','Correction',?)`, db.FormatTime(today.AddDate(0, 0, -1)))
	foreign := f.addAccount("BANK", "10000")
	f.exec(`UPDATE financial_accounts SET currency_code='USD' WHERE id=?`, foreign)
	f.addCardTransaction(foreign, today.AddDate(0, 0, -1), "1000", "CREDIT")
	if err := networth.Backfill(context.Background(), f.conn, today); err != nil {
		t.Fatal(err)
	}
	day, ok := snapshotFor(t, f, today.AddDate(0, 0, -2))
	if !ok || !day.Breakdown.CashBalance.Equal(decimal.RequireFromString("110")) {
		t.Fatalf("first day: %+v %v", day, ok)
	}
	day, ok = snapshotFor(t, f, today.AddDate(0, 0, -1))
	if !ok || !day.Breakdown.CashBalance.Equal(decimal.RequireFromString("115")) {
		t.Fatalf("adjustment day: %+v %v", day, ok)
	}
}
