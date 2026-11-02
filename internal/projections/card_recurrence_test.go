package projections_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/projections"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/scenarios"
)

type cardFixture struct {
	t                         *testing.T
	conn                      *sql.DB
	sourceID, importID, accID string
}

// newAccount inserts an account of the given type; "CREDIT" is a credit card.
func newAccount(t *testing.T, conn *sql.DB, accountType string) *cardFixture {
	t.Helper()
	now := db.FormatTime(time.Now().UTC())
	f := &cardFixture{t: t, conn: conn, sourceID: uuid.NewString(), accID: uuid.NewString()}
	runID := uuid.NewString()
	f.importID = uuid.NewString()
	f.exec(`INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at) VALUES (?, 'pluggy', ?, ?, ?)`, f.sourceID, f.sourceID, now, now)
	f.exec(`INSERT INTO sync_runs (id, source_id, status, started_at, finished_at) VALUES (?, ?, 'completed', ?, ?)`, runID, f.sourceID, now, now)
	f.exec(`INSERT INTO raw_imports (id, sync_run_id, source_id, scope, page_sequence, request_attempt, request_method, request_path, http_status, response_headers, payload, payload_sha256, received_at) VALUES (?, ?, ?, 'transactions', 1, 1, 'GET', '/x', 200, '{}', x'00', 'hash', ?)`, f.importID, runID, f.sourceID, now)
	f.exec(`INSERT INTO financial_accounts (id, source_id, external_id, currency_code, account_type, current_raw_import_id, normalized_hash, created_at, updated_at) VALUES (?, ?, ?, 'BRL', ?, ?, 'hash', ?, ?)`, f.accID, f.sourceID, f.accID, accountType, f.importID, now, now)
	return f
}

func (f *cardFixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.conn.Exec(query, args...); err != nil {
		f.t.Fatalf("exec %q: %v", query, err)
	}
}

func (f *cardFixture) bill(closing, due string) {
	f.t.Helper()
	now := db.FormatTime(time.Now().UTC())
	id := uuid.NewString()
	f.exec(`INSERT INTO financial_bills (id, source_id, account_id, external_id, closing_date, due_date, current_raw_import_id, normalized_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, 'hash', ?, ?)`,
		id, f.sourceID, f.accID, id, db.FormatTime(projectionDate(f.t, closing)), db.FormatTime(projectionDate(f.t, due)), f.importID, now, now)
}

// bills: closing on the 3rd, due on the 10th, for Oct-Dec 2026.
func (f *cardFixture) bills() {
	f.bill("2026-10-03", "2026-10-10")
	f.bill("2026-11-03", "2026-11-10")
	f.bill("2026-12-03", "2026-12-10")
}

func (f *cardFixture) purchase(date, amount string) string {
	f.t.Helper()
	now := db.FormatTime(time.Now().UTC())
	id := uuid.NewString()
	f.exec(`INSERT INTO financial_transactions (id, source_id, account_id, external_id, description, amount, amount_in_account_currency, currency_code, occurred_at, provider_status, movement_type, current_raw_import_id, normalized_hash, created_at, updated_at) VALUES (?, ?, ?, ?, 'Streaming', ?, ?, 'BRL', ?, 'POSTED', 'DEBIT', ?, 'hash', ?, ?)`,
		id, f.sourceID, f.accID, id, amount, amount, date+"T00:00:00.000000000Z", f.importID, now, now)
	return id
}

func cardCommitment(t *testing.T, conn *sql.DB, accountID *string) recurrences.RecurringCommitment {
	t.Helper()
	commitment, err := recurrences.Create(context.Background(), conn, recurrences.Write{
		Name: "Streaming", Kind: recurrences.KindExpense, Amount: projectionDecimal(t, "40.00"),
		CategoryID: projectionCategoryID, AccountID: accountID, Cadence: recurrences.CadenceMonthly, DayOfMonth: 5,
		StartDate: projectionDate(t, "2026-01-01"), IsActive: true,
	})
	if err != nil {
		t.Fatalf("recurrences.Create: %v", err)
	}
	return commitment
}

func listWindow(t *testing.T, conn *sql.DB, id, from, to string) []projections.PlannedTransaction {
	t.Helper()
	events, err := projections.List(context.Background(), conn, projections.ProjectionQuery{
		From: projectionDate(t, from), To: projectionDate(t, to),
		Selection: projections.SelectionExplicit, IDs: []string{id},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	return events
}

func occurrenceKey(id, day string) string { return "scenario:" + id + ":occurrence:" + day }

// A card occurrence is dated on the bill that pays it, as the Timeline dates
// the card's real entries, while its key stays on the schedule day.
func TestRecurringOnCreditCardIsProjectedOnTheBillDueDate(t *testing.T) {
	conn := projectionDB(t)
	card := newAccount(t, conn, "CREDIT")
	card.bills()
	commitment := cardCommitment(t, conn, &card.accID)

	// The Oct 5 purchase is after the Oct 3 closing: it goes to the bill due Nov 10.
	november := listWindow(t, conn, commitment.ID, "2026-11-01", "2026-11-30")
	if len(november) != 1 || november[0].Date.Format("2006-01-02") != "2026-11-10" || november[0].EventKey != occurrenceKey(commitment.ID, "2026-10-05") {
		t.Fatalf("november = %+v, want the Oct 5 occurrence on Nov 10", november)
	}
	december := listWindow(t, conn, commitment.ID, "2026-12-01", "2026-12-31")
	if len(december) != 1 || december[0].Date.Format("2006-01-02") != "2026-12-10" || december[0].EventKey != occurrenceKey(commitment.ID, "2026-11-05") {
		t.Fatalf("december = %+v, want the Nov 5 occurrence on Dec 10", december)
	}
	// Past the synced bills the cycle is inferred: Dec 5 is after the Dec 3 closing.
	january := listWindow(t, conn, commitment.ID, "2027-01-01", "2027-01-31")
	if len(january) != 1 || january[0].Date.Format("2006-01-02") != "2027-01-10" {
		t.Fatalf("january = %+v, want the Dec 5 occurrence on Jan 10", january)
	}
}

func TestRecurringOnCreditCardIsNotDuplicatedAcrossWindows(t *testing.T) {
	conn := projectionDB(t)
	card := newAccount(t, conn, "CREDIT")
	card.bills()
	commitment := cardCommitment(t, conn, &card.accID)

	seen := map[string]string{}
	for _, window := range [][2]string{{"2026-10-01", "2026-10-31"}, {"2026-11-01", "2026-11-30"}, {"2026-12-01", "2026-12-31"}} {
		for _, event := range listWindow(t, conn, commitment.ID, window[0], window[1]) {
			if previous, ok := seen[event.EventKey]; ok {
				t.Fatalf("%s appears in two windows (%s and %s)", event.EventKey, previous, window[0])
			}
			seen[event.EventKey] = window[0]
		}
	}
}

func TestRecurringOnCreditCardWithoutBillsKeepsItsDay(t *testing.T) {
	conn := projectionDB(t)
	card := newAccount(t, conn, "CREDIT")
	commitment := cardCommitment(t, conn, &card.accID)

	events := listWindow(t, conn, commitment.ID, "2026-11-01", "2026-11-30")
	if len(events) != 1 || events[0].Date.Format("2006-01-02") != "2026-11-05" {
		t.Fatalf("events = %+v, want Nov 5", events)
	}
}

func TestRecurringOnABankAccountOrNoAccountKeepsItsDay(t *testing.T) {
	conn := projectionDB(t)
	bank := newAccount(t, conn, "BANK")
	bank.bills()
	for name, accountID := range map[string]*string{"bank": &bank.accID, "none": nil} {
		commitment := cardCommitment(t, conn, accountID)
		events := listWindow(t, conn, commitment.ID, "2026-11-01", "2026-11-30")
		if len(events) != 1 || events[0].Date.Format("2006-01-02") != "2026-11-05" {
			t.Fatalf("%s: events = %+v, want Nov 5", name, events)
		}
	}
}

// The match uses the transaction's own date: a purchase on Oct 5 reconciles the
// Oct 5 occurrence, which is read before the window yet projected inside it.
func TestRealizedCardOccurrenceBeforeTheWindowStaysRealized(t *testing.T) {
	conn := projectionDB(t)
	card := newAccount(t, conn, "CREDIT")
	card.bills()
	commitment := cardCommitment(t, conn, &card.accID)
	purchase := card.purchase("2026-10-05", "-40.00")
	if _, err := scenarios.RealizeEvent(context.Background(), conn, commitment.ID, occurrenceKey(commitment.ID, "2026-10-05"), scenarios.RealizationWrite{
		State: scenarios.RealizationStateLinked, TransactionID: &purchase,
	}); err != nil {
		t.Fatalf("link occurrence: %v", err)
	}

	events := listWindow(t, conn, commitment.ID, "2026-11-01", "2026-11-30")
	if len(events) != 1 || !events[0].Realized || events[0].Date.Format("2006-01-02") != "2026-11-10" {
		t.Fatalf("events = %+v, want the Oct 5 occurrence realized on Nov 10", events)
	}
}
