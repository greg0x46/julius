package transactions

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
)

// CashOnHand is how much cash sits in the user's non-credit accounts: the
// sum of the providers' reported balances. It is the one implementation
// every cash-side reader goes through — the timeline's t=today anchor and
// net worth's asset side — so those two views cannot drift apart.
//
// The reported balance is authoritative here, and deliberately so. A
// transaction the user marked "ignored" still moved real money through the
// bank account; "ignored" is a reporting decision about what counts toward
// income/expense totals (a transfer to an untracked account, say), not a
// claim that the money never left. Backing such a transaction out of the
// balance would report cash the user does not have.
//
// That is a statement about this number only, not about what a balance
// *reconstruction* should walk: timeline's series drops ignored rows (via
// money.MovedCash) because the ones seen in practice are duplicates and
// reversals the bank never counted as separate movements. The two rules
// never conflict on the number itself — nothing ever backs an ignored
// transaction out of the reported balance — and internal/timeline's
// eligibleRealItems documents where the remaining disagreement lands.
//
// This is the opposite of the credit-card side, where
// CreditCardTransactionTotal computes what is owed from eligible
// transactions rather than the reported balance — there, the question is
// "what will this bill charge me", which inclusion decisions legitimately
// shape.
//
// Credit card accounts are excluded: their balance is owed debt, not cash
// on hand. accountIDs optionally scopes the result; empty means every
// account.
func CashOnHand(ctx context.Context, q Querier, accountIDs []string) (decimal.Decimal, error) {
	// The OR must stay parenthesized: without it, `... IS NULL OR ... AND id
	// IN (...)` binds as `... IS NULL OR (... AND id IN (...))`, letting
	// every untyped account escape the accountIDs filter.
	query := `SELECT id FROM financial_accounts
	          WHERE (balance IS NOT NULL OR origin = 'manual') AND (currency_code IS NULL OR currency_code = 'BRL') AND (account_type IS NULL OR account_type != 'CREDIT')`
	in, args := db.InClause(accountIDs)
	if in != "" {
		query += ` AND id IN (` + in + `)`
	}
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return decimal.Decimal{}, err
	}
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return decimal.Zero, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return decimal.Zero, err
	}
	rows.Close()
	total := decimal.Zero
	for _, id := range ids {
		raw, err := AccountBalance(ctx, q, id)
		if err != nil {
			return decimal.Zero, err
		}
		if raw == nil {
			continue
		}
		amount, err := decimal.NewFromString(*raw)
		if err != nil {
			return decimal.Zero, fmt.Errorf("parse account balance: %w", err)
		}
		total = total.Add(amount)
	}
	return total, nil
}
