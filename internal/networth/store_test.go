package networth_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/networth"
	"github.com/greg0x46/julius/internal/payables"
)

type fixture struct {
	t           *testing.T
	conn        *sql.DB
	sourceID    string
	rawImportID string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	f := &fixture{t: t, conn: conn, sourceID: uuid.NewString(), rawImportID: uuid.NewString()}
	now := db.FormatTime(time.Now())
	f.exec(`INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at)
		VALUES (?, 'pluggy', 'item-1', ?, ?)`, f.sourceID, now, now)
	syncRunID := uuid.NewString()
	f.exec(`INSERT INTO sync_runs (id, source_id, status, started_at, finished_at)
		VALUES (?, ?, 'completed', ?, ?)`, syncRunID, f.sourceID, now, now)
	f.exec(`INSERT INTO raw_imports (
			id, sync_run_id, source_id, scope, page_sequence, request_attempt,
			request_method, request_path, http_status, response_headers, payload,
			payload_sha256, received_at
		) VALUES (?, ?, ?, 'accounts', 1, 1, 'GET', '/x', 200, '{}', x'00', 'sha', ?)`,
		f.rawImportID, syncRunID, f.sourceID, now)
	return f
}

func (f *fixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.conn.Exec(query, args...); err != nil {
		f.t.Fatalf("exec %q: %v", query, err)
	}
}

// addAccount inserts a financial_accounts row. accountType "" leaves the
// column NULL, matching a real non-credit account synced without a type.
func (f *fixture) addAccount(accountType, balance string) string {
	f.t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	var accountTypeArg any
	if accountType != "" {
		accountTypeArg = accountType
	}
	f.exec(`INSERT INTO financial_accounts (
			id, source_id, external_id, account_type, balance, currency_code,
			current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, 'BRL', ?, 'hash', ?, ?)`,
		id, f.sourceID, id, accountTypeArg, balance, f.rawImportID, now, now)
	return id
}

func (f *fixture) addInvestment(balance string) string {
	f.t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	f.exec(`INSERT INTO financial_investments (
			id, source_id, external_id, balance, currency_code,
			current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, 'BRL', ?, 'hash', ?, ?)`,
		id, f.sourceID, id, balance, f.rawImportID, now, now)
	return id
}

// addCardTransaction inserts a financial_transactions row on accountID,
// shaped for transactions.CreditCardTransactionTotal's cycle math
// (occurred_at, movement_type, POSTED status — all it filters on).
func (f *fixture) addCardTransaction(accountID string, occurredAt time.Time, amount, movementType string) string {
	f.t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	f.exec(`INSERT INTO financial_transactions (
			id, source_id, account_id, external_id, description, amount, amount_in_account_currency,
			currency_code, occurred_at, provider_status, movement_type, current_raw_import_id,
			normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, 'Compra', ?, ?, 'BRL', ?, 'POSTED', ?, ?, 'hash', ?, ?)`,
		id, f.sourceID, accountID, id, amount, amount, db.FormatTime(occurredAt), movementType, f.rawImportID, now, now)
	return id
}

// addBill inserts a financial_bills row and — when billExternalID is set —
// tags transactionID's credit_card_metadata with it, mirroring
// internal/httpapi/server_test.go's insertBillForTransaction fixture used to
// test transactions.CreditCardTransactionTotal's HTTP-facing twin
// (handlePayableTotalOwed).
func (f *fixture) addBill(accountID, billExternalID string, closingDate time.Time, transactionID string) {
	f.t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	closing := time.Date(closingDate.Year(), closingDate.Month(), closingDate.Day(), 0, 0, 0, 0, time.UTC)
	f.exec(`INSERT INTO financial_bills (
			id, source_id, account_id, external_id, due_date, closing_date,
			current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, 'hash', ?, ?)`,
		id, f.sourceID, accountID, billExternalID, db.FormatTime(closing), db.FormatTime(closing), f.rawImportID, now, now)
	if transactionID != "" {
		f.exec(`UPDATE financial_transactions SET credit_card_metadata = ? WHERE id = ?`,
			`{"billId":"`+billExternalID+`"}`, transactionID)
	}
}

func (f *fixture) addPayable(kind payables.Kind, totalAmount, initialRemainingAmount string) string {
	f.t.Helper()
	total := mustDecimal(f.t, totalAmount)
	initial := mustDecimal(f.t, initialRemainingAmount)
	p, err := payables.Create(context.Background(), f.conn, kind, "test payable", total, initial)
	if err != nil {
		f.t.Fatalf("Create payable: %v", err)
	}
	return p.ID
}

// backdatePayable overrides a payable's created_at, which payables.Create
// always stamps with the real wall-clock time — tests that reconstruct debt
// on a synthetic past day need the payable to actually predate that day.
func (f *fixture) backdatePayable(payableID string, createdAt time.Time) {
	f.t.Helper()
	f.exec(`UPDATE payables SET created_at = ? WHERE id = ?`, db.FormatTime(createdAt), payableID)
}

func mustDecimal(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.NewFromString(s)
	if err != nil {
		t.Fatalf("parse decimal %q: %v", s, err)
	}
	return d
}

func assertDecimalEqual(t *testing.T, label string, got decimal.Decimal, want string) {
	t.Helper()
	wantDec := mustDecimal(t, want)
	if !got.Equal(wantDec) {
		t.Errorf("%s = %s, want %s", label, got.String(), wantDec.String())
	}
}

func TestComputeSumsCashInvestmentsAndPayables(t *testing.T) {
	f := newFixture(t)
	f.addAccount("", "1000.00")                               // checking account, cash
	f.addAccount("CREDIT", "300.00")                          // provider balance — ignored, see TestComputeCreditCardIgnoresRawAccountBalance
	f.addInvestment("500.00")                                 // investment balance
	f.addPayable(payables.KindDebt, "200.00", "200.00")       // fully open debt (nothing settled yet)
	f.addPayable(payables.KindReceivable, "150.00", "150.00") // fully open receivable — must not affect the totals below

	b, err := networth.Compute(context.Background(), f.conn)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	assertDecimalEqual(t, "CashBalance", b.CashBalance, "1000")
	assertDecimalEqual(t, "InvestmentBalance", b.InvestmentBalance, "500")
	assertDecimalEqual(t, "PayablesDebt", b.PayablesDebt, "200")
	// assets = 1000 + 500 = 1500 (receivables never enter this model — a
	// future possibility, not a realized asset, see Breakdown's doc
	// comment); liabilities = 0 (no bill cycle, see below) + 200 = 200;
	// net = 1300
	assertDecimalEqual(t, "TotalAssets", b.TotalAssets, "1500")
	assertDecimalEqual(t, "TotalLiabilities", b.TotalLiabilities, "200")
	assertDecimalEqual(t, "NetWorth", b.NetWorth, "1300")
}

// TestComputeCashExcludesNonBRLAccounts pins that a USD account's balance is
// not added to the BRL cash figure; there is no conversion.
func TestComputeCashExcludesNonBRLAccounts(t *testing.T) {
	f := newFixture(t)
	f.addAccount("", "200.00")
	usd := f.addAccount("", "100.00")
	f.exec(`UPDATE financial_accounts SET currency_code = 'USD' WHERE id = ?`, usd)

	b, err := networth.Compute(context.Background(), f.conn)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	assertDecimalEqual(t, "CashBalance", b.CashBalance, "200")
	assertDecimalEqual(t, "NetWorth", b.NetWorth, "200")
}

// TestComputeCreditCardIgnoresRawAccountBalance guards the bug the homepage
// mismatch surfaced: CreditCardBalance must come from
// transactions.CreditCardTransactionTotal's bill-cycle transaction total, not
// financial_accounts.balance. Without any financial_bills rows there is no
// reliable cycle, so the correct answer is 0 even though the account's raw
// provider balance is nonzero.
func TestComputeCreditCardIgnoresRawAccountBalance(t *testing.T) {
	f := newFixture(t)
	f.addAccount("CREDIT", "9999.00")

	b, err := networth.Compute(context.Background(), f.conn)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	assertDecimalEqual(t, "CreditCardBalance", b.CreditCardBalance, "0")
}

// TestComputeCreditCardMatchesCurrentBillCycleTransactions proves
// CreditCardBalance agrees with the same math the homepage's "Dívida
// total" widget uses (transactions.CreditCardTransactionTotal): a historical
// closing plus the next (current-cycle) closing bound a window, and only
// the debit transaction assigned to the current-cycle bill counts.
func TestComputeCreditCardMatchesCurrentBillCycleTransactions(t *testing.T) {
	f := newFixture(t)
	accountID := f.addAccount("CREDIT", "0.00") // raw balance deliberately wrong/unused

	loc := time.Local
	now := time.Now().In(loc)
	currentClosing := time.Date(now.Year(), now.Month(), 2, 0, 0, 0, 0, loc)
	if now.Before(currentClosing) {
		currentClosing = time.Date(now.Year(), now.Month()-1, 2, 0, 0, 0, 0, loc)
	}
	nextClosing := time.Date(currentClosing.Year(), currentClosing.Month()+1, 2, 0, 0, 0, 0, loc)

	txID := f.addCardTransaction(accountID, now, "-100.00", "DEBIT")
	f.addBill(accountID, "bill-history", currentClosing, "") // establishes the lower bound
	f.addBill(accountID, "bill-current", nextClosing, txID)  // the transaction's own (current-cycle) bill

	b, err := networth.Compute(context.Background(), f.conn)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	assertDecimalEqual(t, "CreditCardBalance", b.CreditCardBalance, "100")
}

func TestComputeIgnoresSettledPayables(t *testing.T) {
	f := newFixture(t)
	// initialRemainingAmount = 0 means starting_settled_amount = total - 0 =
	// total, so this debt is already fully settled and contributes nothing.
	f.addPayable(payables.KindDebt, "100.00", "0.00")

	b, err := networth.Compute(context.Background(), f.conn)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	assertDecimalEqual(t, "PayablesDebt", b.PayablesDebt, "0")
}

func TestSnapshotIsIdempotentForTheSameDay(t *testing.T) {
	f := newFixture(t)
	f.addAccount("", "1000.00")

	first, err := networth.Snapshot(context.Background(), f.conn)
	if err != nil {
		t.Fatalf("Snapshot (first): %v", err)
	}

	f.addAccount("", "500.00") // balance changes between the two snapshot calls

	second, err := networth.Snapshot(context.Background(), f.conn)
	if err != nil {
		t.Fatalf("Snapshot (second): %v", err)
	}

	if first.ID != second.ID {
		t.Errorf("second snapshot got a new row (id %s != %s), want the same day's row updated in place", second.ID, first.ID)
	}
	assertDecimalEqual(t, "second.CashBalance", second.Breakdown.CashBalance, "1500")

	rows, err := networth.List(context.Background(), f.conn, first.CapturedAt.AddDate(0, 0, -1), first.CapturedAt.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1 (idempotent upsert for the same captured_at)", len(rows))
	}
}

func TestListReturnsSnapshotsInDateRange(t *testing.T) {
	f := newFixture(t)
	f.addAccount("", "100.00")

	snap, err := networth.Snapshot(context.Background(), f.conn)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	inRange, err := networth.List(context.Background(), f.conn, snap.CapturedAt, snap.CapturedAt)
	if err != nil {
		t.Fatalf("List (in range): %v", err)
	}
	if len(inRange) != 1 {
		t.Fatalf("len(inRange) = %d, want 1", len(inRange))
	}

	outOfRange, err := networth.List(context.Background(), f.conn, snap.CapturedAt.AddDate(0, 0, -10), snap.CapturedAt.AddDate(0, 0, -5))
	if err != nil {
		t.Fatalf("List (out of range): %v", err)
	}
	if len(outOfRange) != 0 {
		t.Fatalf("len(outOfRange) = %d, want 0", len(outOfRange))
	}
}
