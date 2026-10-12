package scenarios

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

// ErrRealizationNotFound is returned by DeleteRealization when a
// realization id has no matching row.
var ErrRealizationNotFound = errors.New("scenario transaction realization not found")

// allocationSelect projects an allocation row of scenario_realizations back
// into the installment-allocation shape the plan endpoints speak.
//
// PayableLinkID is recovered by matching the realized transaction against the
// scenario's payable rather than stored: scenario_realizations references the
// real financial transaction, and payable_transaction_links.transaction_id is
// UNIQUE, so the join yields at most one link and never duplicates a row. A
// standalone scenario has no payable and therefore no link id, which is why
// the column stays nullable in the DTO.
const allocationSelect = `
	SELECT sr.id, sr.scenario_transaction_id, l.id, sr.allocated_amount, sr.created_at
	FROM scenario_realizations sr
	JOIN scenarios s ON s.id = sr.scenario_id
	LEFT JOIN payable_transaction_links l
		ON l.payable_id = s.payable_id AND l.transaction_id = sr.transaction_id
	WHERE sr.relation_type = 'allocation'`

func scanAllocation(scan func(dest ...any) error) (ScenarioTransactionRealization, error) {
	var (
		r                                ScenarioTransactionRealization
		payableLinkID                    sql.NullString
		allocatedAmountRaw, createdAtRaw string
	)
	if err := scan(dbtypes.ID(&r.ID), dbtypes.ID(&r.ScenarioTransactionID), dbtypes.ID(&payableLinkID), dbtypes.Decimal(&allocatedAmountRaw), dbtypes.Time(&createdAtRaw)); err != nil {
		return ScenarioTransactionRealization{}, err
	}
	if payableLinkID.Valid {
		r.PayableLinkID = &payableLinkID.String
	}
	var err error
	if r.AllocatedAmount, err = decimal.NewFromString(allocatedAmountRaw); err != nil {
		return ScenarioTransactionRealization{}, err
	}
	if r.CreatedAt, err = dbtypes.ParseTime(createdAtRaw); err != nil {
		return ScenarioTransactionRealization{}, err
	}
	return r, nil
}

// CreateRealization allocates (part of) a payable_transaction_links row's
// amount to scenarioTransactionID. Callers are responsible for validating
// that both ids exist and belong to the same payable before calling this.
//
// The row lands in scenario_realizations as relation_type = 'allocation':
// the same table the Timeline projects from, so an allocation created here
// and one created through the generic event endpoint are the same fact.
func CreateRealization(ctx context.Context, q Querier, scenarioTransactionID string, payableLinkID *string, allocatedAmount decimal.Decimal) (ScenarioTransactionRealization, error) {
	now := time.Now().UTC()
	r := ScenarioTransactionRealization{
		ID: uuid.NewString(), ScenarioTransactionID: scenarioTransactionID, PayableLinkID: payableLinkID,
		AllocatedAmount: allocatedAmount, CreatedAt: now,
	}
	var scenarioID, transactionID string
	if err := q.QueryRowContext(ctx, `
		SELECT st.scenario_id, l.transaction_id
		FROM scenario_transactions st
		JOIN payable_transaction_links l ON l.id = ?
		WHERE st.id = ?`, r.PayableLinkID, r.ScenarioTransactionID).Scan(dbtypes.ID(&scenarioID), dbtypes.ID(&transactionID)); err != nil {
		return ScenarioTransactionRealization{}, err
	}
	if _, err := q.ExecContext(ctx, `
		INSERT INTO scenario_realizations (
			id, scenario_id, scenario_transaction_id, transaction_id, relation_type,
			state, origin, allocated_amount, created_at
		) VALUES (?, ?, ?, ?, 'allocation', 'linked', 'manual', ?, ?)`,
		r.ID, scenarioID, r.ScenarioTransactionID, transactionID,
		money.CanonicalDecimal(r.AllocatedAmount), db.FormatTime(r.CreatedAt),
	); err != nil {
		return ScenarioTransactionRealization{}, err
	}
	return r, nil
}

// GetRealization reads a single allocation by id — callers that need to
// scope it to a specific scenario_transaction (the HTTP layer's delete
// endpoint) check ScenarioTransactionID themselves, the same pattern
// payables.GetLink/DeleteLink uses.
func GetRealization(ctx context.Context, q Querier, id string) (ScenarioTransactionRealization, error) {
	row := q.QueryRowContext(ctx, allocationSelect+` AND sr.id = ?`, id)
	r, err := scanAllocation(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return ScenarioTransactionRealization{}, ErrRealizationNotFound
	}
	if err != nil {
		return ScenarioTransactionRealization{}, err
	}
	return r, nil
}

// DeleteRealization removes a single allocation, undoing it without touching
// the scenario_transaction or the payable link it referenced.
func DeleteRealization(ctx context.Context, q Querier, id string) error {
	res, err := q.ExecContext(ctx,
		`DELETE FROM scenario_realizations WHERE id = ? AND relation_type = 'allocation'`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrRealizationNotFound
	}
	return nil
}

// sumAllocated mirrors the spec's realizedTotal: the sum of
// allocated_amount across every realization allocated to one installment,
// computed on read — never stored on the scenario_transaction row itself.
// Summed in Go with decimal.Decimal (matching debts.PaidAmount's approach)
// rather than SQL SUM, which would have to go through a float-lossy
// CAST(... AS REAL) on this driver.
//
// It takes the rows rather than an id because every caller has already
// loaded them in bulk via realizationsFor — a per-installment query for
// this number is exactly the round trip that batching removed.
func sumAllocated(list []ScenarioTransactionRealization) decimal.Decimal {
	total := decimal.Zero
	for _, r := range list {
		total = total.Add(r.AllocatedAmount)
	}
	return total
}

// listRealizationsForTransaction mirrors listing every allocation a planned
// installment has received, newest first.
func listRealizationsForTransaction(ctx context.Context, q Querier, scenarioTransactionID string) ([]ScenarioTransactionRealization, error) {
	byTransaction, err := realizationsFor(ctx, q, []string{scenarioTransactionID})
	if err != nil {
		return nil, err
	}
	return byTransaction[scenarioTransactionID], nil
}

// realizationsFor loads every allocation belonging to any of
// scenarioTransactionIDs in one query, keyed by scenario_transaction_id and
// newest-first within each key.
//
// It exists because every interesting read here is per-scenario, not
// per-installment: summarizing a scenario, listing a plan's unrealized
// installments, deciding which installments a readjustment may replace. One
// query for the whole set keeps those O(1) in round trips instead of O(number
// of installments). An installment with no allocations is simply absent from
// the map.
func realizationsFor(ctx context.Context, q Querier, scenarioTransactionIDs []string) (map[string][]ScenarioTransactionRealization, error) {
	result := make(map[string][]ScenarioTransactionRealization, len(scenarioTransactionIDs))
	if len(scenarioTransactionIDs) == 0 {
		return result, nil
	}
	in, args := db.InClause(scenarioTransactionIDs)
	rows, err := q.QueryContext(ctx,
		allocationSelect+` AND sr.scenario_transaction_id IN (`+in+`)
		ORDER BY sr.created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		r, err := scanAllocation(rows.Scan)
		if err != nil {
			return nil, err
		}
		result[r.ScenarioTransactionID] = append(result[r.ScenarioTransactionID], r)
	}
	return result, rows.Err()
}
