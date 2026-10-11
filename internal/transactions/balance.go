package transactions

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/fx"
)

// CashOnHand is how much cash sits in the user's non-credit accounts: the
// sum of the providers' reported balances valued in BRL. It is the one implementation
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
	return CashOnHandValued(ctx, q, accountIDs, fx.New(time.Now()))
}

func CashOnHandValued(ctx context.Context, q Querier, accountIDs []string, valuation *fx.Valuation) (decimal.Decimal, error) {
	balances, err := CashBalances(ctx, q, accountIDs)
	if err != nil {
		return decimal.Zero, err
	}
	total := decimal.Zero
	for currency, balance := range balances {
		value, err := valuation.Convert(ctx, q, balance, currency)
		if err != nil {
			return decimal.Zero, err
		}
		total = total.Add(value)
	}
	return total, nil
}

// CashBalances returns provider balances grouped by native currency.
func CashBalances(ctx context.Context, q Querier, accountIDs []string) (map[string]decimal.Decimal, error) {
	// The OR must stay parenthesized: without it, `... IS NULL OR ... AND id
	// IN (...)` binds as `... IS NULL OR (... AND id IN (...))`, letting
	// every untyped account escape the accountIDs filter.
	query := `SELECT balance, currency_code FROM financial_accounts
	          WHERE balance IS NOT NULL AND (account_type IS NULL OR account_type != 'CREDIT')`
	in, args := db.InClause(accountIDs)
	if in != "" {
		query += ` AND id IN (` + in + `)`
	}
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	total := map[string]decimal.Decimal{}
	for rows.Next() {
		// A plain string, not sql.NullString: the query already filters
		// `balance IS NOT NULL`, so a NULL here would be a schema surprise
		// worth surfacing as a scan error rather than silently summing "".
		var balanceRaw string
		var currency sql.NullString
		if err := rows.Scan(&balanceRaw, &currency); err != nil {
			return nil, err
		}
		amount, err := decimal.NewFromString(balanceRaw)
		if err != nil {
			return nil, fmt.Errorf("parse account balance %q: %w", balanceRaw, err)
		}
		code := fx.Currency(&currency.String)
		total[code] = total[code].Add(amount)
	}
	return total, rows.Err()
}
