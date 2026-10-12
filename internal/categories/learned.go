package categories

import (
	"context"
	"database/sql"
	"errors"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"regexp"
	"strings"
)

// installmentSuffix matches the " N/M" parcela suffix Pluggy appends to a
// split card purchase's description (see findInstallmentGroup, which assumes
// the same " %d/%d" shape).
var installmentSuffix = regexp.MustCompile(`\s+\d{1,2}/\d{1,2}$`)

// similarityKey reduces a description to the form two "same" transactions
// share: lowercase, trimmed, inner whitespace collapsed, installment suffix
// dropped — so "Jim.Com* 50450362 Kau 3/12" and "Jim.Com* 50450362 Kau 7/12"
// key identically, as do "Padaria Rv" and "PADARIA RV ".
func similarityKey(description string) string {
	s := installmentSuffix.ReplaceAllString(strings.TrimSpace(description), "")
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// learnedCategoryFor finds the category the user most recently assigned by
// hand to a transaction that looks like transactionID: same similarityKey
// and same movement_type (a "Transferência enviada|X" and a "Transferência
// Recebida|X" already differ in text, but the direction check also keeps a
// same-named debit and credit apart). The transaction itself, soft-deleted
// rows and inactive categories are skipped. ok is false when nothing
// qualifies.
//
// The installment suffix strip isn't expressible portably across SQLite and
// Postgres, so the manual decisions (a small set — the user only ever
// categorizes by hand what the automatic sources got wrong) are pulled and
// compared in Go, newest first, so the first hit is the winner.
func learnedCategoryFor(ctx context.Context, q Querier, transactionID string) (categoryID string, ok bool, err error) {
	var description, movementType sql.NullString
	err = q.QueryRowContext(ctx,
		`SELECT description, movement_type FROM financial_transactions WHERE id = ?`, transactionID,
	).Scan(&description, &movementType)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, ErrTransactionNotFound
	}
	if err != nil {
		return "", false, err
	}
	if !description.Valid {
		return "", false, nil
	}
	wantKey := similarityKey(description.String)
	if wantKey == "" {
		return "", false, nil
	}

	rows, err := q.QueryContext(ctx, `
		SELECT ft.description, ft.movement_type, d.category_id
		FROM transaction_category_decisions d
		JOIN financial_transactions ft ON ft.id = d.transaction_id
		JOIN categories c ON c.id = d.category_id
		WHERE d.origin = ? AND ft.id <> ? AND ft.deleted_at IS NULL AND c.is_active = 1
		ORDER BY d.changed_at DESC, d.transaction_id`,
		string(OriginManual), transactionID,
	)
	if err != nil {
		return "", false, err
	}
	defer rows.Close()
	for rows.Next() {
		var refDescription, refMovementType sql.NullString
		var refCategoryID string
		if err := rows.Scan(&refDescription, &refMovementType, dbtypes.ID(&refCategoryID)); err != nil {
			return "", false, err
		}
		if refMovementType != movementType || !refDescription.Valid {
			continue
		}
		if similarityKey(refDescription.String) == wantKey {
			return refCategoryID, true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", false, err
	}
	return "", false, nil
}

// ApplyLearned assigns transactionID the category of a similar transaction
// the user categorized by hand (see learnedCategoryFor), writing an
// OriginLearned decision. Precedence is manual > rule > learned >
// automatic: it never touches a manual or rule decision, overrides an
// automatic one, and — like ApplyAutomatic — silently no-ops when there is
// no reference, so a sync run never fails on it. Like ApplyRule it
// propagates to every installment of a split card purchase.
func ApplyLearned(ctx context.Context, q Querier, transactionID string) (changed bool, err error) {
	existing, err := getDecision(ctx, q, transactionID)
	if err != nil {
		return false, err
	}
	if existing != nil && existing.Origin != OriginAutomatic {
		return false, nil
	}
	categoryID, ok, err := learnedCategoryFor(ctx, q, transactionID)
	if errors.Is(err, ErrTransactionNotFound) {
		return false, nil
	}
	if err != nil || !ok {
		return false, err
	}

	group, err := findInstallmentGroup(ctx, q, transactionID)
	if err != nil {
		return false, err
	}
	for _, id := range group {
		// Siblings may have arrived and been categorized before this row.
		// Apply the same precedence and idempotency guard to each of them.
		existing, err := getDecision(ctx, q, id)
		if err != nil {
			return false, err
		}
		if existing != nil && existing.Origin != OriginAutomatic {
			continue
		}
		_, didChange, err := writeDecision(ctx, q, id, categoryID, OriginLearned)
		if err != nil {
			return false, err
		}
		if id == transactionID {
			changed = didChange
		}
	}
	return changed, nil
}
