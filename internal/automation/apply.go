package automation

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/greg0x46/julius/internal/categories"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/transactions"
)

// extractCardNumber pulls the card number out of a financial_transactions
// credit_card_metadata blob. The key is "cardNumber" (camelCase) because
// package pluggy stores that JSON blob verbatim from Pluggy's wire format
// rather than renaming it to snake_case the way the Python reference's typed
// dataclass mapping does — see pluggy.TransactionSnapshot's doc comment.
func extractCardNumber(metadata string) string {
	var parsed map[string]any
	if err := json.Unmarshal([]byte(metadata), &parsed); err != nil {
		return ""
	}
	card, _ := parsed["cardNumber"].(string)
	return card
}

func candidateFor(ctx context.Context, q transactions.Querier, transactionID string) (MatchCandidate, error) {
	var description, creditCardMetadata, accountName, accountInstitution sql.NullString
	var amount, accountAmount, currency, accountCurrency, occurredAt sql.NullString
	err := q.QueryRowContext(ctx, `
		SELECT ft.description, ft.credit_card_metadata, fa.name, fa.institution,
		       ft.amount, ft.amount_in_account_currency, ft.currency_code, fa.currency_code, ft.occurred_at
		FROM financial_transactions ft JOIN financial_accounts fa ON fa.id = ft.account_id
		WHERE ft.id = ?`, transactionID,
	).Scan(&description, &creditCardMetadata, &accountName, &accountInstitution, dbtypes.Decimal(&amount), dbtypes.Decimal(&accountAmount), &currency, &accountCurrency, dbtypes.Time(&occurredAt))
	if err != nil {
		return MatchCandidate{}, err
	}
	candidate, err := moneyCandidate(amount, accountAmount, currency, accountCurrency, occurredAt)
	if err != nil {
		return MatchCandidate{}, err
	}
	if description.Valid {
		candidate.Description = &description.String
	}
	if accountName.Valid {
		candidate.AccountName = &accountName.String
	}
	if accountInstitution.Valid {
		candidate.AccountInstitution = &accountInstitution.String
	}
	if creditCardMetadata.Valid {
		if card := extractCardNumber(creditCardMetadata.String); card != "" {
			candidate.CardNumber = &card
		}
	}
	return candidate, nil
}

func moneyCandidate(amount, accountAmount, currency, accountCurrency, occurredAt sql.NullString) (MatchCandidate, error) {
	candidate := MatchCandidate{}
	value, err := dbtypes.ParseNullDecimal(amount)
	if err != nil {
		return candidate, err
	}
	accountValue, err := dbtypes.ParseNullDecimal(accountAmount)
	if err != nil {
		return candidate, err
	}
	var currencyCode, accountCurrencyCode *string
	if currency.Valid {
		currencyCode = &currency.String
	}
	if accountCurrency.Valid {
		accountCurrencyCode = &accountCurrency.String
	}
	if effective := money.SelectEffectiveMoney(accountValue, accountCurrencyCode, value, currencyCode); effective != nil {
		absolute := effective.Value.Abs()
		candidate.Amount = &absolute
	}
	date, err := dbtypes.ParseNullTime(occurredAt)
	if err != nil {
		return candidate, err
	}
	if date != nil {
		day := date.Day()
		candidate.DayOfMonth = &day
	}
	return candidate, nil
}

// findAction returns rule's first action of type t, if any.
func findAction(rule Rule, t ActionType) (Action, bool) {
	for _, a := range rule.Actions {
		if a.Type == t {
			return a, true
		}
	}
	return Action{}, false
}

// isReconcileOnlyRule reports whether rule's only action is "reconcile" —
// such rules have nothing to do with new-transaction/retroactive
// processing, only with internal/timeline's read-time projection (see
// ListActiveReconcileTargets).
func isReconcileOnlyRule(rule Rule) bool {
	return len(rule.Actions) == 1 && rule.Actions[0].Type == ActionReconcile
}

// ApplyToNewTransaction mirrors apply_rules_to_new_transaction: rules are
// evaluated in creation order; a matching rule's "ignore" and
// "set_category" actions (if present) both fire. Once a matching rule's
// "ignore" action fires, evaluation stops — an ignored transaction has
// nothing left to categorize — matching the reference's early return. A
// matching rule whose only action is "set_category" (no "ignore") applies
// it and evaluation continues, so a later rule can still ignore the same
// transaction. A rule whose only action is "reconcile" is skipped here
// entirely — see isReconcileOnlyRule. Despite the name, it also runs for a
// resync's update path (see NewTransactionHook below and
// transactions.UpsertedHook) so a transaction that starts out
// matching no rule gets re-evaluated once a later sync changes a field a
// rule cares about — applyInclusion (package transactions) and
// categories.ApplyRule already refuse to let a rule-origin decision
// overwrite a manual one, so re-running this on an update is safe by
// construction.
func ApplyToNewTransaction(
	ctx context.Context, conn *sql.DB, transactionID string, onIgnored transactions.OnIgnoredHook,
) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := ApplyToNewTransactionWithQuerier(ctx, tx, transactionID, onIgnored); err != nil {
		return err
	}
	return tx.Commit()
}

// ApplyToNewTransactionWithQuerier applies automation inside a transaction
// boundary owned by the caller.
func ApplyToNewTransactionWithQuerier(
	ctx context.Context, q transactions.Querier, transactionID string, onIgnored transactions.OnIgnoredHook,
) error {
	rules, err := ListActive(ctx, q)
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return nil
	}
	candidate, err := candidateFor(ctx, q, transactionID)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if isReconcileOnlyRule(rule) {
			continue
		}
		if !Matches(candidate, rule.Conditions, rule.LogicOperator) {
			continue
		}
		if setCategory, ok := findAction(rule, ActionSetCategory); ok {
			if _, _, err := categories.ApplyRuleWithQuerier(ctx, q, transactionID, *setCategory.CategoryID); err != nil {
				return err
			}
		}
		if _, ok := findAction(rule, ActionIgnore); ok {
			_, err := transactions.SetInclusion(
				ctx, q, transactionID, money.Ignored, transactions.InclusionOriginRule, &rule.ID, &rule.Name, onIgnored,
			)
			return err
		}
	}
	return nil
}

// NewTransactionHook adapts ApplyToNewTransactionWithQuerier to
// transactions.UpsertedHook, closing over onIgnored (the payable-unlink
// hook from package payables; nil is a valid no-op).
func NewTransactionHook(onIgnored transactions.OnIgnoredHook) transactions.UpsertedHook {
	return func(ctx context.Context, q transactions.Querier, transactionID, _ string) error {
		return ApplyToNewTransactionWithQuerier(ctx, q, transactionID, onIgnored)
	}
}

// RetroactiveResult mirrors RetroactiveApplyResult.
type RetroactiveResult struct {
	Matched     int
	Ignored     int
	Categorized int
}

// ApplyRetroactively mirrors apply_retroactively: silently returns a zero
// result for an unknown rule id, matching the reference (the caller already
// validated the rule exists when creating/updating it in the same request).
// It applies whichever of the rule's "ignore"/"set_category" actions are
// present to every matching transaction; a reconcile-only rule keeps
// returning a zero result (see isReconcileOnlyRule).
func ApplyRetroactively(ctx context.Context, conn *sql.DB, ruleID string, onIgnored transactions.OnIgnoredHook) (RetroactiveResult, error) {
	rule, err := Get(ctx, conn, ruleID)
	if err != nil {
		if err == ErrNotFound {
			return RetroactiveResult{}, nil
		}
		return RetroactiveResult{}, err
	}
	_, hasIgnore := findAction(rule, ActionIgnore)
	setCategoryAction, hasSetCategory := findAction(rule, ActionSetCategory)
	if !hasIgnore && !hasSetCategory {
		return RetroactiveResult{}, nil
	}

	rows, err := conn.QueryContext(ctx, `
		SELECT ft.id, ft.description, ft.credit_card_metadata, fa.name, fa.institution,
		       ft.amount, ft.amount_in_account_currency, ft.currency_code, fa.currency_code, ft.occurred_at
		FROM financial_transactions ft JOIN financial_accounts fa ON fa.id = ft.account_id
		WHERE ft.deleted_at IS NULL`)
	if err != nil {
		return RetroactiveResult{}, err
	}
	type row struct {
		id        string
		candidate MatchCandidate
	}
	var candidates []row
	for rows.Next() {
		var id string
		var description, creditCardMetadata, accountName, accountInstitution sql.NullString
		var amount, accountAmount, currency, accountCurrency, occurredAt sql.NullString
		if err := rows.Scan(dbtypes.ID(&id), &description, &creditCardMetadata, &accountName, &accountInstitution, dbtypes.Decimal(&amount), dbtypes.Decimal(&accountAmount), &currency, &accountCurrency, dbtypes.Time(&occurredAt)); err != nil {
			rows.Close()
			return RetroactiveResult{}, err
		}
		candidate, err := moneyCandidate(amount, accountAmount, currency, accountCurrency, occurredAt)
		if err != nil {
			rows.Close()
			return RetroactiveResult{}, err
		}
		if description.Valid {
			candidate.Description = &description.String
		}
		if accountName.Valid {
			candidate.AccountName = &accountName.String
		}
		if accountInstitution.Valid {
			candidate.AccountInstitution = &accountInstitution.String
		}
		if creditCardMetadata.Valid {
			if card := extractCardNumber(creditCardMetadata.String); card != "" {
				candidate.CardNumber = &card
			}
		}
		candidates = append(candidates, row{id: id, candidate: candidate})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return RetroactiveResult{}, err
	}
	rows.Close()

	var result RetroactiveResult
	for _, c := range candidates {
		if !Matches(c.candidate, rule.Conditions, rule.LogicOperator) {
			continue
		}
		result.Matched++
		if hasSetCategory {
			_, changed, err := categories.ApplyRule(ctx, conn, c.id, *setCategoryAction.CategoryID)
			if err != nil {
				return RetroactiveResult{}, err
			}
			if changed {
				result.Categorized++
			}
		}
		if hasIgnore {
			confirmation, err := transactions.SetInclusion(
				ctx, conn, c.id, money.Ignored, transactions.InclusionOriginRule, &rule.ID, &rule.Name, onIgnored,
			)
			if err != nil {
				return RetroactiveResult{}, err
			}
			if confirmation.Changed {
				result.Ignored++
			}
		}
	}
	return result, nil
}
