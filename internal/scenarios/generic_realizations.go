package scenarios

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
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/transactions"
)

type RealizationRelation string

const (
	RelationSettlement       RealizationRelation = "settlement"
	RelationAllocation       RealizationRelation = "allocation"
	RelationReconciliation   RealizationRelation = "reconciliation"
	RealizationStateLinked   string              = "linked"
	RealizationStateDetached string              = "detached"
)

var (
	ErrInvalidEventKey            = errors.New("invalid planned event key")
	ErrInvalidRealization         = errors.New("invalid scenario realization")
	ErrPlannedRealizationNotFound = errors.New("planned event realization not found")
	ErrTransactionUnavailable     = errors.New("real transaction not found")
	ErrTransactionAlreadyRealized = errors.New("real transaction already satisfies another complete event")
)

// GenericRealization is the read model of scenario_realizations. It is
// intentionally independent of the old payable-link/allocation DTOs.
type GenericRealization struct {
	ID                    string
	ScenarioID            string
	ScenarioTransactionID *string
	OccurrenceDate        *time.Time
	TransactionID         *string
	RelationType          RealizationRelation
	State                 string
	Origin                string
	AllocatedAmount       *decimal.Decimal
	LinkedAmount          *decimal.Decimal
	CreatedAt             time.Time
}

type RealizationWrite struct {
	State           string
	TransactionID   *string
	AllocatedAmount decimal.Decimal
	Origin          string
}

// ListRealizations is the read API for every relation owned by one scenario,
// whatever its kind. It returns the normalized relation shape, which is what
// lets a caller treat a settlement, an allocation and an occurrence decision
// as the one thing they are: this scenario met reality here.
func ListRealizations(ctx context.Context, q Querier, scenarioID string) ([]GenericRealization, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, scenario_id, scenario_transaction_id, occurrence_date, transaction_id,
		       relation_type, state, origin, allocated_amount, linked_amount, created_at
		FROM scenario_realizations
		WHERE scenario_id = ?
		ORDER BY created_at, id`, scenarioID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []GenericRealization
	for rows.Next() {
		var (
			value                                  GenericRealization
			transactionID, scenarioTransactionID   sql.NullString
			occurrenceRaw, allocatedRaw, linkedRaw sql.NullString
			createdAtRaw                           string
			relationRaw                            string
		)
		if err := rows.Scan(
			dbtypes.ID(&value.ID), dbtypes.ID(&value.ScenarioID), dbtypes.ID(&scenarioTransactionID), dbtypes.Date(&occurrenceRaw), dbtypes.ID(&transactionID),
			&relationRaw, &value.State, &value.Origin, dbtypes.Decimal(&allocatedRaw), dbtypes.Decimal(&linkedRaw), dbtypes.Time(&createdAtRaw),
		); err != nil {
			return nil, err
		}
		value.RelationType = RealizationRelation(relationRaw)
		if scenarioTransactionID.Valid {
			value.ScenarioTransactionID = &scenarioTransactionID.String
		}
		if occurrenceRaw.Valid {
			parsed, err := dbtypes.ParseDate(occurrenceRaw.String)
			if err != nil {
				return nil, err
			}
			value.OccurrenceDate = &parsed
		}
		if transactionID.Valid {
			value.TransactionID = &transactionID.String
		}
		var parseErr error
		if allocatedRaw.Valid {
			parsed, err := decimal.NewFromString(allocatedRaw.String)
			if err != nil {
				return nil, err
			}
			value.AllocatedAmount = &parsed
		}
		if linkedRaw.Valid {
			parsed, err := decimal.NewFromString(linkedRaw.String)
			if err != nil {
				return nil, err
			}
			value.LinkedAmount = &parsed
		}
		value.CreatedAt, parseErr = dbtypes.ParseTime(createdAtRaw)
		if parseErr != nil {
			return nil, parseErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

// RealizeEvent creates or replaces the realization for one stable event key.
// It is the single write path behind every endpoint that records one. A linked event always references a real financial transaction;
// a detached event explicitly carries no transaction and blocks automatic
// recurrence matching.
func RealizeEvent(ctx context.Context, conn *sql.DB, scenarioID, eventKey string, write RealizationWrite) (GenericRealization, error) {
	scenario, err := GetScenario(ctx, conn, scenarioID)
	if err != nil {
		return GenericRealization{}, err
	}
	if write.Origin == "" {
		write.Origin = "manual"
	}
	if write.State != RealizationStateLinked && write.State != RealizationStateDetached {
		return GenericRealization{}, fmt.Errorf("%w: state", ErrInvalidRealization)
	}
	if write.State == RealizationStateDetached {
		write.TransactionID = nil
		write.AllocatedAmount = decimal.Zero
	}
	if write.State == RealizationStateLinked && (write.TransactionID == nil || strings.TrimSpace(*write.TransactionID) == "") {
		return GenericRealization{}, fmt.Errorf("%w: transaction_id is required", ErrInvalidRealization)
	}
	if write.State == RealizationStateLinked {
		var exists int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(1) FROM financial_transactions WHERE id = ? AND deleted_at IS NULL`, *write.TransactionID).Scan(&exists); err != nil {
			return GenericRealization{}, err
		}
		if exists == 0 {
			return GenericRealization{}, ErrTransactionUnavailable
		}
	}

	relation, transactionID, occurrenceDate, scenarioTransactionID, err := eventIdentity(ctx, conn, scenario, eventKey)
	if err != nil {
		return GenericRealization{}, err
	}
	if relation == RelationAllocation && write.State == RealizationStateDetached {
		return GenericRealization{}, fmt.Errorf("%w: allocations must be linked or deleted", ErrInvalidRealization)
	}
	if relation == RelationAllocation && scenario.PayableID != nil {
		var linked int
		if err := conn.QueryRowContext(ctx, `
			SELECT COUNT(1) FROM payable_transaction_links
			WHERE payable_id = ? AND transaction_id = ?`, *scenario.PayableID, *write.TransactionID).Scan(&linked); err != nil {
			return GenericRealization{}, err
		}
		if linked == 0 {
			return GenericRealization{}, fmt.Errorf("%w: transaction is not linked to the scenario payable", ErrInvalidRealization)
		}
	}
	if relation == RelationAllocation && scenario.PayableID == nil && write.State == RealizationStateLinked {
		planned, err := GetScenarioTransaction(ctx, conn, *scenarioTransactionID)
		if err != nil {
			return GenericRealization{}, err
		}
		if err := validateStandaloneTransaction(ctx, conn, planned.Amount, *write.TransactionID); err != nil {
			return GenericRealization{}, err
		}
	}
	if relation == RelationAllocation {
		if write.State == RealizationStateLinked && !write.AllocatedAmount.IsPositive() {
			// A generic event link may omit an allocation amount (the common
			// case for standalone scenarios). In that case the event's own
			// magnitude is the amount being realized; payable-plan clients can
			// still provide a positive partial allocation explicitly.
			var plannedAmountRaw string
			if err := conn.QueryRowContext(ctx,
				`SELECT amount FROM scenario_transactions WHERE id = ?`, *scenarioTransactionID,
			).Scan(dbtypes.Decimal(&plannedAmountRaw)); err != nil {
				return GenericRealization{}, err
			}
			plannedAmount, err := decimal.NewFromString(plannedAmountRaw)
			if err != nil {
				return GenericRealization{}, err
			}
			write.AllocatedAmount = plannedAmount.Abs()
		}
		if !write.AllocatedAmount.IsPositive() && write.State == RealizationStateLinked {
			return GenericRealization{}, fmt.Errorf("%w: allocated_amount must be positive", ErrInvalidRealization)
		}
	}
	if relation == RelationReconciliation && write.State == RealizationStateLinked {
		if err := validateRecurringTransaction(ctx, conn, scenario.ID, *write.TransactionID); err != nil {
			return GenericRealization{}, err
		}
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return GenericRealization{}, err
	}
	defer tx.Rollback()

	if relation == RelationAllocation {
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM scenario_realizations
			WHERE scenario_id = ? AND relation_type = 'allocation' AND scenario_transaction_id = ?`,
			scenarioID, *scenarioTransactionID); err != nil {
			return GenericRealization{}, err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM scenario_realizations
			WHERE scenario_id = ? AND relation_type = 'reconciliation' AND occurrence_date = ?`,
			scenarioID, formatDate(*occurrenceDate)); err != nil {
			return GenericRealization{}, err
		}
	}

	now := time.Now().UTC()
	row := GenericRealization{
		ID: uuid.NewString(), ScenarioID: scenarioID, ScenarioTransactionID: scenarioTransactionID,
		OccurrenceDate: occurrenceDate, TransactionID: transactionID, RelationType: relation,
		State: write.State, Origin: write.Origin, CreatedAt: now,
	}
	if write.State == RealizationStateLinked {
		row.TransactionID = write.TransactionID
		if relation == RelationAllocation {
			amount := write.AllocatedAmount
			row.AllocatedAmount = &amount
		} else {
			// The transaction below owns the only SQLite connection; reading
			// through conn here would wait forever with the application's
			// single-connection pool. Use the already-open transaction.
			row.LinkedAmount = linkedAmountSnapshot(ctx, tx, write.TransactionID)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO scenario_realizations (
			id, scenario_id, scenario_transaction_id, occurrence_date, transaction_id,
			relation_type, state, origin, allocated_amount, linked_amount, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.ID, row.ScenarioID, row.ScenarioTransactionID, nullableDate(row.OccurrenceDate), row.TransactionID,
		string(row.RelationType), row.State, row.Origin, decimalPtr(row.AllocatedAmount), decimalPtr(row.LinkedAmount), db.FormatTime(now),
	); err != nil {
		if isCompleteEventTransactionConflict(err) {
			return GenericRealization{}, ErrTransactionAlreadyRealized
		}
		return GenericRealization{}, err
	}
	if err := tx.Commit(); err != nil {
		return GenericRealization{}, err
	}
	return row, nil
}

// validateRecurringTransaction keeps the generic event endpoint aligned with
// the occurrence handler: a recurring occurrence can only be satisfied by a
// considered BRL transaction flowing in the schedule's direction. The
// database constraint still handles the cross-occurrence
// uniqueness race; this check supplies the domain-level rejection for an
// ignored, wrong-direction, or non-BRL transaction before opening the write
// transaction.
func validateRecurringTransaction(ctx context.Context, q Querier, scenarioID, transactionID string) error {
	var kindRaw string
	err := q.QueryRowContext(ctx, `
		SELECT cashflow_kind
		FROM scenario_recurring_schedules
		WHERE scenario_id = ?`, scenarioID).Scan(&kindRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: recurring schedule not found", ErrInvalidRealization)
	} else if err != nil {
		return err
	}

	item, found, err := transactions.GetItem(ctx, q, transactionID)
	if err != nil {
		return err
	}
	if !found {
		return ErrTransactionUnavailable
	}
	if eligibility := recurrences.EligibilityForItem(recurrences.Kind(kindRaw), item, false); !eligibility.Eligible {
		return fmt.Errorf("%w: transaction is not eligible for this recurring event", ErrInvalidRealization)
	}
	return nil
}

// validateStandaloneTransaction applies the recurring link rules to a
// standalone event: a considered BRL transaction flowing in the direction of
// the planned amount's sign (a zero amount accepts either).
func validateStandaloneTransaction(ctx context.Context, q Querier, planned decimal.Decimal, transactionID string) error {
	item, found, err := transactions.GetItem(ctx, q, transactionID)
	if err != nil {
		return err
	}
	if !found {
		return ErrTransactionUnavailable
	}
	kinds := []recurrences.Kind{recurrences.KindIncome, recurrences.KindExpense}
	switch {
	case planned.IsNegative():
		kinds = kinds[1:]
	case planned.IsPositive():
		kinds = kinds[:1]
	}
	for _, kind := range kinds {
		if recurrences.EligibilityForItem(kind, item, false).Eligible {
			return nil
		}
	}
	return fmt.Errorf("%w: transaction is not eligible for this event", ErrInvalidRealization)
}

// isCompleteEventTransactionConflict recognises exactly one constraint: the
// partial unique index that lets a transaction satisfy at most one complete
// event. Every other failure of that INSERT — an unknown scenario, a bad
// state — is a real error and must not be reported to the user as "pick a
// different transaction".
func isCompleteEventTransactionConflict(err error) bool {
	return db.IsUniqueViolationOn(err,
		db.ConstraintOneCompleteEventTransactionSQLite,
		db.ConstraintOneCompleteEventTransactionPostgres)
}

func eventIdentity(ctx context.Context, q Querier, scenario Scenario, eventKey string) (RealizationRelation, *string, *time.Time, *string, error) {
	prefix := "scenario:" + scenario.ID + ":"
	if !strings.HasPrefix(eventKey, prefix) {
		return "", nil, nil, nil, ErrInvalidEventKey
	}
	rest := strings.TrimPrefix(eventKey, prefix)
	switch {
	case strings.HasPrefix(rest, "transaction:"):
		id := strings.TrimPrefix(rest, "transaction:")
		if id == "" {
			return "", nil, nil, nil, ErrInvalidEventKey
		}
		if scenario.Kind != KindDebtPlan && scenario.Kind != KindReceivablePlan && scenario.Kind != KindStandalone {
			return "", nil, nil, nil, ErrInvalidEventKey
		}
		planned, err := GetScenarioTransaction(ctx, q, id)
		if err != nil {
			return "", nil, nil, nil, err
		}
		if planned.ScenarioID != scenario.ID {
			return "", nil, nil, nil, ErrInvalidEventKey
		}
		return RelationAllocation, nil, nil, &id, nil
	case strings.HasPrefix(rest, "occurrence:"):
		if scenario.Kind != KindRecurring {
			return "", nil, nil, nil, ErrInvalidEventKey
		}
		dateRaw := strings.TrimPrefix(rest, "occurrence:")
		date, err := dbtypes.ParseDate(dateRaw)
		if err != nil {
			return "", nil, nil, nil, ErrInvalidEventKey
		}
		if !isRecurringOccurrence(ctx, q, scenario.ID, date) {
			return "", nil, nil, nil, ErrInvalidEventKey
		}
		return RelationReconciliation, nil, &date, nil, nil
	default:
		return "", nil, nil, nil, ErrInvalidEventKey
	}
}

// isRecurringOccurrence keeps the generic event endpoint from persisting a
// realization for a date that the schedule can never emit. Occurrence dates
// are derived identities, so accepting an arbitrary date would create a row
// that neither the projector nor the occurrence endpoints could ever
// observe.
func isRecurringOccurrence(ctx context.Context, q Querier, scenarioID string, date time.Time) bool {
	var (
		cashflowKind, amountRaw, cadence, startRaw string
		categoryID, accountID                      sql.NullString
		dayOfMonth                                 int
		monthOfYear                                sql.NullInt64
		endRaw                                     sql.NullString
	)
	err := q.QueryRowContext(ctx, `
		SELECT cashflow_kind, amount, category_id, account_id, cadence,
		       day_of_month, month_of_year, start_date, end_date
		FROM scenario_recurring_schedules
		WHERE scenario_id = ?`, scenarioID).Scan(
		&cashflowKind, dbtypes.Decimal(&amountRaw), dbtypes.ID(&categoryID), dbtypes.ID(&accountID), &cadence,
		&dayOfMonth, &monthOfYear, dbtypes.Date(&startRaw), dbtypes.Date(&endRaw),
	)
	if err != nil {
		return false
	}
	amount, err := decimal.NewFromString(amountRaw)
	if err != nil {
		return false
	}
	start, err := dbtypes.ParseDate(startRaw)
	if err != nil {
		return false
	}
	var end *time.Time
	if endRaw.Valid {
		parsed, err := dbtypes.ParseDate(endRaw.String)
		if err != nil {
			return false
		}
		end = &parsed
	}
	var month *int
	if monthOfYear.Valid {
		value := int(monthOfYear.Int64)
		month = &value
	}
	schedule := recurrences.RecurringSchedule{
		CashflowKind: recurrences.Kind(cashflowKind), Amount: amount,
		CategoryID: nullableString(categoryID), AccountID: nullableString(accountID),
		Cadence: recurrences.Cadence(cadence), DayOfMonth: dayOfMonth,
		MonthOfYear: month, StartDate: start, EndDate: end,
	}
	return len(recurrences.OccurrencesInRange(schedule, date, date)) == 1
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func linkedAmountSnapshot(ctx context.Context, q Querier, transactionID *string) *decimal.Decimal {
	if transactionID == nil {
		return nil
	}
	var amountRaw, amountAccountRaw, currencyRaw, accountCurrencyRaw sql.NullString
	err := q.QueryRowContext(ctx, `
		SELECT ft.amount, ft.amount_in_account_currency, ft.currency_code, fa.currency_code
		FROM financial_transactions ft JOIN financial_accounts fa ON fa.id = ft.account_id
			WHERE ft.id = ? AND ft.deleted_at IS NULL`, *transactionID).Scan(dbtypes.Decimal(&amountRaw), dbtypes.Decimal(&amountAccountRaw), &currencyRaw, &accountCurrencyRaw)
	if err != nil {
		return nil
	}
	var amount *decimal.Decimal
	if amountAccountRaw.Valid && accountCurrencyRaw.Valid && accountCurrencyRaw.String == "BRL" {
		if parsed, err := decimal.NewFromString(amountAccountRaw.String); err == nil {
			amount = &parsed
		}
	} else if amountRaw.Valid && currencyRaw.Valid && currencyRaw.String == "BRL" {
		if parsed, err := decimal.NewFromString(amountRaw.String); err == nil {
			amount = &parsed
		}
	}
	if amount == nil {
		return nil
	}
	value := amount.Abs()
	return &value
}

func decimalPtr(value *decimal.Decimal) any {
	if value == nil {
		return nil
	}
	return money.CanonicalDecimal(*value)
}

func nullableDate(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatDate(*value)
}

// DeleteRealization removes the realization represented by eventKey without
// touching the scenario transaction, financial transaction, or any payable.
func DeletePlannedEventRealization(ctx context.Context, q Querier, scenarioID, eventKey string) error {
	scenario, err := GetScenario(ctx, q, scenarioID)
	if err != nil {
		return err
	}
	relation, _, occurrence, transaction, err := eventIdentity(ctx, q, scenario, eventKey)
	if err != nil {
		return err
	}
	var result sql.Result
	if relation == RelationAllocation {
		result, err = q.ExecContext(ctx, `
			DELETE FROM scenario_realizations
			WHERE scenario_id = ? AND relation_type = 'allocation' AND scenario_transaction_id = ?`, scenarioID, *transaction)
	} else {
		result, err = q.ExecContext(ctx, `
			DELETE FROM scenario_realizations
			WHERE scenario_id = ? AND relation_type = 'reconciliation' AND occurrence_date = ?`, scenarioID, formatDate(*occurrence))
	}
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrPlannedRealizationNotFound
	}
	return nil
}
