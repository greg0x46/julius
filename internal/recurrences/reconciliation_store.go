package recurrences

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/transactions"
)

// ErrOverrideNotFound is returned by DeleteOverride when the occurrence
// carries no manual decision to undo.
var ErrOverrideNotFound = errors.New("recurrence reconciliation not found")

// ErrTransactionAlreadyReconciled is returned by PutOverride when the chosen
// transaction already satisfies another complete event
// (uq_scenario_realizations_one_complete_event_transaction). One transaction
// settles at most one occurrence — otherwise the same money would suppress
// two projections.
var ErrTransactionAlreadyReconciled = errors.New("transaction is already reconciled to another occurrence")

// Overrides live in scenario_realizations as relation_type = 'reconciliation'
// rows: the same table the Timeline projects from and the generic event
// endpoint writes to, so a decision made through either API is the same fact.
// The columns below are the reconciliation-shaped projection of that table.
const overrideColumns = `id, scenario_id, occurrence_date, state, transaction_id, origin, created_at`

const overrideFrom = ` FROM scenario_realizations WHERE relation_type = 'reconciliation'`

func scanOverride(scan func(dest ...any) error) (Override, error) {
	var (
		o                 Override
		occurrenceDateRaw string
		stateRaw          string
		transactionID     sql.NullString
		originRaw         string
		createdAtRaw      string
	)
	if err := scan(dbtypes.ID(&o.ID), dbtypes.ID(&o.ScenarioID), dbtypes.Date(&occurrenceDateRaw), &stateRaw, dbtypes.ID(&transactionID), &originRaw, dbtypes.Time(&createdAtRaw)); err != nil {
		return Override{}, err
	}
	o.State = OverrideState(stateRaw)
	o.Origin = Origin(originRaw)
	if transactionID.Valid {
		o.TransactionID = &transactionID.String
	}
	var err error
	if o.OccurrenceDate, err = parseDate(occurrenceDateRaw); err != nil {
		return Override{}, err
	}
	if o.CreatedAt, err = dbtypes.ParseTime(createdAtRaw); err != nil {
		return Override{}, err
	}
	return o, nil
}

// ListOverrides loads every manual decision belonging to any of scenarioIDs
// whose occurrence falls in [from, to], keyed by scenario id.
//
// It takes the whole set rather than one id because every interesting read is
// per-period, not per-scenario: the Timeline resolves every active recurring
// scenario in one pass. One query for the whole set keeps that O(1) in
// round trips instead of O(number of scenarios) — the same reasoning behind
// scenarios.realizationsFor.
func ListOverrides(ctx context.Context, q Querier, scenarioIDs []string, from, to time.Time) (map[string][]Override, error) {
	result := make(map[string][]Override, len(scenarioIDs))
	if len(scenarioIDs) == 0 {
		return result, nil
	}
	in, args := db.InClause(scenarioIDs)
	args = append(args, formatDate(dates.Day(from)), formatDate(dates.Day(to)))
	rows, err := q.QueryContext(ctx, `
		SELECT `+overrideColumns+overrideFrom+`
			AND scenario_id IN (`+in+`)
			AND occurrence_date >= ? AND occurrence_date <= ?
		ORDER BY occurrence_date`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		o, err := scanOverride(rows.Scan)
		if err != nil {
			return nil, err
		}
		result[o.ScenarioID] = append(result[o.ScenarioID], o)
	}
	return result, rows.Err()
}

// GetOverride reads the manual decision on one occurrence, if any.
func GetOverride(ctx context.Context, q Querier, scenarioID string, occurrenceDate time.Time) (Override, bool, error) {
	row := q.QueryRowContext(ctx, `
		SELECT `+overrideColumns+overrideFrom+`
			AND scenario_id = ? AND occurrence_date = ?`,
		scenarioID, formatDate(dates.Day(occurrenceDate)))
	o, err := scanOverride(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Override{}, false, nil
	}
	if err != nil {
		return Override{}, false, err
	}
	return o, true, nil
}

// OverrideForTransaction finds the occurrence a transaction is hand-linked
// to, if any — the reverse lookup the transaction detail view needs.
func OverrideForTransaction(ctx context.Context, q Querier, transactionID string) (Override, bool, error) {
	row := q.QueryRowContext(ctx, `
		SELECT `+overrideColumns+overrideFrom+` AND transaction_id = ?`, transactionID)
	o, err := scanOverride(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Override{}, false, nil
	}
	if err != nil {
		return Override{}, false, err
	}
	return o, true, nil
}

// LinkedTransactionIDs returns every transaction currently hand-linked to
// some occurrence, so a candidate list can exclude them in one lookup rather
// than one query per candidate.
func LinkedTransactionIDs(ctx context.Context, q Querier) (map[string]bool, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT transaction_id`+overrideFrom+` AND transaction_id IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	linked := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(dbtypes.ID(&id)); err != nil {
			return nil, err
		}
		linked[id] = true
	}
	return linked, rows.Err()
}

// PutOverride records the user's decision about one occurrence, replacing
// whatever decision was there before: (scenario, occurrence_date) is UNIQUE
// among reconciliation rows, and the three UI actions are all one write to
// that one row — linking a transaction, detaching the occurrence, or
// replacing one manual link with another. Delete + insert rather than an
// upsert because the two dialects spell ON CONFLICT differently and the pair
// runs in one transaction anyway.
//
// Callers are responsible for having validated that occurrenceDate really is
// an occurrence of this scenario, and that transactionID is eligible — the
// HTTP layer does both before reaching here.
func PutOverride(ctx context.Context, conn *sql.DB, scenarioID string, occurrenceDate time.Time, state OverrideState, transactionID *string) (Override, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return Override{}, err
	}
	defer tx.Rollback()

	day := dates.Day(occurrenceDate)
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM scenario_realizations
		WHERE scenario_id = ? AND relation_type = 'reconciliation' AND occurrence_date = ?`,
		scenarioID, formatDate(day)); err != nil {
		return Override{}, err
	}

	o := Override{
		ID:             uuid.NewString(),
		ScenarioID:     scenarioID,
		OccurrenceDate: day,
		State:          state,
		TransactionID:  transactionID,
		Origin:         OriginManual,
		CreatedAt:      time.Now().UTC(),
	}
	if state == StateDetached {
		o.TransactionID = nil
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO scenario_realizations (
			id, scenario_id, occurrence_date, transaction_id, relation_type,
			state, origin, created_at
		) VALUES (?, ?, ?, ?, 'reconciliation', ?, ?, ?)`,
		o.ID, o.ScenarioID, formatDate(o.OccurrenceDate), o.TransactionID,
		string(o.State), string(o.Origin), db.FormatTime(o.CreatedAt),
	); err != nil {
		if db.IsUniqueViolationOn(err,
			db.ConstraintOneCompleteEventTransactionSQLite,
			db.ConstraintOneCompleteEventTransactionPostgres) {
			// Another complete event already claims this transaction. The
			// HTTP layer checks for that first, so reaching here means a
			// concurrent writer won the race. Anything else — an unknown
			// scenario, or the (scenario, occurrence_date) index losing its
			// own race — is a real failure and must not be reported as a
			// conflict the user could resolve by picking another transaction.
			return Override{}, ErrTransactionAlreadyReconciled
		}
		return Override{}, err
	}
	if err := tx.Commit(); err != nil {
		return Override{}, err
	}
	return o, nil
}

// DeleteOverride drops the manual decision on one occurrence, handing it back
// to the automation rule ("voltar ao automático"). This is not the same as
// detaching: detaching is a decision that persists, this forgets that a
// decision was ever made.
func DeleteOverride(ctx context.Context, q Querier, scenarioID string, occurrenceDate time.Time) error {
	res, err := q.ExecContext(ctx, `
		DELETE FROM scenario_realizations
		WHERE scenario_id = ? AND relation_type = 'reconciliation' AND occurrence_date = ?`,
		scenarioID, formatDate(dates.Day(occurrenceDate)))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrOverrideNotFound
	}
	return nil
}

// UnlinkIfPresent drops any manual link a transaction holds. Its signature
// matches transactions.OnIgnoredHook exactly — same as
// payables.UnlinkIfPresent — so it can be wired directly into every
// inclusion-changing path: a transaction the user marked as ignored is
// excluded from the totals, so it can't be the thing carrying a commitment's
// money.
//
// Only 'linked' rows have a transaction_id, so a detached occurrence is
// untouched by this — the user's decision to detach survives an unrelated
// transaction being ignored.
func UnlinkIfPresent(ctx context.Context, q transactions.Querier, transactionID string) error {
	_, err := q.ExecContext(ctx, `
		DELETE FROM scenario_realizations
		WHERE relation_type = 'reconciliation' AND transaction_id = ?`, transactionID)
	return err
}
