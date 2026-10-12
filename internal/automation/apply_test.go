package automation_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/automation"
	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/transactions"
)

// fixture mirrors the minimal sync-schema chain other packages' tests use,
// plus a helper to add accounts/transactions with the fields matching cares
// about (description, credit_card_metadata, account name/institution).
type fixture struct {
	t           *testing.T
	conn        *sql.DB
	sourceID    string
	rawImportID string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	conn := newTestDB(t)
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

func (f *fixture) addAccount(name, institution string) string {
	f.t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	f.exec(`INSERT INTO financial_accounts (
			id, source_id, external_id, name, institution, current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, 'hash', ?, ?)`, id, f.sourceID, id, name, institution, f.rawImportID, now, now)
	return id
}

func (f *fixture) addTransaction(accountID, description, creditCardMetadata string) string {
	f.t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	var metadata any
	if creditCardMetadata != "" {
		metadata = creditCardMetadata
	}
	f.exec(`INSERT INTO financial_transactions (
			id, source_id, account_id, external_id, description, credit_card_metadata,
			provider_status, movement_type, current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, 'POSTED', 'DEBIT', ?, 'hash', ?, ?)`,
		id, f.sourceID, accountID, id, description, metadata, f.rawImportID, now, now)
	return id
}

func TestApplyToNewTransactionIgnoresOnFirstMatchingRule(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := f.addAccount("Conta Corrente", "Banco Exemplo")
	txID := f.addTransaction(acc, "Transferencia enviada", "")

	if _, err := automation.Create(ctx, f.conn, automation.Write{
		Name: "Ignorar transferências", IsActive: true, LogicOperator: automation.LogicAnd,
		Conditions: []automation.Condition{{Field: automation.FieldDescription, Operator: automation.OperatorContains, Value: "transferencia"}},
		Actions:    []automation.ActionWrite{{Type: automation.ActionIgnore}},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := automation.ApplyToNewTransaction(ctx, f.conn, txID, nil); err != nil {
		t.Fatalf("ApplyToNewTransaction: %v", err)
	}

	var state, origin string
	if err := f.conn.QueryRow(`SELECT state, origin FROM transaction_inclusion_decisions WHERE transaction_id = ?`, txID).Scan(&state, &origin); err != nil {
		t.Fatalf("query decision: %v", err)
	}
	if state != string(money.Ignored) || origin != string(transactions.InclusionOriginRule) {
		t.Errorf("state=%s origin=%s", state, origin)
	}
}

func TestApplyToNewTransactionNoRuleMatchesLeavesConsidered(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := f.addAccount("Conta Corrente", "Banco Exemplo")
	txID := f.addTransaction(acc, "Mercado XYZ", "")

	if _, err := automation.Create(ctx, f.conn, automation.Write{
		Name: "Ignorar transferências", IsActive: true, LogicOperator: automation.LogicAnd,
		Conditions: []automation.Condition{{Field: automation.FieldDescription, Operator: automation.OperatorContains, Value: "transferencia"}},
		Actions:    []automation.ActionWrite{{Type: automation.ActionIgnore}},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := automation.ApplyToNewTransaction(ctx, f.conn, txID, nil); err != nil {
		t.Fatalf("ApplyToNewTransaction: %v", err)
	}

	var count int
	f.conn.QueryRow(`SELECT COUNT(*) FROM transaction_inclusion_decisions WHERE transaction_id = ?`, txID).Scan(&count)
	if count != 0 {
		t.Errorf("expected no inclusion decision when no rule matches, got %d", count)
	}
}

func TestApplyToNewTransactionMatchesCardMetadata(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := f.addAccount("Cartão", "Banco Exemplo")
	txID := f.addTransaction(acc, "Compra qualquer", `{"cardNumber":"**** 4321"}`)

	if _, err := automation.Create(ctx, f.conn, automation.Write{
		Name: "Ignorar cartão final 4321", IsActive: true, LogicOperator: automation.LogicAnd,
		Conditions: []automation.Condition{{Field: automation.FieldCard, Operator: automation.OperatorContains, Value: "4321"}},
		Actions:    []automation.ActionWrite{{Type: automation.ActionIgnore}},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := automation.ApplyToNewTransaction(ctx, f.conn, txID, nil); err != nil {
		t.Fatalf("ApplyToNewTransaction: %v", err)
	}

	var state string
	if err := f.conn.QueryRow(`SELECT state FROM transaction_inclusion_decisions WHERE transaction_id = ?`, txID).Scan(&state); err != nil {
		t.Fatalf("query decision: %v", err)
	}
	if state != string(money.Ignored) {
		t.Errorf("state = %s, want ignored", state)
	}
}

func TestApplyToNewTransactionNeverActsOnReconcileOnlyRules(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := f.addAccount("Conta Corrente", "Banco Exemplo")
	txID := f.addTransaction(acc, "Transferencia enviada", "")
	commitment := newCommitment(t, f.conn)

	if _, err := automation.Create(ctx, f.conn, reconcileWrite(commitment.ID)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := automation.ApplyToNewTransaction(ctx, f.conn, txID, nil); err != nil {
		t.Fatalf("ApplyToNewTransaction: %v", err)
	}

	var count int
	f.conn.QueryRow(`SELECT COUNT(*) FROM transaction_inclusion_decisions WHERE transaction_id = ?`, txID).Scan(&count)
	if count != 0 {
		t.Errorf("a reconcile-only rule must never ignore a transaction, got %d decisions", count)
	}
}

func TestApplyRetroactivelyNeverActsOnReconcileOnlyRules(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := f.addAccount("Conta Corrente", "Banco Exemplo")
	txID := f.addTransaction(acc, "Transferencia enviada", "")
	commitment := newCommitment(t, f.conn)

	rule, err := automation.Create(ctx, f.conn, reconcileWrite(commitment.ID))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	result, err := automation.ApplyRetroactively(ctx, f.conn, rule.ID, nil)
	if err != nil {
		t.Fatalf("ApplyRetroactively: %v", err)
	}
	if result.Matched != 0 || result.Ignored != 0 {
		t.Errorf("a reconcile-only rule must never match/ignore transactions retroactively, got %+v", result)
	}
	var count int
	f.conn.QueryRow(`SELECT COUNT(*) FROM transaction_inclusion_decisions WHERE transaction_id = ?`, txID).Scan(&count)
	if count != 0 {
		t.Errorf("expected no inclusion decision, got %d", count)
	}
}

func TestApplyToNewTransactionSkipsWhenNoActiveRules(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := f.addAccount("Conta", "Banco")
	txID := f.addTransaction(acc, "Qualquer coisa", "")

	if err := automation.ApplyToNewTransaction(ctx, f.conn, txID, nil); err != nil {
		t.Fatalf("ApplyToNewTransaction: %v", err)
	}
	var count int
	f.conn.QueryRow(`SELECT COUNT(*) FROM transaction_inclusion_decisions WHERE transaction_id = ?`, txID).Scan(&count)
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestManualDecisionBlocksLaterRuleApplication(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := f.addAccount("Conta", "Banco")
	txID := f.addTransaction(acc, "Transferencia enviada", "")

	// Setting state to "considered" with no prior decision is a no-op (it's
	// already the implicit default, so nothing is persisted) — a real
	// manual-origin "considered" row only exists once the user has toggled
	// it away from and back to considered. That's what actually protects a
	// transaction from a later rule, so recreate that sequence here.
	if _, err := transactions.SetInclusion(ctx, f.conn, txID, money.Ignored, transactions.InclusionOriginManual, nil, nil, nil); err != nil {
		t.Fatalf("SetInclusion (ignore): %v", err)
	}
	if _, err := transactions.SetInclusion(ctx, f.conn, txID, money.Considered, transactions.InclusionOriginManual, nil, nil, nil); err != nil {
		t.Fatalf("SetInclusion (un-ignore): %v", err)
	}

	if _, err := automation.Create(ctx, f.conn, automation.Write{
		Name: "Ignorar transferências", IsActive: true, LogicOperator: automation.LogicAnd,
		Conditions: []automation.Condition{{Field: automation.FieldDescription, Operator: automation.OperatorContains, Value: "transferencia"}},
		Actions:    []automation.ActionWrite{{Type: automation.ActionIgnore}},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := automation.ApplyToNewTransaction(ctx, f.conn, txID, nil); err != nil {
		t.Fatalf("ApplyToNewTransaction: %v", err)
	}

	var state, origin string
	f.conn.QueryRow(`SELECT state, origin FROM transaction_inclusion_decisions WHERE transaction_id = ?`, txID).Scan(&state, &origin)
	if state != string(money.Considered) || origin != string(transactions.InclusionOriginManual) {
		t.Errorf("state=%s origin=%s, want considered/manual (unchanged)", state, origin)
	}
}

func TestApplyRetroactivelyCountsMatchedAndActuallyChanged(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := f.addAccount("Conta", "Banco")
	tx1 := f.addTransaction(acc, "Transferencia 1", "")
	tx2 := f.addTransaction(acc, "Transferencia 2", "")
	tx3 := f.addTransaction(acc, "Mercado", "")

	// tx1 is already manually protected (see the comment in
	// TestManualDecisionBlocksLaterRuleApplication for why a bare "set to
	// considered" wouldn't actually persist a manual decision): it should
	// count as matched but not as newly ignored, since the manual decision
	// blocks the rule.
	if _, err := transactions.SetInclusion(ctx, f.conn, tx1, money.Ignored, transactions.InclusionOriginManual, nil, nil, nil); err != nil {
		t.Fatalf("SetInclusion (ignore): %v", err)
	}
	if _, err := transactions.SetInclusion(ctx, f.conn, tx1, money.Considered, transactions.InclusionOriginManual, nil, nil, nil); err != nil {
		t.Fatalf("SetInclusion (un-ignore): %v", err)
	}

	rule, err := automation.Create(ctx, f.conn, automation.Write{
		Name: "Ignorar transferências", IsActive: false, LogicOperator: automation.LogicAnd,
		Conditions: []automation.Condition{{Field: automation.FieldDescription, Operator: automation.OperatorContains, Value: "transferencia"}},
		Actions:    []automation.ActionWrite{{Type: automation.ActionIgnore}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	result, err := automation.ApplyRetroactively(ctx, f.conn, rule.ID, nil)
	if err != nil {
		t.Fatalf("ApplyRetroactively: %v", err)
	}
	if result.Matched != 2 {
		t.Errorf("Matched = %d, want 2 (tx1 and tx2)", result.Matched)
	}
	if result.Ignored != 1 {
		t.Errorf("Ignored = %d, want 1 (only tx2 actually changed)", result.Ignored)
	}

	var tx2State string
	f.conn.QueryRow(`SELECT state FROM transaction_inclusion_decisions WHERE transaction_id = ?`, tx2).Scan(&tx2State)
	if tx2State != string(money.Ignored) {
		t.Errorf("tx2 state = %s, want ignored", tx2State)
	}
	var tx3Count int
	f.conn.QueryRow(`SELECT COUNT(*) FROM transaction_inclusion_decisions WHERE transaction_id = ?`, tx3).Scan(&tx3Count)
	if tx3Count != 0 {
		t.Errorf("tx3 (non-matching) should have no decision, got %d", tx3Count)
	}
}

func TestApplyRetroactivelyUnknownRuleIsANoOp(t *testing.T) {
	f := newFixture(t)
	result, err := automation.ApplyRetroactively(context.Background(), f.conn, "unknown", nil)
	if err != nil {
		t.Fatalf("ApplyRetroactively: %v", err)
	}
	if result.Matched != 0 || result.Ignored != 0 {
		t.Errorf("result = %+v, want zero", result)
	}
}

func TestApplyToNewTransactionAppliesSetCategoryAction(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := f.addAccount("Conta Corrente", "Banco Exemplo")
	txID := f.addTransaction(acc, "Assinatura streaming", "")

	category, err := categories.Create(ctx, f.conn, "Assinaturas", money.Expense, "icon", "#fff")
	if err != nil {
		t.Fatalf("categories.Create: %v", err)
	}

	if _, err := automation.Create(ctx, f.conn, automation.Write{
		Name: "Categorizar assinaturas", IsActive: true, LogicOperator: automation.LogicAnd,
		Conditions: []automation.Condition{{Field: automation.FieldDescription, Operator: automation.OperatorContains, Value: "assinatura"}},
		Actions:    []automation.ActionWrite{{Type: automation.ActionSetCategory, CategoryID: &category.ID}},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := automation.ApplyToNewTransaction(ctx, f.conn, txID, nil); err != nil {
		t.Fatalf("ApplyToNewTransaction: %v", err)
	}

	var categoryID, origin string
	if err := f.conn.QueryRow(`SELECT category_id, origin FROM transaction_category_decisions WHERE transaction_id = ?`, txID).Scan(&categoryID, &origin); err != nil {
		t.Fatalf("query decision: %v", err)
	}
	if categoryID != category.ID || origin != string(categories.OriginRule) {
		t.Errorf("category_id=%s origin=%s, want %s/%s", categoryID, origin, category.ID, categories.OriginRule)
	}
}

func TestApplyToNewTransactionCombinesIgnoreAndSetCategoryOnOneRule(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := f.addAccount("Conta Corrente", "Banco Exemplo")
	txID := f.addTransaction(acc, "Transferencia enviada", "")

	category, err := categories.Create(ctx, f.conn, "Transferências", money.Expense, "icon", "#fff")
	if err != nil {
		t.Fatalf("categories.Create: %v", err)
	}

	if _, err := automation.Create(ctx, f.conn, automation.Write{
		Name: "Ignorar e categorizar transferências", IsActive: true, LogicOperator: automation.LogicAnd,
		Conditions: []automation.Condition{{Field: automation.FieldDescription, Operator: automation.OperatorContains, Value: "transferencia"}},
		Actions: []automation.ActionWrite{
			{Type: automation.ActionIgnore},
			{Type: automation.ActionSetCategory, CategoryID: &category.ID},
		},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := automation.ApplyToNewTransaction(ctx, f.conn, txID, nil); err != nil {
		t.Fatalf("ApplyToNewTransaction: %v", err)
	}

	var state string
	if err := f.conn.QueryRow(`SELECT state FROM transaction_inclusion_decisions WHERE transaction_id = ?`, txID).Scan(&state); err != nil {
		t.Fatalf("query inclusion decision: %v", err)
	}
	if state != string(money.Ignored) {
		t.Errorf("state = %s, want ignored", state)
	}
	var categoryID string
	if err := f.conn.QueryRow(`SELECT category_id FROM transaction_category_decisions WHERE transaction_id = ?`, txID).Scan(&categoryID); err != nil {
		t.Fatalf("query category decision: %v", err)
	}
	if categoryID != category.ID {
		t.Errorf("category_id = %s, want %s", categoryID, category.ID)
	}
}

func TestApplyRetroactivelyAppliesSetCategoryAction(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := f.addAccount("Conta", "Banco")
	tx1 := f.addTransaction(acc, "Assinatura 1", "")
	tx2 := f.addTransaction(acc, "Mercado", "")

	category, err := categories.Create(ctx, f.conn, "Assinaturas", money.Expense, "icon", "#fff")
	if err != nil {
		t.Fatalf("categories.Create: %v", err)
	}

	rule, err := automation.Create(ctx, f.conn, automation.Write{
		Name: "Categorizar assinaturas", IsActive: false, LogicOperator: automation.LogicAnd,
		Conditions: []automation.Condition{{Field: automation.FieldDescription, Operator: automation.OperatorContains, Value: "assinatura"}},
		Actions:    []automation.ActionWrite{{Type: automation.ActionSetCategory, CategoryID: &category.ID}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	result, err := automation.ApplyRetroactively(ctx, f.conn, rule.ID, nil)
	if err != nil {
		t.Fatalf("ApplyRetroactively: %v", err)
	}
	if result.Matched != 1 || result.Categorized != 1 {
		t.Errorf("result = %+v, want Matched=1 Categorized=1", result)
	}

	var categoryID string
	if err := f.conn.QueryRow(`SELECT category_id FROM transaction_category_decisions WHERE transaction_id = ?`, tx1).Scan(&categoryID); err != nil {
		t.Fatalf("query category decision: %v", err)
	}
	if categoryID != category.ID {
		t.Errorf("category_id = %s, want %s", categoryID, category.ID)
	}
	var tx2Count int
	f.conn.QueryRow(`SELECT COUNT(*) FROM transaction_category_decisions WHERE transaction_id = ?`, tx2).Scan(&tx2Count)
	if tx2Count != 0 {
		t.Errorf("tx2 (non-matching) should have no category decision, got %d", tx2Count)
	}
}

func TestApplyRetroactivelySkipsDeletedManualTransactions(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := f.addAccount("Conta", "Banco")
	id, err := transactions.CreateManual(ctx, f.conn, transactions.ManualInput{
		AccountID: acc, Description: "Assinatura removida", Amount: decimal.RequireFromString("-10"), OccurredAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("CreateManual: %v", err)
	}
	if err := transactions.DeleteManual(ctx, f.conn, id, nil); err != nil {
		t.Fatalf("DeleteManual: %v", err)
	}
	category, err := categories.Create(ctx, f.conn, "Assinaturas", money.Expense, "icon", "#fff")
	if err != nil {
		t.Fatalf("categories.Create: %v", err)
	}
	rule, err := automation.Create(ctx, f.conn, automation.Write{
		Name: "Categorizar assinaturas", IsActive: false, LogicOperator: automation.LogicAnd,
		Conditions: []automation.Condition{{Field: automation.FieldDescription, Operator: automation.OperatorContains, Value: "assinatura"}},
		Actions:    []automation.ActionWrite{{Type: automation.ActionSetCategory, CategoryID: &category.ID}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	result, err := automation.ApplyRetroactively(ctx, f.conn, rule.ID, nil)
	if err != nil {
		t.Fatalf("ApplyRetroactively: %v", err)
	}
	if result.Matched != 0 || result.Categorized != 0 {
		t.Errorf("result = %+v, want no deleted transaction matches", result)
	}
}

func TestNewTransactionHookMatchesSyncNewTransactionHookSignature(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := f.addAccount("Conta", "Banco")
	txID := f.addTransaction(acc, "Transferencia enviada", "")

	if _, err := automation.Create(ctx, f.conn, automation.Write{
		Name: "Ignorar transferências", IsActive: true, LogicOperator: automation.LogicAnd,
		Conditions: []automation.Condition{{Field: automation.FieldDescription, Operator: automation.OperatorContains, Value: "transferencia"}},
		Actions:    []automation.ActionWrite{{Type: automation.ActionIgnore}},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var hookCalled bool
	onIgnored := func(ctx context.Context, q transactions.Querier, transactionID string) error {
		hookCalled = true
		return nil
	}
	hook := automation.NewTransactionHook(onIgnored)
	tx, err := f.conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback()
	if err := hook(ctx, tx, txID, acc); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !hookCalled {
		t.Error("onIgnored hook should have been invoked when a rule ignores the transaction")
	}
}

func TestCombinedCategoryAndReconcileApplication(t *testing.T) {
	for _, mode := range []string{"new", "resynced", "retroactive"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			ctx := context.Background()
			commitment := newCommitment(t, f.conn)
			account := f.addAccount("Conta", "Banco")
			f.exec(`UPDATE financial_accounts SET currency_code = 'BRL' WHERE id = ?`, account)
			matching := f.addTransaction(account, "Aluguel", `{"cardNumber":"1234"}`)
			manual := f.addTransaction(account, "Aluguel manual", `{"cardNumber":"1234"}`)
			outside := f.addTransaction(account, "Aluguel outro dia", `{"cardNumber":"1234"}`)
			wrongAmount := f.addTransaction(account, "Aluguel outro valor", `{"cardNumber":"1234"}`)
			wrongDescription := f.addTransaction(account, "Supermercado", `{"cardNumber":"1234"}`)
			for _, id := range []string{matching, manual, outside, wrongAmount, wrongDescription} {
				f.exec(`UPDATE financial_transactions SET amount = '-300', currency_code = 'USD', amount_in_account_currency = '-1500', occurred_at = ? WHERE id = ?`, db.FormatTime(time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)), id)
			}
			f.exec(`UPDATE financial_transactions SET occurred_at = ? WHERE id = ?`, db.FormatTime(time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)), outside)
			// Strictly earlier than the manual twin so the reconciler's earliest-wins pick is not a tie.
			f.exec(`UPDATE financial_transactions SET occurred_at = ? WHERE id = ?`, db.FormatTime(time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)), matching)
			f.exec(`UPDATE financial_transactions SET amount_in_account_currency = '-2500' WHERE id = ?`, wrongAmount)
			manualCategory, err := categories.Create(ctx, f.conn, "Manual", money.Expense, "icon", "#fff")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := categories.AssignManual(ctx, f.conn, manual, manualCategory.ID); err != nil {
				t.Fatal(err)
			}
			write := reconcileWrite(commitment.ID)
			write.Conditions = []automation.Condition{
				{Field: automation.FieldAmount, Operator: automation.OperatorWithinPercent, Value: "1500:10"},
				{Field: automation.FieldDayOfMonth, Operator: automation.OperatorDayRange, Value: "1:10"},
				{Field: automation.FieldDescription, Operator: automation.OperatorContains, Value: "aluguel"},
				{Field: automation.FieldAccount, Operator: automation.OperatorEquals, Value: "Conta"},
				{Field: automation.FieldAccount, Operator: automation.OperatorEquals, Value: "Banco"},
				{Field: automation.FieldCard, Operator: automation.OperatorEquals, Value: "1234"},
			}
			write.Actions = append(write.Actions, automation.ActionWrite{Type: automation.ActionSetCategory, CategoryID: &commitment.CategoryID})
			rule, err := automation.Create(ctx, f.conn, write)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := automation.Get(ctx, f.conn, rule.ID)
			if err != nil || len(loaded.Actions) != 2 {
				t.Fatalf("round trip: %+v, %v", loaded, err)
			}
			if mode == "retroactive" {
				result, err := automation.ApplyRetroactively(ctx, f.conn, rule.ID, nil)
				if err != nil || result.Matched != 2 || result.Categorized != 1 || result.Ignored != 0 {
					t.Fatalf("apply: %+v, %v", result, err)
				}
				again, err := automation.ApplyRetroactively(ctx, f.conn, rule.ID, nil)
				if err != nil || again.Categorized != 0 {
					t.Fatalf("repeat: %+v, %v", again, err)
				}
			} else {
				if mode == "resynced" {
					f.exec(`UPDATE financial_transactions SET amount_in_account_currency = '-2500' WHERE id = ?`, matching)
					if err := automation.ApplyToNewTransaction(ctx, f.conn, matching, nil); err != nil {
						t.Fatal(err)
					}
					f.exec(`UPDATE financial_transactions SET amount_in_account_currency = '-1500' WHERE id = ?`, matching)
				}
				for _, id := range []string{matching, manual, outside, wrongAmount, wrongDescription} {
					for range 2 {
						if err := automation.ApplyToNewTransaction(ctx, f.conn, id, nil); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			for id, expected := range map[string]string{matching: commitment.CategoryID, manual: manualCategory.ID} {
				var categoryID, origin string
				if err := f.conn.QueryRow(`SELECT category_id, origin FROM transaction_category_decisions WHERE transaction_id = ?`, id).Scan(&categoryID, &origin); err != nil {
					t.Fatal(err)
				}
				wantOrigin := "rule"
				if id == manual {
					wantOrigin = "manual"
				}
				if categoryID != expected || origin != wantOrigin {
					t.Fatalf("decision %s: %s/%s", id, categoryID, origin)
				}
			}
			var count int
			if err := f.conn.QueryRow(`SELECT COUNT(*) FROM transaction_category_decisions WHERE transaction_id IN (?, ?, ?)`, outside, wrongAmount, wrongDescription).Scan(&count); err != nil || count != 0 {
				t.Fatalf("non-matches: %d, %v", count, err)
			}
			items, err := transactions.Query(ctx, f.conn, transactions.QueryRequest{Timezone: "UTC", GroupBy: money.GroupMonth, Page: 1, PageSize: 100})
			if err != nil {
				t.Fatal(err)
			}
			targets, err := automation.ListActiveReconcileTargets(ctx, f.conn)
			if err != nil {
				t.Fatal(err)
			}
			target := targets[commitment.ID]
			reconciler := recurrences.NewReconciler(commitment, target.Conditions, target.LogicOperator, true, nil, items.Items)
			resolved := reconciler.Resolve(recurrences.Occurrence{Date: time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), ExpectedAmount: commitment.Amount})
			if resolved.TransactionID == nil || *resolved.TransactionID != matching || resolved.Origin != recurrences.OriginRule {
				t.Fatalf("reconciliation: %+v", resolved)
			}
		})
	}
}
