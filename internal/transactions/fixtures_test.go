package transactions_test

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
)

// fixture wires up the minimal sync-schema chain (data_sources -> sync_runs
// -> raw_imports) that financial_accounts/financial_transactions require as
// foreign keys, so domain tests can insert accounts/transactions directly
// without caring about sync bookkeeping (that's phase 4's concern).
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

	return newFixtureWithConn(t, conn)
}

func newFixtureWithConn(t *testing.T, conn *sql.DB) *fixture {
	t.Helper()
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
		) VALUES (?, ?, ?, 'transactions', 1, 1, 'GET', '/x', 200, '{}', ?, 'sha', ?)`,
		f.rawImportID, syncRunID, f.sourceID, []byte{0}, now)
	return f
}

func (f *fixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.conn.Exec(query, args...); err != nil {
		f.t.Fatalf("exec %q: %v", query, err)
	}
}

// account is the input to addAccount; zero-value optional fields stay NULL.
// A nil AccountType is the "provider never told us" case CashOnHand has to
// treat as non-credit.
type account struct {
	Name         *string
	Institution  *string
	CurrencyCode *string
	AccountType  *string
	Balance      *string
}

func (f *fixture) addAccount(a account) string {
	f.t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	f.exec(`INSERT INTO financial_accounts (
			id, source_id, external_id, name, institution, currency_code, account_type, balance,
			current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'hash', ?, ?)`,
		id, f.sourceID, id, a.Name, a.Institution, a.CurrencyCode, a.AccountType, a.Balance,
		f.rawImportID, now, now)
	return id
}

// bill is the input to addBill.
type bill struct {
	AccountID   string
	ExternalID  string
	DueDate     time.Time
	ClosingDate time.Time // zero stays NULL, matching a provider that reported none
}

func (f *fixture) addBill(b bill) string {
	f.t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	var closing *string
	if !b.ClosingDate.IsZero() {
		formatted := b.ClosingDate.Format("2006-01-02")
		closing = &formatted
	}
	f.exec(`INSERT INTO financial_bills (
			id, source_id, account_id, external_id, due_date, closing_date,
			current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, 'hash', ?, ?)`,
		id, f.sourceID, b.AccountID, b.ExternalID, b.DueDate.Format("2006-01-02"), closing, f.rawImportID, now, now)
	return id
}

// setPreference writes a plaintext settings row directly (mirroring
// settings.Set with encrypted=false) so tests can exercise a non-default
// transactions.period_basis without importing package settings.
func (f *fixture) setPreference(key, value string) {
	f.t.Helper()
	now := db.FormatTime(time.Now())
	f.exec(`INSERT INTO settings (key, value, is_encrypted, updated_at) VALUES (?, ?, FALSE, ?)`, key, value, now)
}

// txn is the input to addTransaction; nil pointers stay NULL, matching a
// provider omitting that field.
type txn struct {
	AccountID                   string
	Description                 *string
	Amount                      *string
	AmountInAccountCurrency     *string
	CurrencyCode                *string
	OccurredAt                  *time.Time
	ProviderStatus              *string
	MovementType                *string
	SourceCategory              *string
	SourceCategoryID            *string
	OperationTypeAdditionalInfo *string
	CreditCardMetadata          *string
}

func (f *fixture) addTransaction(tx txn) string {
	f.t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	var occurredAt any
	if tx.OccurredAt != nil {
		occurredAt = db.FormatTime(*tx.OccurredAt)
	}
	f.exec(`INSERT INTO financial_transactions (
			id, source_id, account_id, external_id, description, amount,
			amount_in_account_currency, currency_code, occurred_at, provider_status,
			movement_type, source_category, source_category_id, operation_type_additional_info,
			credit_card_metadata, current_raw_import_id,
			normalized_hash, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'hash', ?, ?)`,
		id, f.sourceID, tx.AccountID, id, tx.Description, tx.Amount, tx.AmountInAccountCurrency,
		tx.CurrencyCode, occurredAt, tx.ProviderStatus, tx.MovementType, tx.SourceCategory,
		tx.SourceCategoryID, tx.OperationTypeAdditionalInfo,
		tx.CreditCardMetadata, f.rawImportID, now, now)
	return id
}

// addAutomationRule inserts a minimal automation_rules row so tests can
// reference a real rule_id (transaction_inclusion_decisions.rule_id is a
// foreign key into automation_rules).
func (f *fixture) addAutomationRule(name string) string {
	f.t.Helper()
	id := uuid.NewString()
	now := db.FormatTime(time.Now())
	f.exec(`INSERT INTO automation_rules (id, name, is_active, logic_operator, created_at, updated_at)
		VALUES (?, ?, TRUE, 'and', ?, ?)`, id, name, now, now)
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

func strp(s string) *string { return &s }

const (
	categorySupermercado  = "000433b6-3094-5a9c-87df-465b70574a4b" // expense
	categorySalario       = "3c5a9586-2a11-556d-b014-692ed51c3997" // income
	categoryTransferencia = "533d9187-99b6-542b-a2f3-6eb9cbb299ce" // transfer
)
