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

// The partial unique indexes that allow one bank line to be split between
// operations while refusing the same parcel twice. Two spellings each
// because SQLite reports the indexed columns and Postgres the index name.
const (
	constraintReconciliationTransactionSQLite             = "investment_reconciliations.financial_transaction_id"
	constraintReconciliationTransactionPostgres           = "uq_investment_reconciliation_operation_financial_transaction"
	constraintReconciliationInvestmentTransactionSQLite   = "investment_reconciliations.financial_investment_transaction_id"
	constraintReconciliationInvestmentTransactionPostgres = "uq_investment_reconciliation_operation_financial_investment_transaction"
)

const reconciliationColumns = `
	id, operation_id, financial_transaction_id, financial_investment_transaction_id,
	amount, created_at`

type ReconciliationFilter struct {
	OperationID            *string
	FinancialTransactionID *string
}

func scanReconciliation(row interface{ Scan(...any) error }) (Reconciliation, error) {
	var (
		r                                      Reconciliation
		transactionID, investmentTransactionID sql.NullString
		amountRaw, createdAtRaw                string
	)
	if err := row.Scan(dbtypes.ID(&r.ID), dbtypes.ID(&r.OperationID), dbtypes.ID(&transactionID), dbtypes.ID(&investmentTransactionID),
		dbtypes.Decimal(&amountRaw), dbtypes.Time(&createdAtRaw)); err != nil {
		return Reconciliation{}, err
	}
	if transactionID.Valid {
		v := transactionID.String
		r.FinancialTransactionID = &v
	}
	if investmentTransactionID.Valid {
		v := investmentTransactionID.String
		r.FinancialInvestmentTransactionID = &v
	}
	var err error
	if r.Amount, err = decimal.NewFromString(amountRaw); err != nil {
		return Reconciliation{}, fmt.Errorf("parse reconciliation amount: %w", err)
	}
	if r.CreatedAt, err = parseTimestamp(createdAtRaw); err != nil {
		return Reconciliation{}, err
	}
	return r, nil
}

func ListReconciliations(ctx context.Context, q Querier, filter ReconciliationFilter) ([]Reconciliation, error) {
	clauses := []string{}
	args := []any{}
	if filter.OperationID != nil {
		clauses = append(clauses, `operation_id = ?`)
		args = append(args, *filter.OperationID)
	}
	if filter.FinancialTransactionID != nil {
		clauses = append(clauses, `financial_transaction_id = ?`)
		args = append(args, *filter.FinancialTransactionID)
	}
	query := `SELECT ` + reconciliationColumns + ` FROM investment_reconciliations`
	if len(clauses) > 0 {
		query += ` WHERE ` + strings.Join(clauses, ` AND `)
	}
	rows, err := q.QueryContext(ctx, query+` ORDER BY created_at, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Reconciliation{}
	for rows.Next() {
		r, err := scanReconciliation(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func getReconciliation(ctx context.Context, q Querier, id string) (Reconciliation, error) {
	r, err := scanReconciliation(q.QueryRowContext(ctx,
		`SELECT `+reconciliationColumns+` FROM investment_reconciliations WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Reconciliation{}, ErrReconciliationNotFound
	}
	return r, err
}

// reconcilableKind reports whether a bank line can be the same event as this
// operation. A buy or sell only moves money inside the custody account, a
// valuation moves nothing and an initial balance is the book's starting
// point, so none of them ever crosses the bank boundary.
func reconcilableKind(kind OperationKind) bool {
	switch kind {
	case OperationDeposit, OperationWithdrawal, OperationIncome, OperationFee, OperationTax:
		return true
	default:
		return false
	}
}

// bankOutflowKind reports which way the bank line must point: money leaves
// the bank to fund the custody account or to pay its costs, and arrives when
// the investment pays back.
func bankOutflowKind(kind OperationKind) bool {
	switch kind {
	case OperationDeposit, OperationFee, OperationTax:
		return true
	default:
		return false
	}
}

// financialTransactionValue reads the bank line in its own account currency,
// the value the rest of the application reports on. An absent value leaves
// the direction unknowable, which is a refusal rather than a guess.
//
// An ignored line is refused as well: the user already excluded it from the
// totals, so it cannot at the same time be the money that funded an
// operation (see UnlinkTransactionIfPresent for the reverse order).
func financialTransactionValue(ctx context.Context, q Querier, id string) (decimal.Decimal, error) {
	var raw, currency sql.NullString
	var inclusion string
	err := q.QueryRowContext(ctx, `
  SELECT COALESCE(t.amount_in_account_currency, t.amount),
   CASE WHEN t.amount_in_account_currency IS NOT NULL THEN a.currency_code ELSE COALESCE(t.currency_code, a.currency_code) END,
   COALESCE(tid.state, 'considered')
  FROM financial_transactions t JOIN financial_accounts a ON a.id = t.account_id
  LEFT JOIN transaction_inclusion_decisions tid ON tid.transaction_id = t.id
  WHERE t.id = ? AND t.deleted_at IS NULL`, id).Scan(dbtypes.Decimal(&raw), &currency, &inclusion)
	if errors.Is(err, sql.ErrNoRows) {
		return decimal.Zero, ErrInvalidReconciliationLink
	}
	if err != nil {
		return decimal.Zero, err
	}
	if !raw.Valid || !currency.Valid || currency.String != "BRL" {
		return decimal.Zero, ErrInvalidReconciliationLink
	}
	if money.InclusionState(inclusion) == money.Ignored {
		return decimal.Zero, ErrInvalidReconciliationLink
	}
	value, err := decimal.NewFromString(raw.String)
	if err != nil {
		return decimal.Zero, fmt.Errorf("parse financial transaction amount: %w", err)
	}
	if value.IsZero() {
		return decimal.Zero, ErrInvalidReconciliationLink
	}
	return value, nil
}

// sumReconciliations adds up in Go, not with SQL SUM: the amounts are exact
// decimal text and must not be routed through the database's float math.
func sumReconciliations(ctx context.Context, q Querier, column, id string) (decimal.Decimal, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT amount FROM investment_reconciliations WHERE `+column+` = ?`, id)
	if err != nil {
		return decimal.Zero, err
	}
	defer rows.Close()
	total := decimal.Zero
	for rows.Next() {
		var raw string
		if err := rows.Scan(dbtypes.Decimal(&raw)); err != nil {
			return decimal.Zero, err
		}
		amount, err := decimal.NewFromString(raw)
		if err != nil {
			return decimal.Zero, fmt.Errorf("parse reconciliation amount: %w", err)
		}
		total = total.Add(amount)
	}
	return total, rows.Err()
}

// validateOperationLinks re-checks an edited operation against the links it
// already carries: a correction may not leave a bank line explained by more
// than the operation is worth, nor turn the operation into something that
// movement never was.
func validateOperationLinks(ctx context.Context, q Querier, op Operation) error {
	links, err := ListReconciliations(ctx, q, ReconciliationFilter{OperationID: &op.ID})
	if err != nil || len(links) == 0 {
		return err
	}
	if !reconcilableKind(op.Kind) {
		return ErrInvalidReconciliationLink
	}
	total := decimal.Zero
	for _, link := range links {
		total = total.Add(link.Amount)
		if link.FinancialInvestmentTransactionID != nil {
			if _, err := investmentTransactionValue(ctx, q, op, *link.FinancialInvestmentTransactionID); err != nil {
				return err
			}
		}
		if link.FinancialTransactionID == nil {
			continue
		}
		value, err := financialTransactionValue(ctx, q, *link.FinancialTransactionID)
		if err != nil {
			return err
		}
		if value.IsNegative() != bankOutflowKind(op.Kind) {
			return ErrInvalidReconciliationLink
		}
	}
	if total.GreaterThan(op.Amount) {
		return ErrReconciliationConflict
	}
	return nil
}

func requireUnlinkedPair(ctx context.Context, q Querier, operationID string, transactionID, investmentTransactionID *string) error {
	for _, link := range []struct {
		column string
		value  *string
	}{
		{`financial_transaction_id`, transactionID},
		{`financial_investment_transaction_id`, investmentTransactionID},
	} {
		if link.value == nil {
			continue
		}
		var exists bool
		if err := q.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM investment_reconciliations
				WHERE operation_id = ? AND `+link.column+` = ?
			)`, operationID, *link.value).Scan(dbtypes.Bool(&exists)); err != nil {
			return err
		}
		if exists {
			return ErrReconciliationDuplicate
		}
	}
	return nil
}

// CreateReconciliation records that a bank line and/or an imported investment
// movement is the same event as a local operation. It is reporting only: no
// balance and no cash curve changes, which is why it never replays the
// ledger. The user has already confirmed the match — a provider label is
// never enough to infer one.
func CreateReconciliation(ctx context.Context, conn *sql.DB, in ReconciliationInput) (Reconciliation, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return Reconciliation{}, err
	}
	defer tx.Rollback()
	result, err := createReconciliation(ctx, tx, in)
	if err != nil {
		return Reconciliation{}, err
	}
	if err := tx.Commit(); err != nil {
		return Reconciliation{}, err
	}
	return result, nil
}

func createReconciliation(ctx context.Context, tx *sql.Tx, in ReconciliationInput) (Reconciliation, error) {
	if !in.Amount.IsPositive() {
		return Reconciliation{}, ErrInvalidInput
	}
	transactionID := trimOptional(in.FinancialTransactionID)
	investmentTransactionID := trimOptional(in.FinancialInvestmentTransactionID)
	if transactionID == nil && investmentTransactionID == nil {
		return Reconciliation{}, ErrInvalidReconciliationLink
	}

	for _, lock := range []struct{ table, id string }{
		{"financial_transactions", derefOrEmpty(transactionID)},
		{"financial_investment_transactions", derefOrEmpty(investmentTransactionID)},
	} {
		if lock.id == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE `+lock.table+` SET id = id WHERE id = ?`, lock.id); err != nil {
			return Reconciliation{}, err
		}
	}
	operationID := strings.TrimSpace(in.OperationID)
	if operationID == "" {
		if investmentTransactionID == nil {
			return Reconciliation{}, ErrInvalidReconciliationLink
		}
		pivot, reused, err := ensureSyncedPivot(ctx, tx, *investmentTransactionID)
		if err != nil {
			return Reconciliation{}, err
		}
		operationID = pivot
		// The pivot already carries the movement link; a further parcel can
		// only add a bank line, and the operation amount still caps the sum.
		if reused {
			if transactionID == nil {
				return Reconciliation{}, ErrReconciliationDuplicate
			}
			investmentTransactionID = nil
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE investment_operations SET id = id WHERE id = ?`, operationID); err != nil {
		return Reconciliation{}, err
	}
	operation, err := GetOperation(ctx, tx, operationID)
	if err != nil {
		return Reconciliation{}, err
	}
	if !reconcilableKind(operation.Kind) {
		return Reconciliation{}, ErrInvalidReconciliationLink
	}
	if err := requireUnlinkedPair(ctx, tx, operationID, transactionID, investmentTransactionID); err != nil {
		return Reconciliation{}, err
	}
	linked, err := sumReconciliations(ctx, tx, `operation_id`, operationID)
	if err != nil {
		return Reconciliation{}, err
	}
	if linked.Add(in.Amount).GreaterThan(operation.Amount) {
		return Reconciliation{}, ErrReconciliationConflict
	}
	if transactionID != nil {
		value, err := financialTransactionValue(ctx, tx, *transactionID)
		if err != nil {
			return Reconciliation{}, err
		}
		if value.IsNegative() != bankOutflowKind(operation.Kind) {
			return Reconciliation{}, ErrInvalidReconciliationLink
		}
		allocated, err := sumReconciliations(ctx, tx, `financial_transaction_id`, *transactionID)
		if err != nil {
			return Reconciliation{}, err
		}
		if allocated.Add(in.Amount).GreaterThan(value.Abs()) {
			return Reconciliation{}, ErrReconciliationConflict
		}
	}
	if investmentTransactionID != nil {
		value, err := investmentTransactionValue(ctx, tx, operation, *investmentTransactionID)
		if err != nil {
			return Reconciliation{}, err
		}
		allocated, err := sumReconciliations(ctx, tx, `financial_investment_transaction_id`, *investmentTransactionID)
		if err != nil {
			return Reconciliation{}, err
		}
		if allocated.Add(in.Amount).GreaterThan(value) {
			return Reconciliation{}, ErrReconciliationConflict
		}
	}

	id := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO investment_reconciliations (
			id, operation_id, financial_transaction_id,
			financial_investment_transaction_id, amount, created_at
		) VALUES (?, ?, ?, ?, ?, ?)`,
		id, operationID, nullableString(transactionID), nullableString(investmentTransactionID),
		money.CanonicalDecimal(in.Amount), db.FormatTime(time.Now())); err != nil {
		if db.IsUniqueViolationOn(err,
			constraintReconciliationTransactionSQLite, constraintReconciliationTransactionPostgres,
			constraintReconciliationInvestmentTransactionSQLite, constraintReconciliationInvestmentTransactionPostgres) {
			return Reconciliation{}, ErrReconciliationDuplicate
		}
		return Reconciliation{}, err
	}
	return getReconciliation(ctx, tx, id)
}

// DeleteReconciliation undoes a link. What is reportable is derived from the
// rows that remain, so the bank line goes back to ordinary reporting on its
// own. A derived provider pivot only exists to be linked to: once its last
// link is gone it is removed, so the provider movement is again a plain
// import rather than an orphan local operation.
func DeleteReconciliation(ctx context.Context, conn *sql.DB, id string) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	link, err := getReconciliation(ctx, tx, id)
	if err != nil {
		return err
	}
	if err := removeLink(ctx, tx, link); err != nil {
		return err
	}
	return tx.Commit()
}

// UnlinkTransactionIfPresent drops every link a bank line carries, with the
// same pivot bookkeeping as DeleteReconciliation. It runs inside the caller's
// transaction when the line becomes ignored: money the user excluded from the
// totals can no longer be the parcel that funded an operation, and leaving
// the link would hide the operation's amount from the reports while the line
// itself is hidden too. A line with no links is a no-op.
func UnlinkTransactionIfPresent(ctx context.Context, q Querier, financialTransactionID string) error {
	links, err := ListReconciliations(ctx, q, ReconciliationFilter{FinancialTransactionID: &financialTransactionID})
	if err != nil {
		return err
	}
	for _, link := range links {
		if err := removeLink(ctx, q, link); err != nil {
			return err
		}
	}
	return nil
}

// removeLink deletes one row and keeps the derived pivot consistent: the
// movement id it may have carried is handed to a sibling parcel, and a pivot
// left with no parcel at all is removed.
func removeLink(ctx context.Context, q Querier, link Reconciliation) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM investment_reconciliations WHERE id = ?`, link.ID); err != nil {
		return err
	}
	if err := keepPivotReachable(ctx, q, link); err != nil {
		return err
	}
	_, err := q.ExecContext(ctx, `
		DELETE FROM investment_operations
		WHERE id = ? AND source = 'synced'
		  AND NOT EXISTS (SELECT 1 FROM investment_reconciliations WHERE operation_id = ?)`,
		link.OperationID, link.OperationID)
	return err
}

// keepPivotReachable moves the movement id from a deleted parcel onto the
// oldest sibling still on the same derived pivot. Only the first parcel of a
// pivot records financial_investment_transaction_id, and ensureSyncedPivot
// finds the pivot through that column alone: without the hand-off the pivot
// would survive (it still has parcels) yet be invisible, and the next parcel
// would derive a second pivot with the movement's full capacity again. On a
// manual operation the movement link is a per-parcel user decision, so
// nothing moves there.
func keepPivotReachable(ctx context.Context, q Querier, link Reconciliation) error {
	if link.FinancialInvestmentTransactionID == nil {
		return nil
	}
	_, err := q.ExecContext(ctx, `
		UPDATE investment_reconciliations
		SET financial_investment_transaction_id = ?
		WHERE id = (
			SELECT r.id FROM investment_reconciliations r
			JOIN investment_operations o ON o.id = r.operation_id
			WHERE r.operation_id = ? AND o.source = 'synced'
			  AND r.financial_investment_transaction_id IS NULL
			ORDER BY r.created_at, r.id LIMIT 1
		)`, *link.FinancialInvestmentTransactionID, link.OperationID)
	return err
}

// ensureSyncedPivot gives a provider movement the local operation a
// reconciliation pivots on. The kind follows the direction normalized at
// ingestion (money entering the investment is an aporte, leaving it a
// resgate, a payout an income); the user confirming the bank match is what
// turns the provider label into a classification. The pivot is read-only
// and never replays the integrated custody, whose balances stay the
// provider's.
func ensureSyncedPivot(ctx context.Context, q Querier, movementID string) (id string, reused bool, err error) {
	err = q.QueryRowContext(ctx, `
		SELECT o.id FROM investment_operations o
		JOIN investment_reconciliations r ON r.operation_id = o.id
		WHERE o.source = 'synced' AND r.financial_investment_transaction_id = ?
		ORDER BY o.created_at, o.id LIMIT 1`, movementID).Scan(dbtypes.ID(&id))
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", false, err
	}

	var amountRaw, currency, direction, movementType, name sql.NullString
	var tradeDate, occurredAt sql.NullTime
	var sourceID string
	err = q.QueryRowContext(ctx, `
		SELECT t.amount, i.currency_code, t.direction, t.movement_type, t.trade_date, t.occurred_at, i.name, t.source_id
		FROM financial_investment_transactions t JOIN financial_investments i ON i.id = t.investment_id
		WHERE t.id = ?`, movementID).Scan(dbtypes.Decimal(&amountRaw), &currency, &direction, &movementType, dbtypes.Date(&tradeDate), dbtypes.Time(&occurredAt), &name, dbtypes.ID(&sourceID))
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, ErrInvalidReconciliationLink
	}
	if err != nil {
		return "", false, err
	}
	if !amountRaw.Valid || currency.String != "BRL" {
		return "", false, ErrInvalidReconciliationLink
	}
	amount, err := decimal.NewFromString(amountRaw.String)
	if err != nil || amount.IsZero() {
		return "", false, ErrInvalidReconciliationLink
	}
	var kind OperationKind
	switch direction.String {
	case "inflow":
		kind = OperationDeposit
	case "outflow":
		kind = OperationWithdrawal
		if strings.EqualFold(movementType.String, "INTEREST") {
			kind = OperationIncome
		}
	default:
		return "", false, ErrInvalidReconciliationLink
	}
	dateRaw := tradeDate
	if !dateRaw.Valid {
		dateRaw = occurredAt
	}
	if !dateRaw.Valid {
		return "", false, ErrInvalidReconciliationLink
	}
	occurredOn := dateRaw.Time
	if err := EnsureIntegratedAccounts(ctx, q); err != nil {
		return "", false, err
	}
	labelParts := []string{}
	for _, part := range []string{strings.TrimSpace(movementType.String), strings.TrimSpace(name.String)} {
		if part != "" {
			labelParts = append(labelParts, part)
		}
	}
	notes := strings.Join(labelParts, " · ")
	id = uuid.NewString()
	now := db.FormatTime(time.Now())
	if _, err := q.ExecContext(ctx, `INSERT INTO investment_operations (
		id, account_id, position_id, kind, occurred_on, amount, quantity, unit_price,
		fees, taxes, notes, source, created_at, updated_at
	) VALUES (?, ?, NULL, ?, ?, ?, NULL, NULL, '0', '0', ?, 'synced', ?, ?)`,
		id, "integrated:"+sourceID, string(kind), formatDate(occurredOn), money.CanonicalDecimal(amount.Abs()),
		nullableString(&notes), now, now); err != nil {
		return "", false, err
	}
	return id, false, nil
}

// Provider movement direction is normalized at ingestion. An unknown direction
// cannot establish a match. A manually tracked custody can reference imported
// evidence; an integrated custody must reference its own provider connection.
func investmentTransactionValue(ctx context.Context, q Querier, op Operation, id string) (decimal.Decimal, error) {
	var amount, currency, direction sql.NullString
	var sourceID string
	err := q.QueryRowContext(ctx, `SELECT t.amount, i.currency_code, t.direction, t.source_id
  FROM financial_investment_transactions t JOIN financial_investments i ON i.id = t.investment_id
  WHERE t.id = ?`, id).Scan(dbtypes.Decimal(&amount), &currency, &direction, dbtypes.ID(&sourceID))
	if errors.Is(err, sql.ErrNoRows) {
		return decimal.Zero, ErrInvalidReconciliationLink
	}
	if err != nil {
		return decimal.Zero, err
	}
	expected := "outflow"
	if op.Kind == OperationDeposit {
		expected = "inflow"
	}
	if !amount.Valid || currency.String != "BRL" || direction.String != expected {
		return decimal.Zero, ErrInvalidReconciliationLink
	}
	account, err := getRawAccount(ctx, q, op.AccountID)
	if err != nil {
		return decimal.Zero, err
	}
	if account.SourceID != nil && *account.SourceID != sourceID {
		return decimal.Zero, ErrInvalidReconciliationLink
	}
	value, err := decimal.NewFromString(amount.String)
	if err != nil || value.IsZero() {
		return decimal.Zero, ErrInvalidReconciliationLink
	}
	return value.Abs(), nil
}
