package syncsvc_test

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/automation"
	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/pluggy"
	"github.com/greg0x46/julius/internal/syncsvc"
	"github.com/greg0x46/julius/internal/transactions"
)

// ingestion syncs one data source repeatedly, each run with its own sync_runs
// and raw_imports rows, and counts automation hook calls.
type ingestion struct {
	conn      *sql.DB
	sourceID  string
	itemID    string
	provider  *fakeProvider
	hookCalls int
}

func newIngestion(t *testing.T, conn *sql.DB, itemID string, accounts ...string) *ingestion {
	t.Helper()
	now := db.FormatTime(time.Now())
	f := &ingestion{conn: conn, sourceID: uuid.NewString(), itemID: itemID}
	if _, err := conn.Exec(`INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at)
		VALUES (?, 'pluggy', ?, ?, ?)`, f.sourceID, itemID, now, now); err != nil {
		t.Fatalf("insert data_source: %v", err)
	}
	snapshots := make([]pluggy.AccountSnapshot, 0, len(accounts))
	for _, account := range accounts {
		snapshots = append(snapshots, pluggy.AccountSnapshot{ExternalID: account, CurrencyCode: strp("BRL")})
	}
	f.provider = &fakeProvider{
		source:       pluggy.SourceSnapshot{ExternalItemID: itemID, SafeProducts: map[string]bool{"ACCOUNTS": true, "TRANSACTIONS": true}},
		accountsPage: pluggy.AccountsPage{Accounts: snapshots},
	}
	return f
}

// run syncs records (keyed by external account id) and returns the run id.
func (f *ingestion) run(t *testing.T, records map[string][]pluggy.TransactionSnapshot) string {
	t.Helper()
	runID := uuid.NewString()
	if _, err := f.conn.Exec(`INSERT INTO sync_runs (id, source_id, status, started_at) VALUES (?, ?, 'in_progress', ?)`,
		runID, f.sourceID, db.FormatTime(time.Now())); err != nil {
		t.Fatalf("insert sync_run: %v", err)
	}
	accountsRaw := uuid.NewString()
	insertRawImport(t, f.conn, accountsRaw, runID, f.sourceID)
	f.provider.accountsPage.RawImportID = accountsRaw
	f.provider.transactionPages = map[string][]pluggy.TransactionsPage{}
	for account, page := range records {
		rawID := uuid.NewString()
		insertRawImport(t, f.conn, rawID, runID, f.sourceID)
		f.provider.transactionPages[account] = []pluggy.TransactionsPage{{RawImportID: rawID, Transactions: page}}
	}
	ruleHook := automation.NewTransactionHook(nil)
	hook := func(ctx context.Context, q transactions.Querier, transactionID, accountID string) error {
		f.hookCalls++
		return ruleHook(ctx, q, transactionID, accountID)
	}
	if err := (&syncsvc.Service{
		DB: f.conn, Provider: f.provider, SyncRunID: runID, SourceID: f.sourceID, OnTransactionUpserted: hook,
	}).Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return runID
}

// transactionID returns the id of this source's row for externalID.
func (f *ingestion) transactionID(t *testing.T, externalID string) string {
	t.Helper()
	var id string
	if err := f.conn.QueryRow(`SELECT id FROM financial_transactions WHERE source_id = ? AND external_id = ?`, f.sourceID, externalID).Scan(&id); err != nil {
		t.Fatalf("transaction %s: %v", externalID, err)
	}
	return id
}

func (f *ingestion) accountID(t *testing.T, externalID string) string {
	t.Helper()
	var id string
	if err := f.conn.QueryRow(`SELECT id FROM financial_accounts WHERE source_id = ? AND external_id = ?`, f.sourceID, externalID).Scan(&id); err != nil {
		t.Fatalf("account %s: %v", externalID, err)
	}
	return id
}

func count(t *testing.T, conn *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// ingestionState captures every table a sync could duplicate or rewrite.
func ingestionState(t *testing.T, conn *sql.DB) map[string]any {
	t.Helper()
	state := map[string]any{}
	for _, table := range []string{
		"financial_transactions", "transaction_category_decisions", "transaction_category_events",
		"transaction_inclusion_decisions", "transaction_inclusion_events", "payable_transaction_links", "scenario_realizations",
	} {
		state[table] = count(t, conn, `SELECT COUNT(*) FROM `+table)
	}
	state["normalized"] = count(t, conn, `SELECT COUNT(*) FROM normalization_events WHERE entity_type = 'transaction' AND outcome IN ('inserted', 'updated')`)
	state["category decisions"] = rows(t, conn, `SELECT transaction_id, category_id, origin FROM transaction_category_decisions ORDER BY transaction_id`)
	state["inclusion decisions"] = rows(t, conn, `SELECT transaction_id, state, origin FROM transaction_inclusion_decisions ORDER BY transaction_id`)
	state["transactions"] = rows(t, conn, `SELECT id, account_id, external_id, normalized_hash FROM financial_transactions ORDER BY id`)
	return state
}

// rows returns every row of query as text.
func rows(t *testing.T, conn *sql.DB, query string, args ...any) [][]string {
	t.Helper()
	result, err := conn.Query(query, args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer result.Close()
	cols, err := result.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	for result.Next() {
		values := make([]sql.NullString, len(cols))
		dest := make([]any, len(cols))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := result.Scan(dest...); err != nil {
			t.Fatal(err)
		}
		row := make([]string, len(cols))
		for i, v := range values {
			row[i] = v.String
		}
		out = append(out, row)
	}
	if err := result.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func debit(externalID, account, description, amount, status string) pluggy.TransactionSnapshot {
	return pluggy.TransactionSnapshot{
		ExternalID: externalID, ExternalAccountID: account, Description: strp(description),
		Amount: amountP(amount), AmountInAccountCurrency: amountP(amount), CurrencyCode: strp("BRL"),
		ProviderStatus: strp(status), MovementType: strp("DEBIT"),
	}
}

func newCategory(t *testing.T, conn *sql.DB, name string) string {
	t.Helper()
	category, err := categories.Create(context.Background(), conn, name, money.Expense, "tag", "#000000")
	if err != nil {
		t.Fatal(err)
	}
	return category.ID
}

func setManualInclusion(t *testing.T, conn *sql.DB, transactionID string, final money.InclusionState) {
	t.Helper()
	// Ignore then restore: the second call is the one that stores a manual
	// "considered" decision.
	states := []money.InclusionState{final}
	if final == money.Considered {
		states = []money.InclusionState{money.Ignored, money.Considered}
	}
	for _, state := range states {
		if _, err := transactions.SetInclusion(context.Background(), conn, transactionID, state, transactions.InclusionOriginManual, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
}

// Replaying the same provider payload changes nothing: no row, decision,
// event or counter is added, and automation is not re-run.
func TestSyncReplayIsIdempotent(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		lazer := newCategory(t, conn, "Lazer")
		if _, err := automation.Create(ctx, conn, automation.Write{
			Name: "Padaria é lazer", IsActive: true, LogicOperator: automation.LogicAnd,
			Conditions: []automation.Condition{{Field: automation.FieldDescription, Operator: automation.OperatorEquals, Value: "Padaria"}},
			Actions:    []automation.ActionWrite{{Type: automation.ActionSetCategory, CategoryID: &lazer}},
		}); err != nil {
			t.Fatal(err)
		}
		f := newIngestion(t, conn, "item-1", "acc-1")
		groceries := debit("tx-1", "acc-1", "Mercado", "-80.00", "POSTED")
		groceries.SourceCategory = strp("Groceries")
		payload := map[string][]pluggy.TransactionSnapshot{"acc-1": {
			groceries,
			debit("tx-2", "acc-1", "Padaria", "-12.00", "POSTED"),
			debit("tx-3", "acc-1", "Cinema", "-40.00", "POSTED"),
		}}
		first := f.run(t, payload)
		if status, _, inserted, updated := syncRunStatus(t, conn, first); status != "completed" || inserted != 3 || updated != 0 {
			t.Fatalf("first run status=%s inserted=%d updated=%d", status, inserted, updated)
		}
		manual := f.transactionID(t, "tx-3")
		if _, err := categories.AssignManual(ctx, conn, manual, lazer); err != nil {
			t.Fatal(err)
		}
		setManualInclusion(t, conn, manual, money.Ignored)

		before := ingestionState(t, conn)
		hookCallsBefore := f.hookCalls
		for i := 0; i < 2; i++ {
			replay := f.run(t, payload)
			if status, _, inserted, updated := syncRunStatus(t, conn, replay); status != "completed" || inserted != 0 || updated != 0 {
				t.Fatalf("replay %d status=%s inserted=%d updated=%d, want completed/0/0", i, status, inserted, updated)
			}
			if n := count(t, conn, `SELECT COUNT(*) FROM normalization_events WHERE sync_run_id = ? AND entity_type = 'transaction' AND outcome = 'unchanged'`, replay); n != 3 {
				t.Fatalf("replay %d unchanged events=%d, want 3", i, n)
			}
		}
		if after := ingestionState(t, conn); !reflect.DeepEqual(before, after) {
			t.Fatalf("replay changed state:\nbefore %v\nafter  %v", before, after)
		}
		if f.hookCalls != hookCallsBefore {
			t.Fatalf("automation ran %d times on replay", f.hookCalls-hookCallsBefore)
		}
		// Lançamentos carry no user note column, so there is no note for a
		// sync to overwrite.
		if _, err := conn.Exec(`SELECT notes FROM financial_transactions`); err == nil {
			t.Fatal("financial_transactions gained a notes column: add it to the preservation policy")
		}
	})
}

// The same account and transaction ids under two data sources are two
// independent rows, and decisions on one never reach the other.
func TestSyncSameExternalIDInTwoSourcesStaysSeparate(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		a := newIngestion(t, conn, "item-a", "acc-1")
		b := newIngestion(t, conn, "item-b", "acc-1")
		record := debit("tx-1", "acc-1", "Mercado", "-80.00", "POSTED")
		a.run(t, map[string][]pluggy.TransactionSnapshot{"acc-1": {record}})
		b.run(t, map[string][]pluggy.TransactionSnapshot{"acc-1": {record}})

		if n := count(t, conn, `SELECT COUNT(*) FROM financial_accounts`); n != 2 {
			t.Fatalf("%d accounts, want 2", n)
		}
		txA, txB := a.transactionID(t, "tx-1"), b.transactionID(t, "tx-1")
		if txA == txB || a.accountID(t, "acc-1") == b.accountID(t, "acc-1") {
			t.Fatal("sources share a row")
		}
		lazer := newCategory(t, conn, "Lazer")
		if _, err := categories.AssignManual(ctx, conn, txA, lazer); err != nil {
			t.Fatal(err)
		}
		setManualInclusion(t, conn, txA, money.Ignored)
		before := rows(t, conn, `SELECT description, normalized_hash, account_id FROM financial_transactions WHERE id = ?`, txA)

		corrected := record
		corrected.Description = strp("Mercado Central")
		run := b.run(t, map[string][]pluggy.TransactionSnapshot{"acc-1": {corrected}})
		if _, _, inserted, updated := syncRunStatus(t, conn, run); inserted != 0 || updated != 1 {
			t.Fatalf("source b inserted=%d updated=%d, want 0/1", inserted, updated)
		}
		if after := rows(t, conn, `SELECT description, normalized_hash, account_id FROM financial_transactions WHERE id = ?`, txA); !reflect.DeepEqual(before, after) {
			t.Fatalf("source b's update reached source a's row: %v -> %v", before, after)
		}
		var origin string
		if err := conn.QueryRow(`SELECT origin FROM transaction_category_decisions WHERE transaction_id = ?`, txA).Scan(&origin); err != nil || origin != "manual" {
			t.Fatalf("source a category origin=%s err=%v", origin, err)
		}
		for _, table := range []string{"transaction_category_decisions", "transaction_inclusion_decisions"} {
			if n := count(t, conn, `SELECT COUNT(*) FROM `+table+` WHERE transaction_id = ?`, txB); n != 0 {
				t.Fatalf("%s has %d rows for source b", table, n)
			}
		}
	})
}

// A source that reports a known transaction id under another of its
// accounts is refused: the stored row, its hash and the user's decisions stay
// as they were, and the rest of the page still imports.
func TestSyncSameExternalIDUnderAnotherAccountIsRejected(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		f := newIngestion(t, conn, "item-1", "acc-1", "acc-2")
		original := debit("tx-x", "acc-1", "Mercado", "-80.00", "POSTED")
		f.run(t, map[string][]pluggy.TransactionSnapshot{
			"acc-1": {original},
			"acc-2": {debit("tx-2a", "acc-2", "Farmácia", "-20.00", "POSTED")},
		})
		txID := f.transactionID(t, "tx-x")
		lazer := newCategory(t, conn, "Lazer")
		if _, err := categories.AssignManual(ctx, conn, txID, lazer); err != nil {
			t.Fatal(err)
		}
		before := rows(t, conn, `SELECT account_id, description, normalized_hash FROM financial_transactions WHERE id = ?`, txID)
		decisionsBefore := ingestionState(t, conn)["category decisions"]

		moved := debit("tx-x", "acc-2", "Mercado (outra conta)", "-80.00", "POSTED")
		run := f.run(t, map[string][]pluggy.TransactionSnapshot{
			"acc-2": {moved, debit("tx-2b", "acc-2", "Padaria", "-5.00", "POSTED")},
		})
		status, _, inserted, updated := syncRunStatus(t, conn, run)
		if status != "completed_with_failures" || inserted != 1 || updated != 0 {
			t.Fatalf("status=%s inserted=%d updated=%d, want completed_with_failures/1/0", status, inserted, updated)
		}
		var code, externalID string
		if err := conn.QueryRow(`SELECT error_code, external_transaction_id FROM sync_failures WHERE sync_run_id = ?`, run).Scan(&code, &externalID); err != nil {
			t.Fatal(err)
		}
		if code != "unsafe_account_association" || externalID != "tx-x" {
			t.Fatalf("failure %s on %s", code, externalID)
		}
		if n := count(t, conn, `SELECT COUNT(*) FROM normalization_events WHERE sync_run_id = ? AND external_id = 'tx-x' AND outcome <> 'rejected'`, run); n != 0 {
			t.Fatalf("%d non-rejected events for the refused record", n)
		}
		after := rows(t, conn, `SELECT account_id, description, normalized_hash FROM financial_transactions WHERE id = ?`, txID)
		if !reflect.DeepEqual(before, after) || after[0][0] != f.accountID(t, "acc-1") {
			t.Fatalf("refused record changed the stored row: %v -> %v", before, after)
		}
		if got := ingestionState(t, conn)["category decisions"]; !reflect.DeepEqual(decisionsBefore, got) {
			t.Fatalf("decisions changed: %v -> %v", decisionsBefore, got)
		}
		if n := count(t, conn, `SELECT COUNT(*) FROM financial_transactions WHERE external_id = 'tx-x'`); n != 1 {
			t.Fatalf("%d rows for tx-x", n)
		}
		f.transactionID(t, "tx-2b")
	})
}

// A provider must not rewrite a movement under another holding merely because
// it reused the same transaction id. The rejected movement can be retried
// under its original holding without changing its identity.
func TestSyncSameInvestmentTransactionIDUnderAnotherInvestmentIsRejected(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		f := newIngestion(t, conn, "item-1")
		f.provider.source.SafeProducts["INVESTMENTS"] = true
		run := func(records map[string][]pluggy.InvestmentTransactionSnapshot) string {
			t.Helper()
			runID := uuid.NewString()
			if _, err := conn.Exec(`INSERT INTO sync_runs (id, source_id, status, started_at) VALUES (?, ?, 'in_progress', ?)`,
				runID, f.sourceID, db.FormatTime(time.Now())); err != nil {
				t.Fatal(err)
			}
			investmentRaw, transactionRaw := uuid.NewString(), uuid.NewString()
			insertRawImport(t, conn, investmentRaw, runID, f.sourceID)
			insertRawImport(t, conn, transactionRaw, runID, f.sourceID)
			f.provider.investmentsPage = pluggy.InvestmentsPage{
				RawImportID: investmentRaw,
				Investments: []pluggy.InvestmentSnapshot{{ExternalID: "inv-1"}, {ExternalID: "inv-2"}},
			}
			f.provider.investmentTransactions = map[string]pluggy.InvestmentTransactionsPage{}
			for investmentID, movements := range records {
				f.provider.investmentTransactions[investmentID] = pluggy.InvestmentTransactionsPage{
					RawImportID: transactionRaw, Transactions: movements,
				}
			}
			if err := (&syncsvc.Service{DB: conn, Provider: f.provider, SyncRunID: runID, SourceID: f.sourceID}).Execute(context.Background()); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			return runID
		}
		original := pluggy.InvestmentTransactionSnapshot{
			ExternalID: "movement-1", ExternalInvestmentID: "inv-1", Amount: amountP("100"),
		}
		run(map[string][]pluggy.InvestmentTransactionSnapshot{"inv-1": {original}})
		before := rows(t, conn, `SELECT t.id, t.investment_id, t.amount, t.normalized_hash, t.current_raw_import_id
			FROM financial_investment_transactions t WHERE t.external_id = 'movement-1'`)

		conflicting := original
		conflicting.ExternalInvestmentID = "inv-2"
		conflicting.Amount = amountP("999")
		sibling := pluggy.InvestmentTransactionSnapshot{
			ExternalID: "movement-2", ExternalInvestmentID: "inv-2", Amount: amountP("20"),
		}
		rejectedRun := run(map[string][]pluggy.InvestmentTransactionSnapshot{"inv-2": {conflicting, sibling}})
		if after := rows(t, conn, `SELECT t.id, t.investment_id, t.amount, t.normalized_hash, t.current_raw_import_id
			FROM financial_investment_transactions t WHERE t.external_id = 'movement-1'`); !reflect.DeepEqual(before, after) {
			t.Fatalf("conflicting investment changed movement: %v -> %v", before, after)
		}
		if n := count(t, conn, `SELECT COUNT(*) FROM financial_investment_transactions WHERE external_id = 'movement-2'`); n != 1 {
			t.Fatalf("sibling movement count = %d, want 1", n)
		}
		var code, status string
		if err := conn.QueryRow(`SELECT error_code FROM sync_failures WHERE sync_run_id = ?`, rejectedRun).Scan(&code); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRow(`SELECT status FROM sync_runs WHERE id = ?`, rejectedRun).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if code != "unsafe_investment_association" || status != "completed_with_failures" {
			t.Fatalf("rejection code/status = %s/%s", code, status)
		}
		if n := count(t, conn, `SELECT COUNT(*) FROM normalization_events WHERE sync_run_id = ? AND entity_type = 'investment_transaction' AND outcome = 'rejected'`, rejectedRun); n != 1 {
			t.Fatalf("rejected normalization events = %d, want 1", n)
		}
		run(map[string][]pluggy.InvestmentTransactionSnapshot{"inv-1": {original}})
		if after := rows(t, conn, `SELECT t.id, t.investment_id, t.amount, t.normalized_hash, t.current_raw_import_id
			FROM financial_investment_transactions t WHERE t.external_id = 'movement-1'`); !reflect.DeepEqual(before, after) {
			t.Fatalf("replay changed original movement: %v -> %v", before, after)
		}
	})
}

// PENDING -> POSTED (and any provider correction) keeps the transaction id and
// rewrites provider fields only: manual category and inclusion, payable links
// and realizations are left exactly as the user set them.
func TestSyncPendingToPostedKeepsIdentityAndUserDecisions(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		f := newIngestion(t, conn, "item-1", "acc-1")
		pending := debit("tx-1", "acc-1", "Compra pendente", "-50.00", "PENDING")
		f.run(t, map[string][]pluggy.TransactionSnapshot{"acc-1": {pending}})
		txID := f.transactionID(t, "tx-1")

		lazer := newCategory(t, conn, "Lazer")
		if _, err := categories.AssignManual(ctx, conn, txID, lazer); err != nil {
			t.Fatal(err)
		}
		setManualInclusion(t, conn, txID, money.Considered)
		debt, err := payables.Create(ctx, conn, payables.KindDebt, "Empréstimo", decimal.RequireFromString("500"), decimal.RequireFromString("500"))
		if err != nil {
			t.Fatal(err)
		}
		link, err := payables.CreateLink(ctx, conn, payables.KindDebt, debt.ID, txID)
		if err != nil || link.Status != payables.StatusCreated {
			t.Fatalf("CreateLink: %+v %v", link, err)
		}
		userOwned := func() map[string]any {
			return map[string]any{
				"category":     rows(t, conn, `SELECT transaction_id, category_id, origin FROM transaction_category_decisions WHERE transaction_id = ?`, txID),
				"inclusion":    rows(t, conn, `SELECT transaction_id, state, origin FROM transaction_inclusion_decisions WHERE transaction_id = ?`, txID),
				"link":         rows(t, conn, `SELECT id, payable_id, linked_amount FROM payable_transaction_links WHERE transaction_id = ?`, txID),
				"realizations": count(t, conn, `SELECT COUNT(*) FROM scenario_realizations WHERE transaction_id = ?`, txID),
				"cat events":   count(t, conn, `SELECT COUNT(*) FROM transaction_category_events WHERE transaction_id = ?`, txID),
				"inc events":   count(t, conn, `SELECT COUNT(*) FROM transaction_inclusion_events WHERE transaction_id = ?`, txID),
			}
		}
		before := userOwned()

		posted := debit("tx-1", "acc-1", "Compra confirmada", "-55.00", "POSTED")
		run := f.run(t, map[string][]pluggy.TransactionSnapshot{"acc-1": {posted}})
		if status, _, inserted, updated := syncRunStatus(t, conn, run); status != "completed" || inserted != 0 || updated != 1 {
			t.Fatalf("status=%s inserted=%d updated=%d, want completed/0/1", status, inserted, updated)
		}
		if n := count(t, conn, `SELECT COUNT(*) FROM financial_transactions`); n != 1 || f.transactionID(t, "tx-1") != txID {
			t.Fatalf("%d rows; the posted record must keep id %s", n, txID)
		}
		var description, amount, status, hash string
		if err := conn.QueryRow(`SELECT description, amount, provider_status, normalized_hash FROM financial_transactions WHERE id = ?`, txID).
			Scan(&description, &amount, &status, &hash); err != nil {
			t.Fatal(err)
		}
		if description != "Compra confirmada" || amount != "-55.00" || status != "POSTED" || hash != pluggy.TransactionHash(posted) {
			t.Fatalf("provider fields not updated: %s %s %s", description, amount, status)
		}
		if after := userOwned(); !reflect.DeepEqual(before, after) {
			t.Fatalf("user-owned data changed:\nbefore %v\nafter  %v", before, after)
		}
	})
}

// Derived decisions are written on insert: a correction does not re-derive
// the automatic category from the new source category. Automation re-runs on
// the update, so an active rule may now apply, but never over a manual choice.
func TestSyncProviderCorrectionKeepsAutomaticDecisionButRuleMayReapply(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		f := newIngestion(t, conn, "item-1", "acc-1")
		auto := debit("tx-auto", "acc-1", "Mercado", "-80.00", "POSTED")
		auto.SourceCategory = strp("Groceries")
		manual := debit("tx-manual", "acc-1", "Loja", "-30.00", "POSTED")
		ruled := debit("tx-rule", "acc-1", "Loja", "-10.00", "POSTED")
		f.run(t, map[string][]pluggy.TransactionSnapshot{"acc-1": {auto, manual, ruled}})

		lazer := newCategory(t, conn, "Lazer")
		viagem := newCategory(t, conn, "Viagem")
		if _, err := categories.AssignManual(ctx, conn, f.transactionID(t, "tx-manual"), lazer); err != nil {
			t.Fatal(err)
		}
		if _, err := automation.Create(ctx, conn, automation.Write{
			Name: "Hotel é viagem", IsActive: true, LogicOperator: automation.LogicAnd,
			Conditions: []automation.Condition{{Field: automation.FieldDescription, Operator: automation.OperatorEquals, Value: "Hotel"}},
			Actions:    []automation.ActionWrite{{Type: automation.ActionSetCategory, CategoryID: &viagem}},
		}); err != nil {
			t.Fatal(err)
		}

		auto.SourceCategory = strp("Eating out")
		manual.Description = strp("Hotel")
		ruled.Description = strp("Hotel")
		run := f.run(t, map[string][]pluggy.TransactionSnapshot{"acc-1": {auto, manual, ruled}})
		if _, _, inserted, updated := syncRunStatus(t, conn, run); inserted != 0 || updated != 3 {
			t.Fatalf("inserted=%d updated=%d, want 0/3", inserted, updated)
		}
		for externalID, want := range map[string][2]string{
			"tx-auto":   {categories.SourceCategoryMapping["Groceries"], "automatic"},
			"tx-manual": {lazer, "manual"},
			"tx-rule":   {viagem, "rule"},
		} {
			var category, origin string
			if err := conn.QueryRow(`SELECT category_id, origin FROM transaction_category_decisions WHERE transaction_id = ?`, f.transactionID(t, externalID)).
				Scan(&category, &origin); err != nil {
				t.Fatalf("%s: %v", externalID, err)
			}
			if category != want[0] || origin != want[1] {
				t.Errorf("%s category=%s origin=%s, want %s/%s", externalID, category, origin, want[0], want[1])
			}
		}
	})
}
