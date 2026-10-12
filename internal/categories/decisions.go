package categories

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
)

// Origin distinguishes a category decision the user made explicitly from one
// the automatic source_category mapping applied.
type Origin string

const (
	OriginManual    Origin = "manual"
	OriginAutomatic Origin = "automatic"
	// OriginRule is set by a matching automation rule's set_category
	// action — see ApplyRule.
	OriginRule Origin = "rule"
	// OriginLearned is set by ApplyLearned (learned.go) when a newly
	// ingested transaction looks like one the user categorized by hand
	// before: same normalized description and movement type. It sits
	// between rule and automatic in precedence — a rule may overwrite it,
	// it may overwrite an automatic decision, and it never touches a manual
	// or rule one.
	OriginLearned Origin = "learned"
)

// Decision is a transaction's current category assignment.
type Decision struct {
	TransactionID string
	CategoryID    string
	Revision      int
	ChangedAt     time.Time
	Origin        Origin
}

// ErrTransactionNotFound and ErrCategoryInvalid are the two failure modes of
// AssignManual, mirroring assign_transaction_category's
// AssignCategoryError literal.
var (
	ErrTransactionNotFound = errors.New("transaction not found")
	ErrCategoryInvalid     = errors.New("category not found or inactive")
)

func getDecision(ctx context.Context, q Querier, transactionID string) (*Decision, error) {
	var (
		d            Decision
		changedAtRaw string
	)
	err := q.QueryRowContext(ctx,
		`SELECT transaction_id, category_id, revision, changed_at, origin
		 FROM transaction_category_decisions WHERE transaction_id = ?`,
		transactionID,
	).Scan(dbtypes.ID(&d.TransactionID), dbtypes.ID(&d.CategoryID), &d.Revision, dbtypes.Time(&changedAtRaw), &d.Origin)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if d.ChangedAt, err = dbtypes.ParseTime(changedAtRaw); err != nil {
		return nil, err
	}
	return &d, nil
}

// writeDecision upserts the decision row and appends the corresponding
// append-only event, mirroring categorization.py's _write_category_decision.
// It is a no-op (changed=false) when both category and origin are unchanged.
// An origin promotion is a decision change even when the category stays the same.
func writeDecision(
	ctx context.Context, q Querier, transactionID, categoryID string, origin Origin,
) (decision Decision, changed bool, err error) {
	existing, err := getDecision(ctx, q, transactionID)
	if err != nil {
		return Decision{}, false, err
	}
	if existing != nil && existing.CategoryID == categoryID && existing.Origin == origin {
		return *existing, false, nil
	}

	changedAt := time.Now().UTC()
	revision := 1
	var previousCategoryID *string
	if existing != nil {
		revision = existing.Revision + 1
		previousCategoryID = &existing.CategoryID
	}

	_, err = q.ExecContext(ctx,
		`INSERT INTO transaction_category_decisions
			(transaction_id, category_id, revision, changed_at, origin)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (transaction_id) DO UPDATE SET
			category_id = excluded.category_id,
			revision = excluded.revision,
			changed_at = excluded.changed_at,
			origin = excluded.origin`,
		transactionID, categoryID, revision, db.FormatTime(changedAt), string(origin),
	)
	if err != nil {
		return Decision{}, false, err
	}
	_, err = q.ExecContext(ctx,
		`INSERT INTO transaction_category_events
			(id, transaction_id, revision, previous_category_id, resulting_category_id, changed_at, origin)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), transactionID, revision, previousCategoryID, categoryID, db.FormatTime(changedAt), string(origin),
	)
	if err != nil {
		return Decision{}, false, err
	}
	return Decision{
		TransactionID: transactionID,
		CategoryID:    categoryID,
		Revision:      revision,
		ChangedAt:     changedAt,
		Origin:        origin,
	}, true, nil
}

// AssignManual mirrors assign_transaction_category: always overrides any
// prior decision (manual or automatic). Returns ErrTransactionNotFound or
// ErrCategoryInvalid (category missing or inactive) instead of writing.
//
// When transactionID is one installment of a card purchase split across
// several financial_transactions rows (see findInstallmentGroup), the same
// decision is written for every installment in that purchase, all inside a
// single transaction — a card purchase has one category, not one per
// parcela.
func AssignManual(ctx context.Context, conn *sql.DB, transactionID, categoryID string) (Decision, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return Decision{}, err
	}
	defer tx.Rollback()

	decision, err := AssignManualWithQuerier(ctx, tx, transactionID, categoryID)
	if err != nil {
		return Decision{}, err
	}
	if err := tx.Commit(); err != nil {
		return Decision{}, err
	}
	return decision, nil
}

// AssignManualWithQuerier is AssignManual without owning the transaction
// boundary. It lets compound operations keep the transaction row, category
// decision, and category event in one caller-controlled transaction.
func AssignManualWithQuerier(ctx context.Context, q Querier, transactionID, categoryID string) (Decision, error) {
	var exists int
	err := q.QueryRowContext(ctx,
		`SELECT 1 FROM financial_transactions WHERE id = ? AND deleted_at IS NULL`, transactionID,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return Decision{}, ErrTransactionNotFound
	}
	if err != nil {
		return Decision{}, err
	}

	category, err := Get(ctx, q, categoryID)
	if errors.Is(err, ErrNotFound) || !category.IsActive {
		return Decision{}, ErrCategoryInvalid
	}
	if err != nil {
		return Decision{}, err
	}

	group, err := findInstallmentGroup(ctx, q, transactionID)
	if err != nil {
		return Decision{}, err
	}

	var primary Decision
	for _, id := range group {
		decision, _, err := writeDecision(ctx, q, id, categoryID, OriginManual)
		if err != nil {
			return Decision{}, err
		}
		if id == transactionID {
			primary = decision
		}
	}

	return primary, nil
}

// ApplyRule assigns categoryID to transactionID on behalf of a matching
// automation rule's set_category action. It never overrides a manual
// decision (mirroring InclusionOriginRule never overriding
// InclusionOriginManual in internal/transactions/inclusion.go), but does
// override a prior automatic or rule-origin decision. Like AssignManual, it
// propagates the same decision to every installment of a split card
// purchase.
func ApplyRule(ctx context.Context, conn *sql.DB, transactionID, categoryID string) (Decision, bool, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return Decision{}, false, err
	}
	defer tx.Rollback()

	decision, changed, err := ApplyRuleWithQuerier(ctx, tx, transactionID, categoryID)
	if err != nil {
		return Decision{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Decision{}, false, err
	}
	return decision, changed, nil
}

// ApplyRuleWithQuerier is ApplyRule without owning the transaction boundary.
func ApplyRuleWithQuerier(ctx context.Context, q Querier, transactionID, categoryID string) (Decision, bool, error) {
	var exists int
	err := q.QueryRowContext(ctx,
		`SELECT 1 FROM financial_transactions WHERE id = ? AND deleted_at IS NULL`, transactionID,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return Decision{}, false, ErrTransactionNotFound
	}
	if err != nil {
		return Decision{}, false, err
	}

	category, err := Get(ctx, q, categoryID)
	if errors.Is(err, ErrNotFound) || !category.IsActive {
		return Decision{}, false, ErrCategoryInvalid
	}
	if err != nil {
		return Decision{}, false, err
	}

	existing, err := getDecision(ctx, q, transactionID)
	if err != nil {
		return Decision{}, false, err
	}
	if existing != nil && existing.Origin == OriginManual {
		return *existing, false, nil
	}

	group, err := findInstallmentGroup(ctx, q, transactionID)
	if err != nil {
		return Decision{}, false, err
	}

	var primary Decision
	var changed bool
	for _, id := range group {
		decision, didChange, err := writeDecision(ctx, q, id, categoryID, OriginRule)
		if err != nil {
			return Decision{}, false, err
		}
		if id == transactionID {
			primary = decision
			changed = didChange
		}
	}

	return primary, changed, nil
}

// ApplyAutomatic mirrors apply_automatic_category: writes an automatic
// category decision only when the transaction has none yet, and silently
// no-ops (never returns an error a sync run would need to fail on) when
// sourceCategory is absent, unmapped, or maps to a category that no longer
// exists.
func ApplyAutomatic(ctx context.Context, q Querier, transactionID string, sourceCategory *string) error {
	if sourceCategory == nil {
		return nil
	}
	categoryID, ok := SourceCategoryMapping[*sourceCategory]
	if !ok {
		return nil
	}
	existing, err := getDecision(ctx, q, transactionID)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}
	if _, err := Get(ctx, q, categoryID); errors.Is(err, ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	_, _, err = writeDecision(ctx, q, transactionID, categoryID, OriginAutomatic)
	return err
}
