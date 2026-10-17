package investments

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/money"
)

func parseDate(raw string) (time.Time, error) {
	return dbtypes.ParseDate(raw)
}

func parseTimestamp(raw string) (time.Time, error) {
	return dbtypes.ParseTime(raw)
}

func formatDate(t time.Time) string { return Day(t).Format(DateLayout) }

func nullableString(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

func nullableDecimal(d *decimal.Decimal) any {
	if d == nil {
		return nil
	}
	return money.CanonicalDecimal(*d)
}

func nullableDate(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatDate(*t)
}

func trimOptional(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return &v
}

// EnsureIntegratedAccounts materializes the local custody grouping for every
// connection that currently has provider holdings. It is idempotent and is
// called before unified reads, which also covers a new connection created
// after the database migration first ran.
func EnsureIntegratedAccounts(ctx context.Context, q Querier) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO investment_accounts (
			id, name, kind, currency_code, source_id, is_active, created_at, updated_at
		)
		SELECT
			'integrated:' || ds.id,
			COALESCE(NULLIF(ds.label, ''), NULLIF(ds.display_name, ''), ds.external_item_id),
			'integrated', NULL, ds.id, TRUE, ds.created_at, ds.updated_at
		FROM data_sources ds
		WHERE EXISTS (SELECT 1 FROM financial_investments fi WHERE fi.source_id = ds.id)
		  AND NOT EXISTS (SELECT 1 FROM investment_accounts ia WHERE ia.source_id = ds.id)
		ON CONFLICT (source_id) DO NOTHING`)
	return err
}

const accountColumns = `
	ia.id, ia.name, ia.kind, ia.currency_code, ia.source_id,
	COALESCE(NULLIF(ds.label, ''), NULLIF(ds.display_name, ''), ds.external_item_id),
	ia.financial_account_id, ia.is_active, fa.balance, ia.created_at, ia.updated_at`

const accountFrom = `
	FROM investment_accounts ia
	LEFT JOIN data_sources ds ON ds.id = ia.source_id
	LEFT JOIN financial_accounts fa ON fa.id = ia.financial_account_id`

func scanAccount(row interface{ Scan(...any) error }) (Account, error) {
	var (
		a                                                              Account
		currencyRaw, sourceIDRaw, sourceNameRaw, financialAccountIDRaw sql.NullString
		linkedBalanceRaw                                               sql.NullString
		createdAtRaw, updatedAtRaw                                     string
	)
	if err := row.Scan(dbtypes.ID(&a.ID), &a.Name, &a.Kind, &currencyRaw, dbtypes.ID(&sourceIDRaw), &sourceNameRaw,
		dbtypes.ID(&financialAccountIDRaw), dbtypes.Bool(&a.Active), dbtypes.Decimal(&linkedBalanceRaw), dbtypes.Time(&createdAtRaw), dbtypes.Time(&updatedAtRaw)); err != nil {
		return Account{}, err
	}
	if currencyRaw.Valid {
		v := currencyRaw.String
		a.CurrencyCode = &v
	}
	if sourceIDRaw.Valid {
		v := sourceIDRaw.String
		a.SourceID = &v
	}
	if sourceNameRaw.Valid {
		v := sourceNameRaw.String
		a.SourceDisplayName = &v
	}
	if financialAccountIDRaw.Valid {
		v := financialAccountIDRaw.String
		a.FinancialAccountID = &v
	}
	if linkedBalanceRaw.Valid {
		balance, err := decimal.NewFromString(linkedBalanceRaw.String)
		if err != nil {
			return Account{}, fmt.Errorf("parse linked financial account balance: %w", err)
		}
		a.LinkedCashBalance = &balance
	}
	var err error
	if a.CreatedAt, err = parseTimestamp(createdAtRaw); err != nil {
		return Account{}, err
	}
	if a.UpdatedAt, err = parseTimestamp(updatedAtRaw); err != nil {
		return Account{}, err
	}
	return a, nil
}

func getRawAccount(ctx context.Context, q Querier, id string) (Account, error) {
	account, err := scanAccount(q.QueryRowContext(ctx, `SELECT `+accountColumns+accountFrom+` WHERE ia.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrAccountNotFound
	}
	return account, err
}

func listRawAccounts(ctx context.Context, q Querier) ([]Account, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+accountColumns+accountFrom+` ORDER BY ia.kind, ia.name, ia.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Account{}
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

func hydrateAccountCash(ctx context.Context, q Querier, account *Account) (accountLedger, error) {
	ledger, err := replayAccount(ctx, q, account.ID)
	if err != nil {
		return accountLedger{}, err
	}
	account.CashBalance = ledger.cash
	return ledger, nil
}

func ListAccounts(ctx context.Context, q Querier) ([]Account, error) {
	if err := EnsureIntegratedAccounts(ctx, q); err != nil {
		return nil, err
	}
	accounts, err := listRawAccounts(ctx, q)
	if err != nil {
		return nil, err
	}
	for i := range accounts {
		if _, err := hydrateAccountCash(ctx, q, &accounts[i]); err != nil {
			return nil, err
		}
	}
	return accounts, nil
}

func GetAccount(ctx context.Context, q Querier, id string) (Account, error) {
	if err := EnsureIntegratedAccounts(ctx, q); err != nil {
		return Account{}, err
	}
	a, err := getRawAccount(ctx, q, id)
	if err != nil {
		return Account{}, err
	}
	_, err = hydrateAccountCash(ctx, q, &a)
	return a, err
}

func financialAccountIsBRL(ctx context.Context, q Querier, id string) error {
	var currency, accountType sql.NullString
	err := q.QueryRowContext(ctx, `SELECT currency_code, account_type FROM financial_accounts WHERE id = ?`, id).Scan(&currency, &accountType)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidInput
	}
	if err != nil {
		return err
	}
	if !currency.Valid || currency.String != "BRL" || accountType.String == "CREDIT" {
		return ErrInvalidInput
	}
	return nil
}

// financialAccountIsFree refuses a bank account another custody already
// represents. Linking it twice would count the same cash in two custodies, and
// the UNIQUE index would only report that as a storage failure.
func financialAccountIsFree(ctx context.Context, q Querier, financialAccountID, exceptID string) error {
	var linked bool
	if err := q.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM investment_accounts WHERE financial_account_id = ? AND id <> ?)`,
		financialAccountID, exceptID).Scan(dbtypes.Bool(&linked)); err != nil {
		return err
	}
	if linked {
		return ErrFinancialAccountLinked
	}
	return nil
}

// validateFinancialAccountLink runs the checks a link must pass: the bank
// account exists in BRL, is not a card, and is not already someone's custody.
func validateFinancialAccountLink(ctx context.Context, q Querier, financialAccountID, exceptID string) error {
	if err := financialAccountIsBRL(ctx, q, financialAccountID); err != nil {
		return err
	}
	return financialAccountIsFree(ctx, q, financialAccountID, exceptID)
}

func CreateAccount(ctx context.Context, q Querier, in AccountInput) (Account, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Account{}, ErrInvalidInput
	}
	if in.FinancialAccountID != nil {
		if err := validateFinancialAccountLink(ctx, q, *in.FinancialAccountID, ""); err != nil {
			return Account{}, err
		}
	}
	now := time.Now().UTC()
	id := uuid.NewString()
	if _, err := q.ExecContext(ctx, `
		INSERT INTO investment_accounts (
			id, name, kind, currency_code, financial_account_id, is_active, created_at, updated_at
		) VALUES (?, ?, 'manual', 'BRL', ?, TRUE, ?, ?)`,
		id, name, nullableString(in.FinancialAccountID), db.FormatTime(now), db.FormatTime(now)); err != nil {
		return Account{}, err
	}
	return GetAccount(ctx, q, id)
}

// UpdateAccount rewrites the locally owned columns of a custody account in a
// single statement. An integrated grouping only accepts a new linked financial
// account: its name mirrors the provider and it cannot be switched off here,
// so either of those changes is refused with ErrIntegratedReadOnly before
// anything is written, and a request the grouping cannot honour never
// half-applies the link. Manual accounts may also be renamed and toggled
// active; a nil Active keeps the current flag.
func UpdateAccount(ctx context.Context, q Querier, id string, in AccountInput) (Account, error) {
	current, err := getRawAccount(ctx, q, id)
	if err != nil {
		return Account{}, err
	}
	if in.FinancialAccountID != nil {
		if err := validateFinancialAccountLink(ctx, q, *in.FinancialAccountID, id); err != nil {
			return Account{}, err
		}
	}
	name := strings.TrimSpace(in.Name)
	active := current.Active
	if in.Active != nil {
		active = *in.Active
	}
	if current.Kind == AccountKindIntegrated {
		if (name != "" && name != current.Name) || active != current.Active {
			return Account{}, ErrIntegratedReadOnly
		}
		name = current.Name
	} else if name == "" {
		return Account{}, ErrInvalidInput
	}
	_, err = q.ExecContext(ctx, `UPDATE investment_accounts SET name = ?, financial_account_id = ?, is_active = ?, updated_at = ? WHERE id = ?`,
		name, nullableString(in.FinancialAccountID), active, db.FormatTime(time.Now()), id)
	if err != nil {
		return Account{}, err
	}
	return GetAccount(ctx, q, id)
}

func DeleteAccount(ctx context.Context, q Querier, id string) error {
	a, err := getRawAccount(ctx, q, id)
	if err != nil {
		return err
	}
	if a.Kind != AccountKindManual {
		return ErrIntegratedReadOnly
	}
	var positionCount, operationCount int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM investment_positions WHERE account_id = ?`, id).Scan(&positionCount); err != nil {
		return err
	}
	if positionCount > 0 {
		return ErrAccountHasPositions
	}
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM investment_operations WHERE account_id = ?`, id).Scan(&operationCount); err != nil {
		return err
	}
	if operationCount > 0 {
		return ErrAccountHasOperations
	}
	_, err = q.ExecContext(ctx, `DELETE FROM investment_accounts WHERE id = ?`, id)
	return err
}

func requireManualAccount(ctx context.Context, q Querier, id string) (Account, error) {
	a, err := getRawAccount(ctx, q, id)
	if err != nil {
		return Account{}, err
	}
	if a.Kind != AccountKindManual {
		return Account{}, ErrIntegratedReadOnly
	}
	return a, nil
}

func accountEffectiveCash(a Account) decimal.Decimal {
	if a.LinkedCashBalance != nil {
		return *a.LinkedCashBalance
	}
	if a.Kind == AccountKindIntegrated {
		return decimal.Zero
	}
	return a.CashBalance
}
