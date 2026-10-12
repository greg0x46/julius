package transactions

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/money"
)

// InclusionOrigin distinguishes a manual toggle from one an automation rule
// applied.
type InclusionOrigin string

const (
	InclusionOriginManual InclusionOrigin = "manual"
	InclusionOriginRule   InclusionOrigin = "rule"
)

// InclusionConfirmation mirrors InclusionConfirmation: the resulting state
// after applying (or no-op'ing) an inclusion change.
type InclusionConfirmation struct {
	TransactionID string
	State         money.InclusionState
	ChangedAt     *time.Time
	Applied       bool
	Changed       bool
}

// ErrTransactionNotFound is returned by SetInclusion when transactionID has
// no matching row.
var ErrTransactionNotFound = errors.New("transaction not found")

// OnIgnoredHook runs, within the same transaction, whenever a transaction
// transitions into the "ignored" state. It exists so package payables can
// unlink a payable-transaction link without this package importing
// payables — package transactions is a dependency payables already has the
// other way around (eligibility_for_link consumes
// classify/select_effective_money), so the reverse import would cycle.
// Callers pass nil until that hook exists.
type OnIgnoredHook func(ctx context.Context, q Querier, transactionID string) error

// UpsertedHook is called with a transaction's id whenever an ingestion run
// inserts it for the first time, or updates it because a resync changed its
// normalized hash — not on "unchanged" outcomes, where nothing about the
// transaction could newly match a rule. It lives here, on the Lançamentos
// engine, rather than on the ingestion package that calls it: the engines
// that react to a new lançamento (automation) must not have to import a
// provider to describe their own entry point. Package syncsvc aliases this
// type as TransactionUpsertedHook; nil is a valid no-op. The caller owns the
// transaction boundary: hooks must use q for all database effects so failure
// rolls back both ingestion and its consequences.
type UpsertedHook func(ctx context.Context, q Querier, transactionID, accountID string) error

type inclusionDecision struct {
	state     money.InclusionState
	revision  int
	changedAt time.Time
	origin    InclusionOrigin
	ruleID    *string
	ruleName  *string
}

func getInclusionDecision(ctx context.Context, q Querier, transactionID string) (*inclusionDecision, error) {
	var (
		d            inclusionDecision
		changedAtRaw string
		ruleID       sql.NullString
		ruleName     sql.NullString
	)
	err := q.QueryRowContext(ctx,
		`SELECT state, revision, changed_at, origin, rule_id, rule_name
		 FROM transaction_inclusion_decisions WHERE transaction_id = ?`,
		transactionID,
	).Scan(&d.state, &d.revision, dbtypes.Time(&changedAtRaw), &d.origin, dbtypes.ID(&ruleID), &ruleName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if d.changedAt, err = dbtypes.ParseTime(changedAtRaw); err != nil {
		return nil, err
	}
	d.ruleID = nullString(ruleID)
	d.ruleName = nullString(ruleName)
	return &d, nil
}

func upsertInclusionDecision(ctx context.Context, q Querier, transactionID string, d inclusionDecision) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO transaction_inclusion_decisions
			(transaction_id, state, revision, changed_at, origin, rule_id, rule_name)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (transaction_id) DO UPDATE SET
			state = excluded.state,
			revision = excluded.revision,
			changed_at = excluded.changed_at,
			origin = excluded.origin,
			rule_id = excluded.rule_id,
			rule_name = excluded.rule_name`,
		transactionID, string(d.state), d.revision, db.FormatTime(d.changedAt), string(d.origin), d.ruleID, d.ruleName,
	)
	return err
}

// applyInclusion mirrors apply_transaction_inclusion. It assumes the caller
// has already confirmed transactionID exists (SetInclusion does that; the
// sync pipeline, in a later phase, will already hold the row from its own
// upsert).
func applyInclusion(
	ctx context.Context, q Querier, transactionID string, target money.InclusionState,
	origin InclusionOrigin, ruleID, ruleName *string, onIgnored OnIgnoredHook,
) (InclusionConfirmation, error) {
	existing, err := getInclusionDecision(ctx, q, transactionID)
	if err != nil {
		return InclusionConfirmation{}, err
	}

	current := money.Considered
	if existing != nil {
		current = existing.state
	}

	if origin == InclusionOriginRule && existing != nil && existing.origin == InclusionOriginManual {
		return InclusionConfirmation{
			TransactionID: transactionID,
			State:         current,
			ChangedAt:     &existing.changedAt,
			Applied:       false,
		}, nil
	}

	if current == target {
		if origin == InclusionOriginManual && existing != nil && existing.origin != InclusionOriginManual {
			claimed := *existing
			claimed.origin = InclusionOriginManual
			claimed.ruleID = nil
			claimed.ruleName = nil
			if err := upsertInclusionDecision(ctx, q, transactionID, claimed); err != nil {
				return InclusionConfirmation{}, err
			}
		}
		var changedAt *time.Time
		if existing != nil {
			changedAt = &existing.changedAt
		}
		return InclusionConfirmation{TransactionID: transactionID, State: target, ChangedAt: changedAt, Applied: true}, nil
	}

	changedAt := time.Now().UTC()
	revision := 1
	if existing != nil {
		revision = existing.revision + 1
	}
	var decisionRuleID, decisionRuleName, eventRuleID, eventRuleName *string
	if origin == InclusionOriginRule {
		decisionRuleID, decisionRuleName = ruleID, ruleName
		eventRuleID, eventRuleName = ruleID, ruleName
	}

	if err := upsertInclusionDecision(ctx, q, transactionID, inclusionDecision{
		state: target, revision: revision, changedAt: changedAt, origin: origin,
		ruleID: decisionRuleID, ruleName: decisionRuleName,
	}); err != nil {
		return InclusionConfirmation{}, err
	}
	_, err = q.ExecContext(ctx,
		`INSERT INTO transaction_inclusion_events
			(id, transaction_id, revision, previous_state, resulting_state, changed_at, origin, rule_id, rule_name)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), transactionID, revision, string(current), string(target),
		db.FormatTime(changedAt), string(origin), eventRuleID, eventRuleName,
	)
	if err != nil {
		return InclusionConfirmation{}, err
	}

	if target == money.Ignored && onIgnored != nil {
		if err := onIgnored(ctx, q, transactionID); err != nil {
			return InclusionConfirmation{}, err
		}
	}

	return InclusionConfirmation{
		TransactionID: transactionID,
		State:         target,
		ChangedAt:     &changedAt,
		Applied:       true,
		Changed:       true,
	}, nil
}

// SetInclusion mirrors set_transaction_inclusion: it confirms the
// transaction exists, then applies the inclusion change. Returns
// ErrTransactionNotFound when it doesn't.
func SetInclusion(
	ctx context.Context, q Querier, transactionID string, target money.InclusionState,
	origin InclusionOrigin, ruleID, ruleName *string, onIgnored OnIgnoredHook,
) (InclusionConfirmation, error) {
	var exists int
	err := q.QueryRowContext(ctx, `SELECT 1 FROM financial_transactions WHERE id = ? AND deleted_at IS NULL`, transactionID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return InclusionConfirmation{}, ErrTransactionNotFound
	}
	if err != nil {
		return InclusionConfirmation{}, err
	}
	return applyInclusion(ctx, q, transactionID, target, origin, ruleID, ruleName, onIgnored)
}
