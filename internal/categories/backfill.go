package categories

import (
	"context"
	"database/sql"
	"errors"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
)

// BackfillAutomatic applies the automatic category mapping to transactions
// that were already stored before sourceCategory had an entry in
// SourceCategoryMapping.
//
// A new mapping entry alone never reaches them: the sync service only calls
// ApplyAutomatic for a freshly inserted transaction (an update deliberately
// does not re-categorize, so it cannot undo a later manual choice), and
// ApplyAutomatic itself is insert-only. Every row synced before the mapping
// grew stays uncategorized forever without this pass.
//
// It reuses ApplyAutomatic per transaction rather than writing decisions
// directly, so the "never override an existing decision" guard — manual
// choices above all — lives in exactly one place. Re-running is therefore a
// no-op, which is what lets the caller run it unconditionally on startup.
// Returns how many transactions were newly categorized.
func BackfillAutomatic(ctx context.Context, conn *sql.DB, sourceCategory string) (int, error) {
	categoryID, ok := SourceCategoryMapping[sourceCategory]
	if !ok {
		return 0, nil
	}
	// Checked once here rather than trusted per row: it is the only other
	// reason ApplyAutomatic silently no-ops, so ruling it out up front keeps
	// the returned count exact.
	if _, err := Get(ctx, conn, categoryID); errors.Is(err, ErrNotFound) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}

	rows, err := conn.QueryContext(ctx,
		`SELECT ft.id
		 FROM financial_transactions ft
		 LEFT JOIN transaction_category_decisions d ON d.transaction_id = ft.id
		 WHERE ft.source_category = ? AND d.transaction_id IS NULL`,
		sourceCategory,
	)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(dbtypes.ID(&id)); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	// SQLite runs on a single connection, so the rows above must be drained
	// and closed before ApplyAutomatic can write.
	if err := rows.Close(); err != nil {
		return 0, err
	}

	applied := 0
	for _, id := range ids {
		if err := ApplyAutomatic(ctx, conn, id, &sourceCategory); err != nil {
			return applied, err
		}
		applied++
	}
	return applied, nil
}
