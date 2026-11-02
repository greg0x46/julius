package transactions

import (
	"context"
	"fmt"

	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/settings"
)

// History returns undeleted ledger items without pagination or FX conversion.
// Classification, inclusion and transfer eligibility use the ordinary ledger rules.
func History(ctx context.Context, q Querier) ([]Item, error) {
	views, err := fetchAllViews(ctx, q, QueryRequest{Timezone: "UTC", GroupBy: money.GroupNone})
	if err != nil {
		return nil, err
	}
	items := make([]Item, len(views))
	for i, v := range views {
		items[i] = toItem(v)
	}
	return items, nil
}

// GetItem loads one transaction as the very same Item the list paths produce
// — every money rule already applied (classification, effective money,
// totals eligibility), not raw columns.
//
// It exists because callers that need to *decide* something about a single
// transaction (is it eligible to reconcile a recurring commitment?) were
// otherwise forced to re-derive those rules from raw columns, which is how
// two code paths start disagreeing about what a transaction is. The date
// filtering/grouping a QueryRequest carries is meaningless for one row by
// id, so this skips it: buildView still runs so the period-basis preference
// applies identically, but nothing filters the result away.
//
// found is false — rather than an error — when id matches no row, mirroring
// how the store packages report a missing link.
func GetItem(ctx context.Context, q Querier, id string) (item Item, found bool, err error) {
	transfer, err := investments.ReconciledTransactionAmount(ctx, q, id)
	if err != nil {
		return Item{}, false, err
	}
	periodBasis, err := settings.GetTransactionsPeriodBasis(ctx, q)
	if err != nil {
		return Item{}, false, fmt.Errorf("read transactions period basis preference: %w", err)
	}
	billDueDates, err := fetchBillDueDates(ctx, q)
	if err != nil {
		return Item{}, false, err
	}

	rows, err := q.QueryContext(ctx, viewSelect+` AND ft.id = ?`, id)
	if err != nil {
		return Item{}, false, fmt.Errorf("query transaction %s: %w", id, err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Item{}, false, err
		}
		return Item{}, false, nil
	}
	r, err := scanRow(rows)
	if err != nil {
		return Item{}, false, err
	}
	if err := rows.Err(); err != nil {
		return Item{}, false, err
	}

	// GroupNone/UTC: buildView only consults the request for grouping, which
	// a single row by id has no use for.
	v, err := buildView(r, QueryRequest{Timezone: "UTC", GroupBy: money.GroupNone}, periodBasis, billDueDates)
	if err != nil {
		return Item{}, false, err
	}
	v.applyInvestmentTransfer(transfer)
	return toItem(v), true, nil
}
