package invariants_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/networth"
	"github.com/greg0x46/julius/internal/timeline"
	"github.com/greg0x46/julius/internal/transactions"
)

// fixture is a deliberate copy of the package-private transactions/timeline
// fixtures: the sync-schema chain (data_sources -> sync_runs -> raw_imports)
// that financial_accounts/financial_transactions require as foreign keys.
type fixture struct {
	t           *testing.T
	ctx         context.Context
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

	f := &fixture{t: t, ctx: context.Background(), conn: conn, sourceID: uuid.NewString(), rawImportID: uuid.NewString()}
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
		) VALUES (?, ?, ?, 'transactions', 1, 1, 'GET', '/x', 200, '{}', x'00', 'sha', ?)`,
		f.rawImportID, syncRunID, f.sourceID, now)
	return f
}

func (f *fixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.conn.Exec(query, args...); err != nil {
		f.t.Fatalf("exec %q: %v", query, err)
	}
}

func (f *fixture) count(query string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.conn.QueryRow(query, args...).Scan(&n); err != nil {
		f.t.Fatalf("count %q: %v", query, err)
	}
	return n
}

// addAccount inserts a BRL account. An empty accountType stays NULL, the
// "provider never told us" case CashOnHand treats as non-credit.
func (f *fixture) addAccount(balance, accountType string) string {
	f.t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	var kind any
	if accountType != "" {
		kind = accountType
	}
	f.exec(`INSERT INTO financial_accounts (
			id, source_id, external_id, name, currency_code, account_type, balance,
			current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, 'Conta', 'BRL', ?, ?, ?, 'hash', ?, ?)`,
		id, f.sourceID, id, kind, balance, f.rawImportID, now, now)
	return id
}

// addTx inserts a BRL transaction with Pluggy's signed amount: negative is a
// DEBIT, positive a CREDIT.
func (f *fixture) addTx(accountID, signedAmount string, occurredAt time.Time, status string) string {
	f.t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	movementType := "CREDIT"
	if strings.HasPrefix(signedAmount, "-") {
		movementType = "DEBIT"
	}
	f.exec(`INSERT INTO financial_transactions (
			id, source_id, account_id, external_id, description, amount,
			amount_in_account_currency, currency_code, occurred_at, provider_status,
			movement_type, current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, 'Transação', ?, ?, 'BRL', ?, ?, ?, ?, 'hash', ?, ?)`,
		id, f.sourceID, accountID, id, signedAmount, signedAmount, db.FormatTime(occurredAt),
		status, movementType, f.rawImportID, now, now)
	return id
}

func (f *fixture) setCategory(transactionID, categoryID string) {
	f.t.Helper()
	now := db.FormatTime(time.Now())
	f.exec(`INSERT INTO transaction_category_decisions
			(transaction_id, category_id, revision, changed_at, origin)
		 VALUES (?, ?, 1, ?, 'manual')`, transactionID, categoryID, now)
	f.exec(`INSERT INTO transaction_category_events
			(id, transaction_id, revision, previous_category_id, resulting_category_id, changed_at, origin)
		 VALUES (?, ?, 1, NULL, ?, ?, 'manual')`, uuid.NewString(), transactionID, categoryID, now)
}

func (f *fixture) setInclusion(transactionID string, state money.InclusionState, onIgnored transactions.OnIgnoredHook) transactions.InclusionConfirmation {
	f.t.Helper()
	confirmation, err := transactions.SetInclusion(f.ctx, f.conn, transactionID, state, transactions.InclusionOriginManual, nil, nil, onIgnored)
	if err != nil {
		f.t.Fatalf("SetInclusion(%s): %v", state, err)
	}
	return confirmation
}

// totals is the income/expense reading of transactions.Query: inflow and
// outflow magnitudes across every BRL item.
func (f *fixture) totals() (inflow, outflow decimal.Decimal) {
	f.t.Helper()
	result, err := transactions.Query(f.ctx, f.conn, transactions.QueryRequest{
		Timezone: "UTC", GroupBy: money.GroupNone, Page: 1, PageSize: 500,
	})
	if err != nil {
		f.t.Fatalf("Query: %v", err)
	}
	if len(result.Totals) == 0 {
		return decimal.Zero, decimal.Zero
	}
	if len(result.Totals) != 1 {
		f.t.Fatalf("Totals = %+v, want one currency", result.Totals)
	}
	return mustDec(f.t, result.Totals[0].Inflow), mustDec(f.t, result.Totals[0].Outflow)
}

func (f *fixture) item(id string) transactions.Item {
	f.t.Helper()
	item, found, err := transactions.GetItem(f.ctx, f.conn, id)
	if err != nil || !found {
		f.t.Fatalf("GetItem(%s): found=%v err=%v", id, found, err)
	}
	return item
}

func (f *fixture) cash() decimal.Decimal {
	f.t.Helper()
	cash, err := transactions.CashOnHand(f.ctx, f.conn, nil)
	if err != nil {
		f.t.Fatalf("CashOnHand: %v", err)
	}
	return cash
}

func (f *fixture) netWorth() networth.Breakdown {
	f.t.Helper()
	breakdown, err := networth.Compute(f.ctx, f.conn)
	if err != nil {
		f.t.Fatalf("networth.Compute: %v", err)
	}
	return breakdown
}

// series builds August 2026 with today's anchor on the last day, so every
// fixture transaction sits in the window's past.
func (f *fixture) series(params timeline.BuildParams) timeline.Series {
	f.t.Helper()
	params.From, params.To, params.ReferenceDate = day(1), day(31), day(31)
	series, err := timeline.BuildSeries(f.ctx, f.conn, params)
	if err != nil {
		f.t.Fatalf("BuildSeries: %v", err)
	}
	return series
}

func day(n int) time.Time { return time.Date(2026, 8, n, 12, 0, 0, 0, time.UTC) }

func mustDec(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.NewFromString(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return d
}

func assertDec(t *testing.T, label string, got decimal.Decimal, want string) {
	t.Helper()
	if !got.Equal(mustDec(t, want)) {
		t.Errorf("%s = %s, want %s", label, got, want)
	}
}

const (
	categorySupermercado  = "000433b6-3094-5a9c-87df-465b70574a4b" // expense
	categorySalario       = "3c5a9586-2a11-556d-b014-692ed51c3997" // income
	categoryTransferencia = "533d9187-99b6-542b-a2f3-6eb9cbb299ce" // transfer
)
