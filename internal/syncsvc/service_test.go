package syncsvc_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/automation"
	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/pluggy"
	"github.com/greg0x46/julius/internal/syncsvc"
	"github.com/greg0x46/julius/internal/transactions"
)

type fakeProvider struct {
	source            pluggy.SourceSnapshot
	sourceRawImportID string
	sourceErr         error
	accountsPage      pluggy.AccountsPage
	accountsErr       error
	transactionPages  map[string][]pluggy.TransactionsPage
	iterErr           error

	investmentsPage        pluggy.InvestmentsPage
	investmentsErr         error
	investmentTransactions map[string]pluggy.InvestmentTransactionsPage
	investmentTxErr        error

	bills    map[string]pluggy.BillsPage
	billsErr error
}

func (f *fakeProvider) GetSource(context.Context) (pluggy.SourceSnapshot, string, error) {
	return f.source, f.sourceRawImportID, f.sourceErr
}

func (f *fakeProvider) GetAccounts(context.Context) (pluggy.AccountsPage, error) {
	return f.accountsPage, f.accountsErr
}

func (f *fakeProvider) IterTransactionPages(_ context.Context, externalAccountID string, handle func(pluggy.TransactionsPage) error) error {
	for _, page := range f.transactionPages[externalAccountID] {
		if err := handle(page); err != nil {
			return err
		}
	}
	return f.iterErr
}

func (f *fakeProvider) GetInvestments(context.Context) (pluggy.InvestmentsPage, error) {
	return f.investmentsPage, f.investmentsErr
}

func (f *fakeProvider) GetInvestmentTransactions(_ context.Context, externalInvestmentID string) (pluggy.InvestmentTransactionsPage, error) {
	return f.investmentTransactions[externalInvestmentID], f.investmentTxErr
}

func (f *fakeProvider) GetBills(_ context.Context, externalAccountID string) (pluggy.BillsPage, error) {
	return f.bills[externalAccountID], f.billsErr
}

func defaultSource() pluggy.SourceSnapshot {
	return pluggy.SourceSnapshot{
		ExternalItemID: "item-1",
		SafeProducts:   map[string]bool{"ACCOUNTS": true, "TRANSACTIONS": true},
	}
}

func newTestConn(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// newSyncRun inserts the data_sources + sync_runs rows a Service needs to
// already exist (created by the HTTP layer's POST /api/sync-runs in
// production), returning the ids Service needs.
func newSyncRun(t *testing.T, conn *sql.DB) (sourceID, syncRunID string) {
	t.Helper()
	sourceID, syncRunID = uuid.NewString(), uuid.NewString()
	now := db.FormatTime(time.Now())
	if _, err := conn.Exec(`INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at)
		VALUES (?, 'pluggy', 'item-1', ?, ?)`, sourceID, now, now); err != nil {
		t.Fatalf("insert data_source: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO sync_runs (id, source_id, status, started_at) VALUES (?, ?, 'in_progress', ?)`,
		syncRunID, sourceID, now); err != nil {
		t.Fatalf("insert sync_run: %v", err)
	}
	return sourceID, syncRunID
}

// insertRawImport satisfies the current_raw_import_id/raw_import_id foreign
// keys financial_accounts/financial_transactions/normalization_events/
// sync_failures all carry: in production these ids always come from a real
// RawImportWriter.Write call, so tests that fabricate a raw import id must
// give it a backing row too.
var rawImportSequence int

func insertRawImport(t *testing.T, conn *sql.DB, id, syncRunID, sourceID string) {
	t.Helper()
	if id == "" {
		return
	}
	rawImportSequence++
	now := db.FormatTime(time.Now())
	_, err := conn.Exec(`INSERT INTO raw_imports (
			id, sync_run_id, source_id, scope, page_sequence, request_attempt,
			request_method, request_path, http_status, response_headers, payload,
			payload_sha256, received_at
		) VALUES (?, ?, ?, 'item', ?, 1, 'GET', '/x', 200, '{}', ?, 'sha', ?)`,
		id, syncRunID, sourceID, rawImportSequence, []byte{0}, now)
	if err != nil {
		t.Fatalf("insert raw_import %s: %v", id, err)
	}
}

func syncRunStatus(t *testing.T, conn *sql.DB, syncRunID string) (status string, accountsProcessed, inserted, updated int) {
	t.Helper()
	err := conn.QueryRow(`SELECT status, accounts_processed, transactions_inserted, transactions_updated FROM sync_runs WHERE id = ?`, syncRunID).
		Scan(&status, &accountsProcessed, &inserted, &updated)
	if err != nil {
		t.Fatalf("query sync_run: %v", err)
	}
	return
}

func amountP(s string) *decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return &d
}

func strp(s string) *string { return &s }

func TestExecuteHappyPathInsertsAccountAndTransaction(t *testing.T) {
	conn := newTestConn(t)
	sourceID, syncRunID := newSyncRun(t, conn)
	insertRawImport(t, conn, "raw-item", syncRunID, sourceID)
	insertRawImport(t, conn, "raw-accounts", syncRunID, sourceID)
	insertRawImport(t, conn, "raw-tx-1", syncRunID, sourceID)

	provider := &fakeProvider{
		source:            defaultSource(),
		sourceRawImportID: "raw-item",
		accountsPage: pluggy.AccountsPage{
			RawImportID: "raw-accounts",
			Accounts:    []pluggy.AccountSnapshot{{ExternalID: "acc-1", CurrencyCode: strp("BRL")}},
		},
		transactionPages: map[string][]pluggy.TransactionsPage{
			"acc-1": {{
				RawImportID: "raw-tx-1",
				Transactions: []pluggy.TransactionSnapshot{{
					ExternalID: "tx-1", ExternalAccountID: "acc-1", Amount: amountP("-42.50"),
					AmountInAccountCurrency: amountP("-42.50"), CurrencyCode: strp("BRL"),
					ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT"), SourceCategory: strp("Groceries"),
				}},
			}},
		},
	}

	service := &syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID, SourceID: sourceID}
	if err := service.Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	status, accountsProcessed, inserted, updated := syncRunStatus(t, conn, syncRunID)
	if status != "completed" || accountsProcessed != 1 || inserted != 1 || updated != 0 {
		t.Errorf("status=%s accounts=%d inserted=%d updated=%d", status, accountsProcessed, inserted, updated)
	}

	var accountCount, txCount int
	conn.QueryRow(`SELECT COUNT(*) FROM financial_accounts`).Scan(&accountCount)
	conn.QueryRow(`SELECT COUNT(*) FROM financial_transactions`).Scan(&txCount)
	if accountCount != 1 || txCount != 1 {
		t.Errorf("accountCount=%d txCount=%d", accountCount, txCount)
	}

	// "Groceries" maps to Supermercado in categories.SourceCategoryMapping,
	// so the automatic categorization hook should have already run.
	var categoryID, origin string
	err := conn.QueryRow(`
		SELECT tcd.category_id, tcd.origin FROM transaction_category_decisions tcd
		JOIN financial_transactions ft ON ft.id = tcd.transaction_id
		WHERE ft.external_id = 'tx-1'`).Scan(&categoryID, &origin)
	if err != nil {
		t.Fatalf("query category decision: %v", err)
	}
	if categoryID != "000433b6-3094-5a9c-87df-465b70574a4b" || origin != "automatic" {
		t.Errorf("category=%s origin=%s", categoryID, origin)
	}
}

func TestExecutePersistsCreditAccountLimitsAndDates(t *testing.T) {
	conn := newTestConn(t)
	sourceID, syncRunID := newSyncRun(t, conn)
	insertRawImport(t, conn, "raw-accounts", syncRunID, sourceID)

	closeDate := time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC)
	dueDate := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	provider := &fakeProvider{
		source: defaultSource(),
		accountsPage: pluggy.AccountsPage{
			RawImportID: "raw-accounts",
			Accounts: []pluggy.AccountSnapshot{{
				ExternalID: "acc-1", AccountType: strp("CREDIT"), CurrencyCode: strp("BRL"),
				Balance: amountP("1234.56"), CreditLimit: amountP("5000.00"),
				AvailableCreditLimit: amountP("3765.44"),
				BalanceCloseDate:     &closeDate, BalanceDueDate: &dueDate,
			}},
		},
	}
	if err := (&syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID, SourceID: sourceID}).Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var available, closeRaw, dueRaw string
	err := conn.QueryRow(`SELECT available_credit_limit, balance_close_date, balance_due_date
		FROM financial_accounts WHERE external_id = 'acc-1'`).Scan(&available, &closeRaw, &dueRaw)
	if err != nil {
		t.Fatalf("query account: %v", err)
	}
	if available != "3765.44" {
		t.Errorf("available_credit_limit = %s", available)
	}
	if closeRaw != db.FormatTime(closeDate) || dueRaw != db.FormatTime(dueDate) {
		t.Errorf("close=%s due=%s", closeRaw, dueRaw)
	}
}

func TestExecuteSecondRunDetectsUnchangedRecords(t *testing.T) {
	conn := newTestConn(t)
	sourceID, syncRunID1 := newSyncRun(t, conn)
	insertRawImport(t, conn, "raw-accounts", syncRunID1, sourceID)
	insertRawImport(t, conn, "raw-tx-1", syncRunID1, sourceID)

	provider := &fakeProvider{
		source: defaultSource(),
		accountsPage: pluggy.AccountsPage{
			RawImportID: "raw-accounts",
			Accounts:    []pluggy.AccountSnapshot{{ExternalID: "acc-1", CurrencyCode: strp("BRL")}},
		},
		transactionPages: map[string][]pluggy.TransactionsPage{
			"acc-1": {{
				RawImportID: "raw-tx-1",
				Transactions: []pluggy.TransactionSnapshot{{
					ExternalID: "tx-1", ExternalAccountID: "acc-1", Amount: amountP("-1.00"),
					ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT"),
				}},
			}},
		},
	}
	if err := (&syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID1, SourceID: sourceID}).Execute(context.Background()); err != nil {
		t.Fatalf("first Execute: %v", err)
	}

	now := db.FormatTime(time.Now())
	syncRunID2 := uuid.NewString()
	if _, err := conn.Exec(`INSERT INTO sync_runs (id, source_id, status, started_at) VALUES (?, ?, 'in_progress', ?)`, syncRunID2, sourceID, now); err != nil {
		t.Fatalf("insert second sync_run: %v", err)
	}
	if err := (&syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID2, SourceID: sourceID}).Execute(context.Background()); err != nil {
		t.Fatalf("second Execute: %v", err)
	}

	status, accountsProcessed, inserted, updated := syncRunStatus(t, conn, syncRunID2)
	if status != "completed" || accountsProcessed != 1 || inserted != 0 || updated != 0 {
		t.Errorf("second run: status=%s accounts=%d inserted=%d updated=%d, want completed/1/0/0", status, accountsProcessed, inserted, updated)
	}

	var txCount int
	conn.QueryRow(`SELECT COUNT(*) FROM financial_transactions`).Scan(&txCount)
	if txCount != 1 {
		t.Errorf("txCount = %d, want 1 (no duplicate insert)", txCount)
	}
}

// TestExecuteResyncReevaluatesAutomationRuleOnUpdatedTransaction covers the
// gap OnTransactionUpserted closes: a transaction that matches no active
// rule when first inserted (e.g. a PENDING installment with no card number
// yet) must still get evaluated once a later resync updates it with data
// that now matches (e.g. Pluggy filling in credit_card_metadata once the
// installment posts) — without anyone running "apply retroactively" by hand.
func TestExecuteResyncReevaluatesAutomationRuleOnUpdatedTransaction(t *testing.T) {
	conn := newTestConn(t)
	ctx := context.Background()
	sourceID, syncRunID1 := newSyncRun(t, conn)
	insertRawImport(t, conn, "raw-accounts", syncRunID1, sourceID)
	insertRawImport(t, conn, "raw-tx-1", syncRunID1, sourceID)

	if _, err := automation.Create(ctx, conn, automation.Write{
		Name: "Ignorar cartão final 4321", IsActive: true, LogicOperator: automation.LogicAnd,
		Conditions: []automation.Condition{{Field: automation.FieldCard, Operator: automation.OperatorContains, Value: "4321"}},
		Actions:    []automation.ActionWrite{{Type: automation.ActionIgnore}},
	}); err != nil {
		t.Fatalf("Create rule: %v", err)
	}

	provider := &fakeProvider{
		source: defaultSource(),
		accountsPage: pluggy.AccountsPage{
			RawImportID: "raw-accounts",
			Accounts:    []pluggy.AccountSnapshot{{ExternalID: "acc-1", CurrencyCode: strp("BRL")}},
		},
		transactionPages: map[string][]pluggy.TransactionsPage{
			"acc-1": {{
				RawImportID: "raw-tx-1",
				Transactions: []pluggy.TransactionSnapshot{{
					ExternalID: "tx-1", ExternalAccountID: "acc-1", Amount: amountP("-1.00"),
					ProviderStatus: strp("PENDING"), MovementType: strp("DEBIT"),
				}},
			}},
		},
	}
	service1 := &syncsvc.Service{
		DB: conn, Provider: provider, SyncRunID: syncRunID1, SourceID: sourceID,
		OnTransactionUpserted: automation.NewTransactionHook(nil),
	}
	if err := service1.Execute(ctx); err != nil {
		t.Fatalf("first Execute: %v", err)
	}

	var preCount int
	conn.QueryRow(`SELECT COUNT(*) FROM transaction_inclusion_decisions`).Scan(&preCount)
	if preCount != 0 {
		t.Fatalf("expected no inclusion decision before the card number is known, got %d", preCount)
	}

	// Resync: the installment posts and Pluggy now reports its card number,
	// so the same transaction's normalized hash changes (outcome=updated).
	now := db.FormatTime(time.Now())
	syncRunID2 := uuid.NewString()
	if _, err := conn.Exec(`INSERT INTO sync_runs (id, source_id, status, started_at) VALUES (?, ?, 'in_progress', ?)`, syncRunID2, sourceID, now); err != nil {
		t.Fatalf("insert second sync_run: %v", err)
	}
	insertRawImport(t, conn, "raw-tx-2", syncRunID2, sourceID)
	provider.transactionPages["acc-1"] = []pluggy.TransactionsPage{{
		RawImportID: "raw-tx-2",
		Transactions: []pluggy.TransactionSnapshot{{
			ExternalID: "tx-1", ExternalAccountID: "acc-1", Amount: amountP("-1.00"),
			ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT"),
			CreditCardMetadata: []byte(`{"cardNumber":"**** 4321"}`),
		}},
	}}
	service2 := &syncsvc.Service{
		DB: conn, Provider: provider, SyncRunID: syncRunID2, SourceID: sourceID,
		OnTransactionUpserted: automation.NewTransactionHook(nil),
	}
	if err := service2.Execute(ctx); err != nil {
		t.Fatalf("second Execute: %v", err)
	}

	status, _, inserted, updated := syncRunStatus(t, conn, syncRunID2)
	if status != "completed" || inserted != 0 || updated != 1 {
		t.Errorf("second run: status=%s inserted=%d updated=%d, want completed/0/1", status, inserted, updated)
	}

	var state, origin string
	err := conn.QueryRow(`
		SELECT tid.state, tid.origin FROM transaction_inclusion_decisions tid
		JOIN financial_transactions ft ON ft.id = tid.transaction_id
		WHERE ft.external_id = 'tx-1'`).Scan(&state, &origin)
	if err != nil {
		t.Fatalf("query inclusion decision: %v", err)
	}
	if state != string(money.Ignored) || origin != string(transactions.InclusionOriginRule) {
		t.Errorf("state=%s origin=%s, want ignored/rule (no manual 'apply retroactively' should be needed)", state, origin)
	}
}

// TestExecuteResyncNeverOverridesManualInclusionDecision confirms the safety
// net (internal/transactions/inclusion.go applyInclusion, which refuses a
// rule-origin decision when a manual one already exists) actually holds
// through the resync path added for reevaluation, not just when
// ApplyToNewTransaction is called directly: a transaction the user has
// manually marked "considered" must stay that way even after a resync makes
// it newly match an active rule.
func TestExecuteResyncNeverOverridesManualInclusionDecision(t *testing.T) {
	conn := newTestConn(t)
	ctx := context.Background()
	sourceID, syncRunID1 := newSyncRun(t, conn)
	insertRawImport(t, conn, "raw-accounts", syncRunID1, sourceID)
	insertRawImport(t, conn, "raw-tx-1", syncRunID1, sourceID)

	if _, err := automation.Create(ctx, conn, automation.Write{
		Name: "Ignorar cartão final 4321", IsActive: true, LogicOperator: automation.LogicAnd,
		Conditions: []automation.Condition{{Field: automation.FieldCard, Operator: automation.OperatorContains, Value: "4321"}},
		Actions:    []automation.ActionWrite{{Type: automation.ActionIgnore}},
	}); err != nil {
		t.Fatalf("Create rule: %v", err)
	}

	provider := &fakeProvider{
		source: defaultSource(),
		accountsPage: pluggy.AccountsPage{
			RawImportID: "raw-accounts",
			Accounts:    []pluggy.AccountSnapshot{{ExternalID: "acc-1", CurrencyCode: strp("BRL")}},
		},
		transactionPages: map[string][]pluggy.TransactionsPage{
			"acc-1": {{
				RawImportID: "raw-tx-1",
				Transactions: []pluggy.TransactionSnapshot{{
					ExternalID: "tx-1", ExternalAccountID: "acc-1", Amount: amountP("-1.00"),
					ProviderStatus: strp("PENDING"), MovementType: strp("DEBIT"),
				}},
			}},
		},
	}
	service1 := &syncsvc.Service{
		DB: conn, Provider: provider, SyncRunID: syncRunID1, SourceID: sourceID,
		OnTransactionUpserted: automation.NewTransactionHook(nil),
	}
	if err := service1.Execute(ctx); err != nil {
		t.Fatalf("first Execute: %v", err)
	}

	var txID string
	if err := conn.QueryRow(`SELECT id FROM financial_transactions WHERE external_id = 'tx-1'`).Scan(&txID); err != nil {
		t.Fatalf("query transaction id: %v", err)
	}

	// User explicitly toggles the transaction: ignore, then bring it back to
	// "considered" — the second call is what actually persists a
	// manual-origin decision (see the comment on the analogous sequence in
	// automation's TestManualDecisionBlocksLaterRuleApplication).
	if _, err := transactions.SetInclusion(ctx, conn, txID, money.Ignored, transactions.InclusionOriginManual, nil, nil, nil); err != nil {
		t.Fatalf("SetInclusion (ignore): %v", err)
	}
	if _, err := transactions.SetInclusion(ctx, conn, txID, money.Considered, transactions.InclusionOriginManual, nil, nil, nil); err != nil {
		t.Fatalf("SetInclusion (un-ignore): %v", err)
	}

	now := db.FormatTime(time.Now())
	syncRunID2 := uuid.NewString()
	if _, err := conn.Exec(`INSERT INTO sync_runs (id, source_id, status, started_at) VALUES (?, ?, 'in_progress', ?)`, syncRunID2, sourceID, now); err != nil {
		t.Fatalf("insert second sync_run: %v", err)
	}
	insertRawImport(t, conn, "raw-tx-2", syncRunID2, sourceID)
	provider.transactionPages["acc-1"] = []pluggy.TransactionsPage{{
		RawImportID: "raw-tx-2",
		Transactions: []pluggy.TransactionSnapshot{{
			ExternalID: "tx-1", ExternalAccountID: "acc-1", Amount: amountP("-1.00"),
			ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT"),
			CreditCardMetadata: []byte(`{"cardNumber":"**** 4321"}`),
		}},
	}}
	service2 := &syncsvc.Service{
		DB: conn, Provider: provider, SyncRunID: syncRunID2, SourceID: sourceID,
		OnTransactionUpserted: automation.NewTransactionHook(nil),
	}
	if err := service2.Execute(ctx); err != nil {
		t.Fatalf("second Execute: %v", err)
	}

	var state, origin string
	if err := conn.QueryRow(`SELECT state, origin FROM transaction_inclusion_decisions WHERE transaction_id = ?`, txID).Scan(&state, &origin); err != nil {
		t.Fatalf("query inclusion decision: %v", err)
	}
	if state != string(money.Considered) || origin != string(transactions.InclusionOriginManual) {
		t.Errorf("state=%s origin=%s, want considered/manual (resync must not override the user's manual choice)", state, origin)
	}
}

func TestExecuteRecordsRejectionsAsCompletedWithFailures(t *testing.T) {
	conn := newTestConn(t)
	sourceID, syncRunID := newSyncRun(t, conn)
	insertRawImport(t, conn, "raw-accounts", syncRunID, sourceID)
	insertRawImport(t, conn, "raw-tx-1", syncRunID, sourceID)

	provider := &fakeProvider{
		source: defaultSource(),
		accountsPage: pluggy.AccountsPage{
			RawImportID: "raw-accounts",
			Accounts:    []pluggy.AccountSnapshot{{ExternalID: "acc-1", CurrencyCode: strp("BRL")}},
		},
		transactionPages: map[string][]pluggy.TransactionsPage{
			"acc-1": {{
				RawImportID: "raw-tx-1",
				Transactions: []pluggy.TransactionSnapshot{{
					ExternalID: "tx-1", ExternalAccountID: "acc-1", Amount: amountP("-1.00"),
					ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT"),
				}},
				Rejections: []pluggy.RejectedRecord{{EntityType: "transaction", ExternalID: strp("tx-bad"), Code: "invalid_provider_payload", SafeMessage: "bad"}},
			}},
		},
	}
	if err := (&syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID, SourceID: sourceID}).Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	status, _, inserted, _ := syncRunStatus(t, conn, syncRunID)
	if status != "completed_with_failures" || inserted != 1 {
		t.Errorf("status=%s inserted=%d, want completed_with_failures/1", status, inserted)
	}

	var failureCount int
	conn.QueryRow(`SELECT COUNT(*) FROM sync_failures WHERE sync_run_id = ? AND stage = 'normalize'`, syncRunID).Scan(&failureCount)
	if failureCount != 1 {
		t.Errorf("failureCount = %d, want 1", failureCount)
	}
}

func investmentSafeSource() pluggy.SourceSnapshot {
	return pluggy.SourceSnapshot{
		ExternalItemID: "item-1",
		SafeProducts:   map[string]bool{"ACCOUNTS": true, "TRANSACTIONS": true, "INVESTMENTS": true},
	}
}

func TestExecuteInsertsInvestmentAndItsTransactions(t *testing.T) {
	conn := newTestConn(t)
	sourceID, syncRunID := newSyncRun(t, conn)
	insertRawImport(t, conn, "raw-accounts", syncRunID, sourceID)
	insertRawImport(t, conn, "raw-investments", syncRunID, sourceID)
	insertRawImport(t, conn, "raw-invtx-1", syncRunID, sourceID)

	provider := &fakeProvider{
		source: investmentSafeSource(),
		investmentsPage: pluggy.InvestmentsPage{
			RawImportID: "raw-investments",
			Investments: []pluggy.InvestmentSnapshot{{ExternalID: "inv-1", Name: strp("Fundo XYZ"), Balance: amountP("1000.50")}},
		},
		investmentTransactions: map[string]pluggy.InvestmentTransactionsPage{
			"inv-1": {
				RawImportID: "raw-invtx-1",
				Transactions: []pluggy.InvestmentTransactionSnapshot{{
					ExternalID: "invtx-1", ExternalInvestmentID: "inv-1", MovementType: strp("BUY"),
					Direction: strp("inflow"), Amount: amountP("500.00"),
				}},
			},
		},
	}
	if err := (&syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID, SourceID: sourceID}).Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var investmentsProcessed, txInserted int
	if err := conn.QueryRow(`SELECT investments_processed, investment_transactions_inserted FROM sync_runs WHERE id = ?`, syncRunID).
		Scan(&investmentsProcessed, &txInserted); err != nil {
		t.Fatalf("query sync_run: %v", err)
	}
	if investmentsProcessed != 1 || txInserted != 1 {
		t.Errorf("investmentsProcessed=%d txInserted=%d, want 1/1", investmentsProcessed, txInserted)
	}

	var investmentCount, txCount int
	conn.QueryRow(`SELECT COUNT(*) FROM financial_investments`).Scan(&investmentCount)
	conn.QueryRow(`SELECT COUNT(*) FROM financial_investment_transactions`).Scan(&txCount)
	if investmentCount != 1 || txCount != 1 {
		t.Errorf("investmentCount=%d txCount=%d, want 1/1", investmentCount, txCount)
	}

	// The normalized direction is what the yield math reads, so it has to
	// survive the upsert rather than being left for a reader to re-derive
	// from the provider's movement_type.
	var direction string
	if err := conn.QueryRow(`SELECT direction FROM financial_investment_transactions`).Scan(&direction); err != nil {
		t.Fatalf("query direction: %v", err)
	}
	if direction != "inflow" {
		t.Errorf("direction = %q, want inflow", direction)
	}
}

// TestExecuteUpsertsInvestmentTaxFields covers amount_original/taxes/taxes2:
// the Pluggy FIXED_INCOME (CDB/LCA) detail fields that were previously parsed
// nowhere and silently dropped. The first Execute must persist them on
// INSERT; a second Execute with a changed taxes value must persist the new
// value on UPDATE (InvestmentHash includes the three, so the changed digest
// takes the update branch rather than being treated as unchanged).
func TestExecuteUpsertsInvestmentTaxFields(t *testing.T) {
	conn := newTestConn(t)
	sourceID, syncRunID1 := newSyncRun(t, conn)
	insertRawImport(t, conn, "raw-investments-1", syncRunID1, sourceID)

	provider := &fakeProvider{
		source: investmentSafeSource(),
		investmentsPage: pluggy.InvestmentsPage{
			RawImportID: "raw-investments-1",
			Investments: []pluggy.InvestmentSnapshot{{
				ExternalID: "inv-1", InvestmentType: strp("FIXED_INCOME"), Balance: amountP("1015.32"),
				AmountOriginal: amountP("900.00"), Taxes: amountP("15.00"), Taxes2: amountP("2.50"),
			}},
		},
	}
	if err := (&syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID1, SourceID: sourceID}).Execute(context.Background()); err != nil {
		t.Fatalf("first Execute: %v", err)
	}

	var amountOriginal, taxes, taxes2 string
	if err := conn.QueryRow(`SELECT amount_original, taxes, taxes2 FROM financial_investments WHERE external_id = 'inv-1'`).
		Scan(&amountOriginal, &taxes, &taxes2); err != nil {
		t.Fatalf("query tax fields after insert: %v", err)
	}
	if amountOriginal != "900.00" || taxes != "15.00" || taxes2 != "2.50" {
		t.Errorf("after insert: amount_original=%q taxes=%q taxes2=%q, want 900.00/15.00/2.50", amountOriginal, taxes, taxes2)
	}

	now := db.FormatTime(time.Now())
	syncRunID2 := uuid.NewString()
	if _, err := conn.Exec(`INSERT INTO sync_runs (id, source_id, status, started_at) VALUES (?, ?, 'in_progress', ?)`, syncRunID2, sourceID, now); err != nil {
		t.Fatalf("insert second sync_run: %v", err)
	}
	insertRawImport(t, conn, "raw-investments-2", syncRunID2, sourceID)
	provider.investmentsPage = pluggy.InvestmentsPage{
		RawImportID: "raw-investments-2",
		Investments: []pluggy.InvestmentSnapshot{{
			ExternalID: "inv-1", InvestmentType: strp("FIXED_INCOME"), Balance: amountP("1015.32"),
			AmountOriginal: amountP("900.00"), Taxes: amountP("18.40"), Taxes2: amountP("2.50"),
		}},
	}
	if err := (&syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID2, SourceID: sourceID}).Execute(context.Background()); err != nil {
		t.Fatalf("second Execute: %v", err)
	}

	var investmentCount int
	conn.QueryRow(`SELECT COUNT(*) FROM financial_investments`).Scan(&investmentCount)
	if investmentCount != 1 {
		t.Fatalf("investmentCount = %d, want 1 (update in place, not a duplicate insert)", investmentCount)
	}
	if err := conn.QueryRow(`SELECT amount_original, taxes, taxes2 FROM financial_investments WHERE external_id = 'inv-1'`).
		Scan(&amountOriginal, &taxes, &taxes2); err != nil {
		t.Fatalf("query tax fields after update: %v", err)
	}
	if amountOriginal != "900.00" || taxes != "18.40" || taxes2 != "2.50" {
		t.Errorf("after update: amount_original=%q taxes=%q taxes2=%q, want 900.00/18.40/2.50", amountOriginal, taxes, taxes2)
	}
}

func TestExecuteSkipsInvestmentsWhenNotSafe(t *testing.T) {
	conn := newTestConn(t)
	sourceID, syncRunID := newSyncRun(t, conn)
	insertRawImport(t, conn, "raw-accounts", syncRunID, sourceID)

	provider := &fakeProvider{
		source:         defaultSource(), // no INVESTMENTS key
		investmentsErr: errors.New("GetInvestments should not be called"),
		accountsPage:   pluggy.AccountsPage{RawImportID: "raw-accounts"},
	}
	if err := (&syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID, SourceID: sourceID}).Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var investmentCount int
	conn.QueryRow(`SELECT COUNT(*) FROM financial_investments`).Scan(&investmentCount)
	if investmentCount != 0 {
		t.Errorf("investmentCount = %d, want 0", investmentCount)
	}
}

func TestExecuteSecondRunDetectsUnchangedInvestment(t *testing.T) {
	conn := newTestConn(t)
	sourceID, syncRunID1 := newSyncRun(t, conn)
	insertRawImport(t, conn, "raw-accounts", syncRunID1, sourceID)
	insertRawImport(t, conn, "raw-investments", syncRunID1, sourceID)
	insertRawImport(t, conn, "raw-invtx-local", syncRunID1, sourceID)

	provider := &fakeProvider{
		source:       investmentSafeSource(),
		accountsPage: pluggy.AccountsPage{RawImportID: "raw-accounts"},
		investmentsPage: pluggy.InvestmentsPage{
			RawImportID: "raw-investments",
			Investments: []pluggy.InvestmentSnapshot{{ExternalID: "inv-1", Balance: amountP("1000.50"), CurrencyCode: strp("BRL")}},
		},
		investmentTransactions: map[string]pluggy.InvestmentTransactionsPage{
			"inv-1": {RawImportID: "raw-invtx-local", Transactions: []pluggy.InvestmentTransactionSnapshot{{ExternalID: "mov-1", ExternalInvestmentID: "inv-1", Direction: strp("inflow"), Amount: amountP("1000")}}},
		},
	}
	if err := (&syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID1, SourceID: sourceID}).Execute(context.Background()); err != nil {
		t.Fatalf("first Execute: %v", err)
	}

	ctx := context.Background()
	positions, err := investments.ListPositions(ctx, conn, investments.PositionFilter{})
	if err != nil || len(positions) != 1 {
		t.Fatalf("positions: %v %v", positions, err)
	}
	original := positions[0]
	goal, err := investments.CreatePortfolio(ctx, conn, investments.PortfolioInput{Name: "Reserva"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = investments.UpdatePosition(ctx, conn, original.ID, investments.PositionUpdate{Name: original.Name, Ticker: original.Ticker, AssetType: original.AssetType, PortfolioID: &goal.ID}); err != nil {
		t.Fatal(err)
	}
	op, err := investments.CreateOperation(ctx, conn, investments.OperationInput{AccountID: original.AccountID, Kind: investments.OperationDeposit, OccurredOn: time.Now(), Amount: decimal.NewFromInt(1000)})
	if err != nil {
		t.Fatal(err)
	}
	var movementID string
	if err = conn.QueryRow(`SELECT id FROM financial_investment_transactions WHERE investment_id=?`, original.ID).Scan(&movementID); err != nil {
		t.Fatal(err)
	}
	link, err := investments.CreateReconciliation(ctx, conn, investments.ReconciliationInput{OperationID: op.ID, FinancialInvestmentTransactionID: &movementID, Amount: decimal.NewFromInt(1000)})
	if err != nil {
		t.Fatal(err)
	}

	now := db.FormatTime(time.Now())
	syncRunID2 := uuid.NewString()
	if _, err := conn.Exec(`INSERT INTO sync_runs (id, source_id, status, started_at) VALUES (?, ?, 'in_progress', ?)`, syncRunID2, sourceID, now); err != nil {
		t.Fatalf("insert second sync_run: %v", err)
	}
	if err := (&syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID2, SourceID: sourceID}).Execute(context.Background()); err != nil {
		t.Fatalf("second Execute: %v", err)
	}

	var investmentsProcessed int
	conn.QueryRow(`SELECT investments_processed FROM sync_runs WHERE id = ?`, syncRunID2).Scan(&investmentsProcessed)
	if investmentsProcessed != 1 {
		t.Errorf("investmentsProcessed = %d, want 1", investmentsProcessed)
	}

	var investmentCount int
	conn.QueryRow(`SELECT COUNT(*) FROM financial_investments`).Scan(&investmentCount)
	if investmentCount != 1 {
		t.Errorf("investmentCount = %d, want 1 (no duplicate insert)", investmentCount)
	}
	updated, err := investments.GetPosition(ctx, conn, original.ID)
	if err != nil || updated.PortfolioID == nil || *updated.PortfolioID != goal.ID {
		t.Fatalf("sync lost goal: %+v %v", updated, err)
	}
	links, err := investments.ListReconciliations(ctx, conn, investments.ReconciliationFilter{OperationID: &op.ID})
	if err != nil || len(links) != 1 || links[0].ID != link.ID {
		t.Fatalf("sync lost reconciliation: %+v %v", links, err)
	}

}

func TestExecuteRecordsRejectedInvestmentAsSyncFailure(t *testing.T) {
	conn := newTestConn(t)
	sourceID, syncRunID := newSyncRun(t, conn)
	insertRawImport(t, conn, "raw-accounts", syncRunID, sourceID)
	insertRawImport(t, conn, "raw-investments", syncRunID, sourceID)

	provider := &fakeProvider{
		source:       investmentSafeSource(),
		accountsPage: pluggy.AccountsPage{RawImportID: "raw-accounts"},
		investmentsPage: pluggy.InvestmentsPage{
			RawImportID: "raw-investments",
			Investments: []pluggy.InvestmentSnapshot{{ExternalID: "inv-1", Balance: amountP("1000.50")}},
			Rejections:  []pluggy.RejectedRecord{{EntityType: "investment", ExternalID: strp("inv-bad"), Code: "invalid_provider_payload", SafeMessage: "bad"}},
		},
	}
	if err := (&syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID, SourceID: sourceID}).Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	status, _, _, _ := syncRunStatus(t, conn, syncRunID)
	if status != "completed_with_failures" {
		t.Errorf("status = %s, want completed_with_failures", status)
	}

	var failureCount int
	conn.QueryRow(`SELECT COUNT(*) FROM sync_failures WHERE sync_run_id = ? AND stage = 'normalize'`, syncRunID).Scan(&failureCount)
	if failureCount != 1 {
		t.Errorf("failureCount = %d, want 1", failureCount)
	}
}

func TestExecuteFailsGeneralWhenAccountsUnsafe(t *testing.T) {
	conn := newTestConn(t)
	sourceID, syncRunID := newSyncRun(t, conn)
	insertRawImport(t, conn, "raw-item", syncRunID, sourceID)

	provider := &fakeProvider{
		source:            pluggy.SourceSnapshot{ExternalItemID: "item-1", SafeProducts: map[string]bool{}},
		sourceRawImportID: "raw-item",
	}
	if err := (&syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID, SourceID: sourceID}).Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var status, generalCode string
	conn.QueryRow(`SELECT status, general_error_code FROM sync_runs WHERE id = ?`, syncRunID).Scan(&status, &generalCode)
	if status != "failed" || generalCode != "item_unavailable" {
		t.Errorf("status=%s generalCode=%s, want failed/item_unavailable", status, generalCode)
	}
}

func TestExecuteFailsGeneralOnProviderError(t *testing.T) {
	conn := newTestConn(t)
	sourceID, syncRunID := newSyncRun(t, conn)

	provider := &fakeProvider{
		sourceErr: &pluggy.ProviderError{Code: "invalid_provider_credentials", Stage: pluggy.StageAuth, SafeMessage: "bad creds"},
	}
	if err := (&syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID, SourceID: sourceID}).Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var status, generalCode string
	conn.QueryRow(`SELECT status, general_error_code FROM sync_runs WHERE id = ?`, syncRunID).Scan(&status, &generalCode)
	if status != "failed" || generalCode != "invalid_provider_credentials" {
		t.Errorf("status=%s generalCode=%s", status, generalCode)
	}
}

// A transaction the user categorized by hand in an earlier sync becomes the
// reference for a same-looking one inserted later: the learned decision
// beats the Groceries → Supermercado mapping.
func TestExecuteLearnsCategoryFromEarlierManualDecision(t *testing.T) {
	conn := newTestConn(t)
	sourceID, syncRunID := newSyncRun(t, conn)
	insertRawImport(t, conn, "raw-item", syncRunID, sourceID)
	insertRawImport(t, conn, "raw-accounts", syncRunID, sourceID)
	insertRawImport(t, conn, "raw-tx-1", syncRunID, sourceID)

	snapshot := func(externalID string) pluggy.TransactionSnapshot {
		return pluggy.TransactionSnapshot{
			ExternalID: externalID, ExternalAccountID: "acc-1", Amount: amountP("-35.00"),
			AmountInAccountCurrency: amountP("-35.00"), CurrencyCode: strp("BRL"),
			ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT"), SourceCategory: strp("Groceries"),
			Description: strp("Conveniencia Nova York"),
		}
	}
	provider := &fakeProvider{
		source:            defaultSource(),
		sourceRawImportID: "raw-item",
		accountsPage: pluggy.AccountsPage{
			RawImportID: "raw-accounts",
			Accounts:    []pluggy.AccountSnapshot{{ExternalID: "acc-1", CurrencyCode: strp("BRL")}},
		},
		transactionPages: map[string][]pluggy.TransactionsPage{
			"acc-1": {{RawImportID: "raw-tx-1", Transactions: []pluggy.TransactionSnapshot{snapshot("tx-1")}}},
		},
	}
	service := &syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID, SourceID: sourceID}
	if err := service.Execute(context.Background()); err != nil {
		t.Fatalf("first Execute: %v", err)
	}

	// The user recategorizes it as Lazer.
	lazer, err := categories.Create(context.Background(), conn, "Lazer", money.Expense, "tag", "#000000")
	if err != nil {
		t.Fatal(err)
	}
	var firstID string
	if err := conn.QueryRow(`SELECT id FROM financial_transactions WHERE external_id = 'tx-1'`).Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	if _, err := categories.AssignManual(context.Background(), conn, firstID, lazer.ID); err != nil {
		t.Fatal(err)
	}

	// A second sync brings a new transaction at the same merchant.
	syncRunID2 := uuid.NewString()
	if _, err := conn.Exec(`INSERT INTO sync_runs (id, source_id, status, started_at) VALUES (?, ?, 'in_progress', ?)`,
		syncRunID2, sourceID, db.FormatTime(time.Now())); err != nil {
		t.Fatalf("insert second sync_run: %v", err)
	}
	insertRawImport(t, conn, "raw-item-2", syncRunID2, sourceID)
	insertRawImport(t, conn, "raw-accounts-2", syncRunID2, sourceID)
	insertRawImport(t, conn, "raw-tx-2", syncRunID2, sourceID)
	provider.sourceRawImportID = "raw-item-2"
	provider.accountsPage.RawImportID = "raw-accounts-2"
	provider.transactionPages = map[string][]pluggy.TransactionsPage{
		"acc-1": {{RawImportID: "raw-tx-2", Transactions: []pluggy.TransactionSnapshot{snapshot("tx-1"), snapshot("tx-2")}}},
	}
	service = &syncsvc.Service{DB: conn, Provider: provider, SyncRunID: syncRunID2, SourceID: sourceID}
	if err := service.Execute(context.Background()); err != nil {
		t.Fatalf("second Execute: %v", err)
	}

	var categoryID, origin string
	err = conn.QueryRow(`
		SELECT tcd.category_id, tcd.origin FROM transaction_category_decisions tcd
		JOIN financial_transactions ft ON ft.id = tcd.transaction_id
		WHERE ft.external_id = 'tx-2'`).Scan(&categoryID, &origin)
	if err != nil {
		t.Fatalf("query category decision: %v", err)
	}
	if categoryID != lazer.ID || origin != "learned" {
		t.Errorf("category=%s origin=%s, want %s/learned", categoryID, origin, lazer.ID)
	}
	// The reference itself stays manual.
	err = conn.QueryRow(`SELECT tcd.origin FROM transaction_category_decisions tcd WHERE tcd.transaction_id = ?`, firstID).Scan(&origin)
	if err != nil || origin != "manual" {
		t.Errorf("reference origin=%s err=%v", origin, err)
	}
}
