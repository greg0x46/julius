package syncsvc_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/automation"
	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/pluggy"
	"github.com/greg0x46/julius/internal/syncsvc"
	"github.com/greg0x46/julius/internal/transactions"
)

// retryFixture runs an account-only sync, then supplies distinct run/import
// audit records for each attempted transaction sync.
type retryFixture struct {
	conn     *sql.DB
	sourceID string
	provider *fakeProvider
}

func newRetryFixture(t *testing.T, conn *sql.DB) retryFixture {
	t.Helper()
	sourceID, runID := newSyncRun(t, conn)
	insertRawImport(t, conn, "raw-accounts", runID, sourceID)
	provider := &fakeProvider{
		source: defaultSource(),
		accountsPage: pluggy.AccountsPage{
			RawImportID: "raw-accounts",
			Accounts:    []pluggy.AccountSnapshot{{ExternalID: "acc-1", CurrencyCode: strp("BRL")}},
		},
		transactionPages: make(map[string][]pluggy.TransactionsPage),
	}
	if err := (&syncsvc.Service{DB: conn, Provider: provider, SyncRunID: runID, SourceID: sourceID}).Execute(context.Background()); err != nil {
		t.Fatalf("account sync: %v", err)
	}
	return retryFixture{conn: conn, sourceID: sourceID, provider: provider}
}

func (f retryFixture) sync(t *testing.T, records []pluggy.TransactionSnapshot, hook transactions.UpsertedHook) string {
	t.Helper()
	runID := uuid.NewString()
	if _, err := f.conn.Exec(`INSERT INTO sync_runs (id, source_id, status, started_at) VALUES (?, ?, 'in_progress', ?)`,
		runID, f.sourceID, db.FormatTime(time.Now())); err != nil {
		t.Fatalf("insert sync run: %v", err)
	}
	rawID := uuid.NewString()
	insertRawImport(t, f.conn, rawID, runID, f.sourceID)
	f.provider.transactionPages["acc-1"] = []pluggy.TransactionsPage{{RawImportID: rawID, Transactions: records}}
	if err := (&syncsvc.Service{
		DB: f.conn, Provider: f.provider, SyncRunID: runID, SourceID: f.sourceID, OnTransactionUpserted: hook,
	}).Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return runID
}

func assertRetryRun(t *testing.T, conn *sql.DB, runID, wantStatus string, wantInserted, wantUpdated, wantRejected int) {
	t.Helper()
	status, _, inserted, updated := syncRunStatus(t, conn, runID)
	if status != wantStatus || inserted != wantInserted || updated != wantUpdated {
		t.Fatalf("run status=%s inserted=%d updated=%d, want %s/%d/%d", status, inserted, updated, wantStatus, wantInserted, wantUpdated)
	}
	var rejected, failures int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM normalization_events WHERE sync_run_id = ? AND entity_type = 'transaction' AND outcome = 'rejected'`, runID).Scan(&rejected); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`SELECT COUNT(*) FROM sync_failures WHERE sync_run_id = ?`, runID).Scan(&failures); err != nil {
		t.Fatal(err)
	}
	if rejected != wantRejected || failures != wantRejected {
		t.Fatalf("rejected=%d failures=%d, want %d each", rejected, failures, wantRejected)
	}
	var normalized int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM normalization_events WHERE sync_run_id = ? AND entity_type = 'transaction' AND outcome IN ('inserted', 'updated')`, runID).Scan(&normalized); err != nil {
		t.Fatal(err)
	}
	if normalized != wantInserted+wantUpdated {
		t.Fatalf("normalization events=%d, want %d", normalized, wantInserted+wantUpdated)
	}
}

func TestExecuteRetriesFailedInsertWithSamePayload(t *testing.T) {
	for _, backend := range dbtest.Backends() {
		t.Run(backend.Name, func(t *testing.T) { testExecuteRetriesFailedInsert(t, backend.Open) })
	}
}

func testExecuteRetriesFailedInsert(t *testing.T, open func(*testing.T) *sql.DB) {
	for _, stage := range []string{"card_payment", "source_category", "learned_category", "automation"} {
		t.Run(stage, func(t *testing.T) {
			f := newRetryFixture(t, open(t))
			ctx := context.Background()
			target := pluggy.TransactionSnapshot{
				ExternalID: "target", ExternalAccountID: "acc-1", Description: strp("Target"),
				Amount: amountP("-42.50"), MovementType: strp("DEBIT"), ProviderStatus: strp("POSTED"),
				SourceCategory: strp("Groceries"),
			}
			wantCategory := categories.SourceCategoryMapping["Groceries"]
			wantOrigin := "automatic"
			faultOrigin := "automatic"
			hookCalls := 0
			hook := automation.NewTransactionHook(nil)
			switch stage {
			case "card_payment":
				target.OperationTypeAdditionalInfo = strp("PAGAMENTO_FATURA")
				wantCategory = categories.CardPaymentCategoryID
			case "learned_category":
				var accountID string
				if err := f.conn.QueryRow(`SELECT id FROM financial_accounts WHERE external_id = 'acc-1'`).Scan(&accountID); err != nil {
					t.Fatal(err)
				}
				refID, err := transactions.CreateManual(ctx, f.conn, transactions.ManualInput{
					AccountID: accountID, Description: "Target", Amount: decimal.RequireFromString("-1"), OccurredAt: time.Now(),
				})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := categories.AssignManual(ctx, f.conn, refID, categories.CardPaymentCategoryID); err != nil {
					t.Fatal(err)
				}
				wantCategory, wantOrigin, faultOrigin = categories.CardPaymentCategoryID, "learned", "learned"
			case "automation":
				if _, err := automation.Create(ctx, f.conn, automation.Write{
					Name: "Categorizar e ignorar Target", IsActive: true, LogicOperator: automation.LogicAnd,
					Conditions: []automation.Condition{{Field: automation.FieldDescription, Operator: automation.OperatorEquals, Value: "Target"}},
					Actions: []automation.ActionWrite{
						{Type: automation.ActionSetCategory, CategoryID: strp(categories.CardPaymentCategoryID)},
						{Type: automation.ActionIgnore},
					},
				}); err != nil {
					t.Fatal(err)
				}
				hook = automation.NewTransactionHook(func(context.Context, transactions.Querier, string) error {
					hookCalls++
					if hookCalls == 1 {
						return errors.New("injected unlink failure")
					}
					return nil
				})
				wantCategory, wantOrigin = categories.CardPaymentCategoryID, "rule"
			}
			dropFault := func() {}
			if stage != "automation" {
				// Fail after the decision write, exposing partial category/events
				// persistence as well as loss of retry on an already-stored hash.
				dropFault = dbtest.FailInserts(t, f.conn, "transaction_category_events",
					`NEW.origin = '`+faultOrigin+`' AND NEW.transaction_id IN (SELECT id FROM financial_transactions WHERE external_id = 'target')`)
			}
			neighbor := pluggy.TransactionSnapshot{ExternalID: "neighbor", ExternalAccountID: "acc-1", Description: strp("Neighbor"), MovementType: strp("DEBIT")}
			records := []pluggy.TransactionSnapshot{target, neighbor}
			failedRun := f.sync(t, records, hook)
			assertRetryRun(t, f.conn, failedRun, "completed_with_failures", 1, 0, 1)
			var count int
			if err := f.conn.QueryRow(`SELECT COUNT(*) FROM financial_transactions WHERE external_id = 'target'`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatal("failed insert persisted the target")
			}
			// Reference manual decisions remain; no target decision/event may
			// survive. FK checks also make leaked events without a row impossible.
			for _, table := range []string{"transaction_category_decisions", "transaction_category_events", "transaction_inclusion_decisions", "transaction_inclusion_events"} {
				if err := f.conn.QueryRow(`SELECT COUNT(*) FROM ` + table + ` d JOIN financial_transactions ft ON ft.id = d.transaction_id WHERE ft.origin = 'synced'`).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatalf("%s retained %d partial records", table, count)
				}
			}
			dropFault()
			retryRun := f.sync(t, records, hook)
			assertRetryRun(t, f.conn, retryRun, "completed", 1, 0, 0)
			var category, origin string
			if err := f.conn.QueryRow(`SELECT d.category_id, d.origin FROM transaction_category_decisions d JOIN financial_transactions ft ON ft.id = d.transaction_id WHERE ft.external_id = 'target'`).Scan(&category, &origin); err != nil {
				t.Fatal(err)
			}
			if category != wantCategory || origin != wantOrigin {
				t.Fatalf("category=%s origin=%s, want %s/%s", category, origin, wantCategory, wantOrigin)
			}
			unchangedRun := f.sync(t, records, hook)
			assertRetryRun(t, f.conn, unchangedRun, "completed", 0, 0, 0)
			if stage == "automation" && hookCalls != 2 {
				t.Fatalf("unlink calls=%d, want failed insert + successful retry only", hookCalls)
			}
		})
	}
}

func TestExecuteRetriesFailedUpdateWithSamePayload(t *testing.T) {
	dbtest.Each(t, testExecuteRetriesFailedUpdate)
}

func testExecuteRetriesFailedUpdate(t *testing.T, conn *sql.DB) {
	f := newRetryFixture(t, conn)
	ctx := context.Background()
	initial := pluggy.TransactionSnapshot{
		ExternalID: "target", ExternalAccountID: "acc-1", Description: strp("Original"),
		Amount: amountP("-42.50"), MovementType: strp("DEBIT"), ProviderStatus: strp("PENDING"),
		SourceCategory: strp("Groceries"),
	}
	f.sync(t, []pluggy.TransactionSnapshot{initial}, nil)
	if _, err := automation.Create(ctx, f.conn, automation.Write{
		Name: "Categorizar e ignorar Target", IsActive: true, LogicOperator: automation.LogicAnd,
		Conditions: []automation.Condition{{Field: automation.FieldDescription, Operator: automation.OperatorEquals, Value: "Target"}},
		Actions: []automation.ActionWrite{
			{Type: automation.ActionSetCategory, CategoryID: strp(categories.CardPaymentCategoryID)},
			{Type: automation.ActionIgnore},
		},
	}); err != nil {
		t.Fatal(err)
	}
	updated := initial
	updated.Description, updated.ProviderStatus = strp("Target"), strp("POSTED")
	var transactionID, originalHash, originalRawID string
	if err := f.conn.QueryRow(`SELECT id, normalized_hash, current_raw_import_id FROM financial_transactions WHERE external_id = 'target'`).Scan(&transactionID, &originalHash, &originalRawID); err != nil {
		t.Fatal(err)
	}
	failingHook := automation.NewTransactionHook(func(context.Context, transactions.Querier, string) error {
		return errors.New("injected unlink failure")
	})
	failedRun := f.sync(t, []pluggy.TransactionSnapshot{updated}, failingHook)
	assertRetryRun(t, f.conn, failedRun, "completed_with_failures", 0, 0, 1)
	var hash, rawID, description, category, origin string
	if err := f.conn.QueryRow(`SELECT ft.normalized_hash, ft.current_raw_import_id, ft.description, d.category_id, d.origin
		FROM financial_transactions ft JOIN transaction_category_decisions d ON d.transaction_id = ft.id WHERE ft.id = ?`, transactionID).
		Scan(&hash, &rawID, &description, &category, &origin); err != nil {
		t.Fatal(err)
	}
	if hash != originalHash || rawID != originalRawID || description != "Original" || category != categories.SourceCategoryMapping["Groceries"] || origin != "automatic" {
		t.Fatalf("failed update changed original data or category: hash=%s raw=%s description=%s category=%s origin=%s", hash, rawID, description, category, origin)
	}
	for table, want := range map[string]int{"transaction_category_events": 1, "transaction_inclusion_decisions": 0, "transaction_inclusion_events": 0} {
		var count int
		if err := f.conn.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE transaction_id = ?`, transactionID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != want {
			t.Fatalf("%s records=%d, want %d", table, count, want)
		}
	}
	// User choices made before retry must still outrank the rule.
	manualCategory := categories.SourceCategoryMapping["Groceries"]
	if _, err := categories.AssignManual(ctx, f.conn, transactionID, manualCategory); err != nil {
		t.Fatal(err)
	}
	for _, state := range []money.InclusionState{money.Ignored, money.Considered} {
		if _, err := transactions.SetInclusion(ctx, f.conn, transactionID, state, transactions.InclusionOriginManual, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	hookCalls := 0
	ruleHook := automation.NewTransactionHook(nil)
	hook := func(ctx context.Context, q transactions.Querier, transactionID, accountID string) error {
		hookCalls++
		return ruleHook(ctx, q, transactionID, accountID)
	}
	retryRun := f.sync(t, []pluggy.TransactionSnapshot{updated}, hook)
	assertRetryRun(t, f.conn, retryRun, "completed", 0, 1, 0)
	var state, inclusionOrigin string
	if err := f.conn.QueryRow(`SELECT ft.normalized_hash, ft.description, cd.category_id, cd.origin, i.state, i.origin
		FROM financial_transactions ft JOIN transaction_category_decisions cd ON cd.transaction_id = ft.id
		JOIN transaction_inclusion_decisions i ON i.transaction_id = ft.id WHERE ft.id = ?`, transactionID).
		Scan(&hash, &description, &category, &origin, &state, &inclusionOrigin); err != nil {
		t.Fatal(err)
	}
	if hash != pluggy.TransactionHash(updated) || description != "Target" || category != manualCategory || origin != "manual" || state != "considered" || inclusionOrigin != "manual" {
		t.Fatalf("retry did not apply update and preserve manual choices: hash=%s description=%s category=%s origin=%s inclusion=%s/%s", hash, description, category, origin, state, inclusionOrigin)
	}
	unchangedRun := f.sync(t, []pluggy.TransactionSnapshot{updated}, hook)
	assertRetryRun(t, f.conn, unchangedRun, "completed", 0, 0, 0)
	if hookCalls != 1 {
		t.Fatalf("hook calls=%d, want successful update only", hookCalls)
	}
}
