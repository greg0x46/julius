package scenarios

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/money"
)

// ErrScenarioNotFound is returned by GetScenario/DeleteScenario when a
// scenario id has no matching row.
var ErrScenarioNotFound = errors.New("scenario not found")

// ErrTransactionNotFound is returned by GetScenarioTransaction/
// UpdateScenarioTransaction/DeleteScenarioTransaction when a scenario
// transaction id has no matching row.
var ErrTransactionNotFound = errors.New("scenario transaction not found")

// Querier is satisfied by both *sql.DB and *sql.Tx — mirrors payables.Querier.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// dateLayout is the on-disk format for the date-only projected_at column —
// deliberately narrower than db.TimeLayout (no time-of-day, no timezone)
// since a planned installment's date is a calendar date, not an instant.
const dateLayout = "2006-01-02"

func formatDate(t time.Time) string         { return t.UTC().Format(dateLayout) }
func parseDate(s string) (time.Time, error) { return dbtypes.ParseDate(s) }

const scenarioColumns = `id, kind, name, payable_id, is_active, is_accounting_source, created_at, updated_at`

// CreateScenario mirrors creating a Scenario row. Callers are responsible
// for enforcing kind-specific rules (payableID required, and its kind must
// agree with kind) before calling this — the schema also enforces
// payableID being non-null via a CHECK constraint as defense in depth.
func CreateScenario(ctx context.Context, q Querier, kind Kind, name string, payableID *string) (Scenario, error) {
	now := time.Now().UTC()
	if kind != KindDebtPlan && kind != KindReceivablePlan && kind != KindStandalone && kind != KindRecurring {
		return Scenario{}, errors.New("unknown scenario kind")
	}
	isActive := kind != KindStandalone
	isAccountingSource := false
	if kind == KindDebtPlan || kind == KindReceivablePlan {
		if payableID == nil {
			return Scenario{}, errors.New("payable id is required for a payable plan")
		}
		var payableKind string
		if err := q.QueryRowContext(ctx, `SELECT kind FROM payables WHERE id = ?`, *payableID).Scan(&payableKind); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return Scenario{}, errors.New("payable not found")
			}
			return Scenario{}, err
		}
		if (kind == KindDebtPlan && payableKind != "debt") ||
			(kind == KindReceivablePlan && payableKind != "receivable") {
			return Scenario{}, errors.New("scenario kind does not match payable kind")
		}
		var count int
		if err := q.QueryRowContext(ctx,
			`SELECT COUNT(1) FROM scenarios WHERE payable_id = ? AND is_accounting_source = TRUE`, payableID,
		).Scan(&count); err != nil {
			return Scenario{}, err
		}
		isAccountingSource = count == 0
	} else if kind == KindStandalone || kind == KindRecurring {
		if payableID != nil {
			return Scenario{}, errors.New("this scenario kind cannot reference a payable")
		}
	}
	s := Scenario{
		ID: uuid.NewString(), Kind: kind, Name: name, PayableID: payableID,
		IsActive: isActive, IsAccountingSource: isAccountingSource,
		CreatedAt: now, UpdatedAt: now,
	}
	_, err := q.ExecContext(ctx, `
		INSERT INTO scenarios (id, kind, name, payable_id, is_active, is_accounting_source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, string(s.Kind), s.Name, s.PayableID, s.IsActive, s.IsAccountingSource,
		db.FormatTime(now), db.FormatTime(now),
	)
	if err != nil {
		return Scenario{}, err
	}
	if s.IsAccountingSource {
		if err := backfillAccountingSettlements(ctx, q, s); err != nil {
			return Scenario{}, err
		}
	}
	return s, nil
}

// backfillAccountingSettlements settles the links a payable already had when
// its first plan appears. This is the product's ordering, not a migration
// leftover: a payable is created with no scenario, can be linked to real
// transactions immediately, and only gets an accounting source when someone
// creates a plan for it. Without this catch-up, money linked in that gap
// would never count toward the payable's balance. Allocations in other
// scenarios remain unrelated.
func backfillAccountingSettlements(ctx context.Context, q Querier, s Scenario) error {
	if s.PayableID == nil {
		return nil
	}
	rows, err := q.QueryContext(ctx, `
		SELECT transaction_id, linked_amount, linked_at
		FROM payable_transaction_links
		WHERE payable_id = ?`, *s.PayableID)
	if err != nil {
		return err
	}
	type linkSeed struct {
		transactionID string
		amount        string
		createdAt     string
	}
	var seeds []linkSeed
	for rows.Next() {
		var seed linkSeed
		if err := rows.Scan(dbtypes.ID(&seed.transactionID), dbtypes.Decimal(&seed.amount), dbtypes.Time(&seed.createdAt)); err != nil {
			rows.Close()
			return err
		}
		seeds = append(seeds, seed)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, seed := range seeds {
		var exists int
		if err := q.QueryRowContext(ctx, `
			SELECT COUNT(1)
			FROM scenario_realizations
			WHERE scenario_id = ? AND relation_type = 'settlement' AND transaction_id = ?`,
			s.ID, seed.transactionID).Scan(&exists); err != nil {
			return err
		}
		if exists != 0 {
			continue
		}
		if _, err := q.ExecContext(ctx, `
			INSERT INTO scenario_realizations (
				id, scenario_id, transaction_id, relation_type, state, origin,
				linked_amount, created_at
			) VALUES (?, ?, ?, 'settlement', 'linked', 'manual', ?, ?)`,
			uuid.NewString(), s.ID, seed.transactionID, seed.amount, seed.createdAt); err != nil {
			return err
		}
	}
	return nil
}

func scanScenario(row *sql.Row) (Scenario, error) {
	var (
		s                          Scenario
		kind                       string
		payableID                  sql.NullString
		isActive, isAccounting     int
		createdAtRaw, updatedAtRaw string
	)
	if err := row.Scan(dbtypes.ID(&s.ID), &kind, &s.Name, dbtypes.ID(&payableID), dbtypes.Bool(&isActive), dbtypes.Bool(&isAccounting), dbtypes.Time(&createdAtRaw), dbtypes.Time(&updatedAtRaw)); err != nil {
		return Scenario{}, err
	}
	s.Kind = Kind(kind)
	s.IsActive = isActive != 0
	s.IsAccountingSource = isAccounting != 0
	if payableID.Valid {
		s.PayableID = &payableID.String
	}
	var err error
	if s.CreatedAt, err = dbtypes.ParseTime(createdAtRaw); err != nil {
		return Scenario{}, err
	}
	if s.UpdatedAt, err = dbtypes.ParseTime(updatedAtRaw); err != nil {
		return Scenario{}, err
	}
	return s, nil
}

// GetScenario mirrors reading a single Scenario by id.
func GetScenario(ctx context.Context, q Querier, id string) (Scenario, error) {
	row := q.QueryRowContext(ctx, `SELECT `+scenarioColumns+` FROM scenarios WHERE id = ?`, id)
	s, err := scanScenario(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Scenario{}, ErrScenarioNotFound
	}
	return s, err
}

// ListScenariosByPayable mirrors listing every scenario attached to a
// payable, newest first.
func ListScenariosByPayable(ctx context.Context, q Querier, payableID string) ([]Scenario, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT `+scenarioColumns+` FROM scenarios WHERE payable_id = ? ORDER BY created_at DESC`, payableID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScenarioRows(rows)
}

// listPayablePlanScenarios mirrors listing every Scenario{Kind: debt_plan
// or receivable_plan} — i.e. every scenario backed by a payable, regardless
// of which one — fed to internal/timeline's M4 projection.
func listPayablePlanScenarios(ctx context.Context, q Querier) ([]Scenario, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT `+scenarioColumns+` FROM scenarios WHERE kind IN (?, ?) ORDER BY created_at DESC`,
		string(KindDebtPlan), string(KindReceivablePlan))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScenarioRows(rows)
}

// ListStandaloneScenarios mirrors listing every Scenario{Kind: standalone},
// newest first — the "what if" scenarios with no payable_id, fed to the
// M5 multi-select.
func ListStandaloneScenarios(ctx context.Context, q Querier) ([]Scenario, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT `+scenarioColumns+` FROM scenarios WHERE kind = ? ORDER BY created_at DESC`, string(KindStandalone))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScenarioRows(rows)
}

// ListScenariosByIDs mirrors fetching a batch of scenarios by id, in no
// particular order — used to load just the scenarios active in a
// simulation. An empty ids returns an empty slice without touching the
// database.
func ListScenariosByIDs(ctx context.Context, q Querier, ids []string) ([]Scenario, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	in, args := db.InClause(ids)
	query := `SELECT ` + scenarioColumns + ` FROM scenarios WHERE id IN (` + in + `)`
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScenarioRows(rows)
}

func scanScenarioRows(rows *sql.Rows) ([]Scenario, error) {
	var list []Scenario
	for rows.Next() {
		var (
			s                          Scenario
			kind                       string
			payableID                  sql.NullString
			isActive, isAccounting     int
			createdAtRaw, updatedAtRaw string
		)
		if err := rows.Scan(dbtypes.ID(&s.ID), &kind, &s.Name, dbtypes.ID(&payableID), dbtypes.Bool(&isActive), dbtypes.Bool(&isAccounting), dbtypes.Time(&createdAtRaw), dbtypes.Time(&updatedAtRaw)); err != nil {
			return nil, err
		}
		s.Kind = Kind(kind)
		s.IsActive = isActive != 0
		s.IsAccountingSource = isAccounting != 0
		if payableID.Valid {
			s.PayableID = &payableID.String
		}
		var err error
		if s.CreatedAt, err = dbtypes.ParseTime(createdAtRaw); err != nil {
			return nil, err
		}
		if s.UpdatedAt, err = dbtypes.ParseTime(updatedAtRaw); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

// SetActive changes only whether a scenario participates in the default
// projection selection. It deliberately leaves transactions and all
// realization history untouched.
func SetActive(ctx context.Context, q Querier, id string, active bool) (Scenario, error) {
	now := db.FormatTime(time.Now().UTC())
	result, err := q.ExecContext(ctx,
		`UPDATE scenarios SET is_active = ?, updated_at = ? WHERE id = ?`,
		active, now, id,
	)
	if err != nil {
		return Scenario{}, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return Scenario{}, ErrScenarioNotFound
	}
	return GetScenario(ctx, q, id)
}

// ListScenarios returns the unified catalog. Nil filters mean no filter; the
// explicit shape avoids overloading nil/empty IDs with active-selection
// semantics in callers such as the projection reader.
type ListFilter struct {
	Kind      *Kind
	IsActive  *bool
	PayableID *string
}

func ListScenarios(ctx context.Context, q Querier, filter ListFilter) ([]Scenario, error) {
	query := `SELECT ` + scenarioColumns + ` FROM scenarios`
	var clauses []string
	var args []any
	if filter.Kind != nil {
		clauses = append(clauses, "kind = ?")
		args = append(args, string(*filter.Kind))
	}
	if filter.IsActive != nil {
		clauses = append(clauses, "is_active = ?")
		args = append(args, *filter.IsActive)
	}
	if filter.PayableID != nil {
		clauses = append(clauses, "payable_id = ?")
		args = append(args, *filter.PayableID)
	}
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY created_at, id"
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScenarioRows(rows)
}

// ErrScenarioLinkedToAutomationRule is returned by DeleteScenario when an
// automation rule's reconcile action still targets a recurring Scenario (ON
// DELETE RESTRICT on automation_rule_actions.scenario_id).
var ErrScenarioLinkedToAutomationRule = errors.New("scenario is targeted by an automation rule's reconcile action")

// DeleteScenario mirrors deleting a Scenario. Its planned installments,
// recurring schedule and every realization recorded against it cascade via
// the schema's ON DELETE CASCADE; an automation rule targeting it does not,
// so that case surfaces as a typed conflict the caller can act on.
func DeleteScenario(ctx context.Context, q Querier, id string) error {
	res, err := q.ExecContext(ctx, `DELETE FROM scenarios WHERE id = ?`, id)
	if err != nil {
		if db.IsForeignKeyViolation(err) {
			return ErrScenarioLinkedToAutomationRule
		}
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrScenarioNotFound
	}
	return nil
}

// CreateScenarioTransaction mirrors inserting a single planned installment.
func CreateScenarioTransaction(ctx context.Context, q Querier, scenarioID, description string, amount decimal.Decimal, projectedAt time.Time, category *string) (ScenarioTransaction, error) {
	now := time.Now().UTC()
	st := ScenarioTransaction{
		ID: uuid.NewString(), ScenarioID: scenarioID, Description: description,
		Amount: amount, ProjectedAt: projectedAt, Category: category,
		CreatedAt: now, UpdatedAt: now,
	}
	_, err := q.ExecContext(ctx, `
		INSERT INTO scenario_transactions (id, scenario_id, description, amount, projected_at, category, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		st.ID, st.ScenarioID, st.Description, money.CanonicalDecimal(st.Amount), formatDate(st.ProjectedAt), st.Category,
		db.FormatTime(now), db.FormatTime(now),
	)
	if err != nil {
		return ScenarioTransaction{}, err
	}
	return st, nil
}

func scanScenarioTransaction(row *sql.Row) (ScenarioTransaction, error) {
	var (
		st                         ScenarioTransaction
		amountRaw, projectedAtRaw  string
		category                   sql.NullString
		createdAtRaw, updatedAtRaw string
	)
	if err := row.Scan(dbtypes.ID(&st.ID), dbtypes.ID(&st.ScenarioID), &st.Description, dbtypes.Decimal(&amountRaw), dbtypes.Date(&projectedAtRaw), dbtypes.ID(&category), dbtypes.Time(&createdAtRaw), dbtypes.Time(&updatedAtRaw)); err != nil {
		return ScenarioTransaction{}, err
	}
	if category.Valid {
		st.Category = &category.String
	}
	var err error
	if st.Amount, err = decimal.NewFromString(amountRaw); err != nil {
		return ScenarioTransaction{}, err
	}
	if st.ProjectedAt, err = parseDate(projectedAtRaw); err != nil {
		return ScenarioTransaction{}, err
	}
	if st.CreatedAt, err = dbtypes.ParseTime(createdAtRaw); err != nil {
		return ScenarioTransaction{}, err
	}
	if st.UpdatedAt, err = dbtypes.ParseTime(updatedAtRaw); err != nil {
		return ScenarioTransaction{}, err
	}
	return st, nil
}

const scenarioTransactionColumns = `id, scenario_id, description, amount, projected_at, category, created_at, updated_at`

// GetScenarioTransaction mirrors reading a single planned installment by id.
func GetScenarioTransaction(ctx context.Context, q Querier, id string) (ScenarioTransaction, error) {
	row := q.QueryRowContext(ctx, `SELECT `+scenarioTransactionColumns+` FROM scenario_transactions WHERE id = ?`, id)
	st, err := scanScenarioTransaction(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ScenarioTransaction{}, ErrTransactionNotFound
	}
	return st, err
}

// ListScenarioTransactions mirrors listing every planned installment of a
// scenario, ordered by projected_at (earliest first — the natural reading
// order for a payment plan).
func ListScenarioTransactions(ctx context.Context, q Querier, scenarioID string) ([]ScenarioTransaction, error) {
	byScenario, err := ListScenarioTransactionsFor(ctx, q, []string{scenarioID})
	if err != nil {
		return nil, err
	}
	return byScenario[scenarioID], nil
}

// ListScenarioTransactionsFor loads the installments of any number of
// scenarios in one query, keyed by scenario_id and ordered by projected_at
// within each key — the same batching shape realizationsFor uses, for the
// same reason: internal/projections walks every plan of a selection at once,
// and a query per plan would make the timeline's cost grow with how many
// plans the user has open. A scenario with no installments is simply absent
// from the map. Callers holding a single id should use
// ListScenarioTransactions.
func ListScenarioTransactionsFor(ctx context.Context, q Querier, scenarioIDs []string) (map[string][]ScenarioTransaction, error) {
	result := make(map[string][]ScenarioTransaction, len(scenarioIDs))
	if len(scenarioIDs) == 0 {
		return result, nil
	}
	in, args := db.InClause(scenarioIDs)
	rows, err := q.QueryContext(ctx,
		`SELECT `+scenarioTransactionColumns+` FROM scenario_transactions
		 WHERE scenario_id IN (`+in+`)
		 ORDER BY projected_at, created_at`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			st                         ScenarioTransaction
			amountRaw, projectedAtRaw  string
			category                   sql.NullString
			createdAtRaw, updatedAtRaw string
		)
		if err := rows.Scan(dbtypes.ID(&st.ID), dbtypes.ID(&st.ScenarioID), &st.Description, dbtypes.Decimal(&amountRaw), dbtypes.Date(&projectedAtRaw), dbtypes.ID(&category), dbtypes.Time(&createdAtRaw), dbtypes.Time(&updatedAtRaw)); err != nil {
			return nil, err
		}
		if category.Valid {
			st.Category = &category.String
		}
		if st.Amount, err = decimal.NewFromString(amountRaw); err != nil {
			return nil, err
		}
		if st.ProjectedAt, err = parseDate(projectedAtRaw); err != nil {
			return nil, err
		}
		if st.CreatedAt, err = dbtypes.ParseTime(createdAtRaw); err != nil {
			return nil, err
		}
		if st.UpdatedAt, err = dbtypes.ParseTime(updatedAtRaw); err != nil {
			return nil, err
		}
		result[st.ScenarioID] = append(result[st.ScenarioID], st)
	}
	return result, rows.Err()
}

// UpdateScenarioTransaction mirrors editing a planned installment's fields
// directly (used both by manual edits and, later, by the readjustment flow).
func UpdateScenarioTransaction(ctx context.Context, q Querier, id, description string, amount decimal.Decimal, projectedAt time.Time, category *string) (ScenarioTransaction, error) {
	now := db.FormatTime(time.Now())
	res, err := q.ExecContext(ctx, `
		UPDATE scenario_transactions SET description = ?, amount = ?, projected_at = ?, category = ?, updated_at = ?
		WHERE id = ?`,
		description, money.CanonicalDecimal(amount), formatDate(projectedAt), category, now, id)
	if err != nil {
		return ScenarioTransaction{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ScenarioTransaction{}, ErrTransactionNotFound
	}
	return GetScenarioTransaction(ctx, q, id)
}

// DeleteScenarioTransaction mirrors removing a single planned installment
// (its scenario_realizations allocations cascade via ON DELETE CASCADE).
func DeleteScenarioTransaction(ctx context.Context, q Querier, id string) error {
	res, err := q.ExecContext(ctx, `DELETE FROM scenario_transactions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrTransactionNotFound
	}
	return nil
}
