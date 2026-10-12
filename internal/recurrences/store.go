package recurrences

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/money"
)

// ErrNotFound is returned by Get/Update/SetActive/Delete when id has no
// matching row.
var ErrNotFound = errors.New("recurring commitment not found")

// ErrLinkedToAutomationRule is returned by Delete when the scenario is still
// targeted by an automation rule's reconcile action (ON DELETE RESTRICT on
// automation_rule_actions.scenario_id).
var ErrLinkedToAutomationRule = errors.New("recurring commitment is targeted by an automation rule's reconcile action")

// Querier is satisfied by both *sql.DB and *sql.Tx.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// dateLayout is the on-disk format for the date-only start_date/end_date
// columns — mirrors internal/scenarios' dateLayout.
const dateLayout = "2006-01-02"

func formatDate(t time.Time) string         { return t.UTC().Format(dateLayout) }
func parseDate(s string) (time.Time, error) { return dbtypes.ParseDate(s) }

// A recurring commitment is a Scenario of kind 'recurring' plus its row in
// scenario_recurring_schedules: identity, name and activation on the scenario,
// cash flow and calendar on the schedule. The two are always written together,
// so every read here is that join.
const selectCommitment = `
	SELECT s.id, s.name, r.cashflow_kind, r.amount, COALESCE(r.category_id, ''), r.account_id,
	       r.cadence, r.day_of_month, r.month_of_year, r.start_date, r.end_date,
	       s.is_active, s.created_at, s.updated_at
	FROM scenarios s
	JOIN scenario_recurring_schedules r ON r.scenario_id = s.id
	WHERE s.kind = 'recurring'`

// Write is the fields a create/update request supplies.
type Write struct {
	Name        string
	Kind        Kind
	Amount      decimal.Decimal
	CategoryID  string
	AccountID   *string
	Cadence     Cadence
	DayOfMonth  int
	MonthOfYear *int
	StartDate   time.Time
	EndDate     *time.Time
	IsActive    bool
}

// Create inserts a new recurring Scenario and its schedule in one
// transaction. Callers are responsible for calling
// RecurringCommitment.Validate() first (the HTTP layer does this before ever
// reaching the store).
//
// It returns the struct it wrote rather than reading the row back, because a
// create is the one write that supplies every field — including the
// timestamps. Update and SetActive do read back, for the mirror reason: they
// receive a partial write and would otherwise have to invent CreatedAt.
func Create(ctx context.Context, conn *sql.DB, write Write) (RecurringCommitment, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return RecurringCommitment{}, err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	c := RecurringCommitment{
		ID: uuid.NewString(), Name: write.Name, Kind: write.Kind, Amount: write.Amount,
		CategoryID: write.CategoryID, AccountID: write.AccountID, Cadence: write.Cadence,
		DayOfMonth: write.DayOfMonth, MonthOfYear: write.MonthOfYear, StartDate: write.StartDate,
		EndDate: write.EndDate, IsActive: write.IsActive, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO scenarios (id, kind, name, payable_id, is_active, is_accounting_source, created_at, updated_at)
		VALUES (?, 'recurring', ?, NULL, ?, 0, ?, ?)`,
		c.ID, c.Name, boolToInt(c.IsActive), db.FormatTime(now), db.FormatTime(now),
	); err != nil {
		return RecurringCommitment{}, err
	}
	if err := insertSchedule(ctx, tx, c); err != nil {
		return RecurringCommitment{}, err
	}
	if err := tx.Commit(); err != nil {
		return RecurringCommitment{}, err
	}
	return c, nil
}

// Update replaces every field of an existing commitment: the name and
// activation on the Scenario, the cash flow and calendar on its schedule.
// The schedule is one row per scenario, so it is updated in place — there is
// no list to rebuild, and an UPDATE cannot leave the commitment scheduleless
// the way a delete followed by a failing insert could.
func Update(ctx context.Context, conn *sql.DB, id string, write Write) (RecurringCommitment, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return RecurringCommitment{}, err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	c := RecurringCommitment{
		ID: id, Name: write.Name, Kind: write.Kind, Amount: write.Amount,
		CategoryID: write.CategoryID, AccountID: write.AccountID, Cadence: write.Cadence,
		DayOfMonth: write.DayOfMonth, MonthOfYear: write.MonthOfYear, StartDate: write.StartDate,
		EndDate: write.EndDate, IsActive: write.IsActive, UpdatedAt: now,
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE scenarios SET name = ?, is_active = ?, updated_at = ?
		WHERE id = ? AND kind = 'recurring'`,
		c.Name, boolToInt(c.IsActive), db.FormatTime(now), id)
	if err != nil {
		return RecurringCommitment{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return RecurringCommitment{}, ErrNotFound
	}
	res, err = tx.ExecContext(ctx, `
		UPDATE scenario_recurring_schedules SET
			cashflow_kind = ?, amount = ?, category_id = ?, account_id = ?, cadence = ?,
			day_of_month = ?, month_of_year = ?, start_date = ?, end_date = ?
		WHERE scenario_id = ?`,
		string(c.Kind), money.CanonicalDecimal(c.Amount), c.CategoryID, c.AccountID,
		string(c.Cadence), c.DayOfMonth, c.MonthOfYear, formatDate(c.StartDate),
		formatDatePtr(c.EndDate), id,
	)
	if err != nil {
		return RecurringCommitment{}, err
	}
	// A recurring Scenario whose schedule row is missing is not updatable —
	// it is the same half-formed state loadSchedules refuses to project. Left
	// unchecked this would commit the name change, then fail on the read-back
	// below, reporting a 404 for a write that already happened.
	if n, _ := res.RowsAffected(); n == 0 {
		return RecurringCommitment{}, ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return RecurringCommitment{}, err
	}
	// CreatedAt is the one field the write does not carry, so it comes from
	// the row rather than being invented here.
	return Get(ctx, conn, id)
}

// SetActive toggles is_active without touching any other field.
func SetActive(ctx context.Context, q Querier, id string, isActive bool) (RecurringCommitment, error) {
	res, err := q.ExecContext(ctx, `
		UPDATE scenarios SET is_active = ?, updated_at = ?
		WHERE id = ? AND kind = 'recurring'`,
		boolToInt(isActive), db.FormatTime(time.Now().UTC()), id)
	if err != nil {
		return RecurringCommitment{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return RecurringCommitment{}, ErrNotFound
	}
	// Like Update: the caller supplied only the flag, so the rest of the
	// commitment is read back rather than assumed.
	return Get(ctx, q, id)
}

// Delete removes a recurring Scenario, cascading its schedule and every
// occurrence decision recorded against it. Fails with
// ErrLinkedToAutomationRule if an automation rule's reconcile action still
// targets it (ON DELETE RESTRICT on automation_rule_actions.scenario_id) —
// remove or repoint that rule's action first.
func Delete(ctx context.Context, q Querier, id string) error {
	var linked int
	if err := q.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM automation_rule_actions
		WHERE action_type = 'reconcile' AND scenario_id = ?`, id).Scan(&linked); err != nil {
		return err
	}
	if linked > 0 {
		return ErrLinkedToAutomationRule
	}
	res, err := q.ExecContext(ctx, `DELETE FROM scenarios WHERE id = ? AND kind = 'recurring'`, id)
	if err != nil {
		if db.IsForeignKeyViolation(err) {
			return ErrLinkedToAutomationRule
		}
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanCommitment(scan func(dest ...any) error) (RecurringCommitment, error) {
	var (
		c                          RecurringCommitment
		kindRaw, cadenceRaw        string
		amountRaw                  string
		accountID                  sql.NullString
		monthOfYear                sql.NullInt64
		startDateRaw               string
		endDateRaw                 sql.NullString
		isActive                   int
		createdAtRaw, updatedAtRaw string
	)
	if err := scan(
		dbtypes.ID(&c.ID), &c.Name, &kindRaw, dbtypes.Decimal(&amountRaw), dbtypes.ID(&c.CategoryID), dbtypes.ID(&accountID),
		&cadenceRaw, &c.DayOfMonth, &monthOfYear, dbtypes.Date(&startDateRaw), dbtypes.Date(&endDateRaw), dbtypes.Bool(&isActive),
		dbtypes.Time(&createdAtRaw), dbtypes.Time(&updatedAtRaw),
	); err != nil {
		return RecurringCommitment{}, err
	}
	c.Kind = Kind(kindRaw)
	c.Cadence = Cadence(cadenceRaw)
	c.IsActive = isActive != 0
	if accountID.Valid {
		c.AccountID = &accountID.String
	}
	if monthOfYear.Valid {
		m := int(monthOfYear.Int64)
		c.MonthOfYear = &m
	}
	var err error
	if c.Amount, err = decimal.NewFromString(amountRaw); err != nil {
		return RecurringCommitment{}, err
	}
	if c.StartDate, err = parseDate(startDateRaw); err != nil {
		return RecurringCommitment{}, err
	}
	if endDateRaw.Valid {
		end, err := parseDate(endDateRaw.String)
		if err != nil {
			return RecurringCommitment{}, err
		}
		c.EndDate = &end
	}
	if c.CreatedAt, err = dbtypes.ParseTime(createdAtRaw); err != nil {
		return RecurringCommitment{}, err
	}
	if c.UpdatedAt, err = dbtypes.ParseTime(updatedAtRaw); err != nil {
		return RecurringCommitment{}, err
	}
	return c, nil
}

// Get fetches a single commitment by its Scenario id.
func Get(ctx context.Context, q Querier, id string) (RecurringCommitment, error) {
	row := q.QueryRowContext(ctx, selectCommitment+` AND s.id = ?`, id)
	c, err := scanCommitment(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return RecurringCommitment{}, ErrNotFound
	}
	if err != nil {
		return RecurringCommitment{}, err
	}
	return c, nil
}

func listWhere(ctx context.Context, q Querier, where string) ([]RecurringCommitment, error) {
	query := selectCommitment
	if where != "" {
		query += " AND " + where
	}
	query += " ORDER BY s.name"
	rows, err := q.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var commitments []RecurringCommitment
	for rows.Next() {
		c, err := scanCommitment(rows.Scan)
		if err != nil {
			return nil, err
		}
		commitments = append(commitments, c)
	}
	return commitments, rows.Err()
}

// List returns every commitment, ordered by name.
func List(ctx context.Context, q Querier) ([]RecurringCommitment, error) {
	return listWhere(ctx, q, "")
}

// ListActive returns only commitments whose Scenario is active — the set
// internal/projections projects occurrences from.
func ListActive(ctx context.Context, q Querier) ([]RecurringCommitment, error) {
	return listWhere(ctx, q, "s.is_active = 1")
}

func formatDatePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatDate(*t)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func insertSchedule(ctx context.Context, q Querier, c RecurringCommitment) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO scenario_recurring_schedules (
			scenario_id, cashflow_kind, amount, category_id, account_id, cadence,
			day_of_month, month_of_year, start_date, end_date
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, string(c.Kind), money.CanonicalDecimal(c.Amount), c.CategoryID, c.AccountID,
		string(c.Cadence), c.DayOfMonth, c.MonthOfYear, formatDate(c.StartDate), formatDatePtr(c.EndDate),
	)
	return err
}
