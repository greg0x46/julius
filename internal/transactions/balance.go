package transactions

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/valuation"
)

// CashOnHandByCurrency is how much cash sits in the user's non-credit
// accounts: provider-reported balances plus derived manual balances (see
// AccountBalance), one total per currency. Wealth's cash side reads it
// through CashOnHandIn.
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
// Balances in different currencies are never added together, and there is
// no conversion. An account with a balance but a missing or malformed
// currency_code fails the whole call (wrapping money.ErrMissingCurrency or
// money.ErrInvalidCurrency) instead of being assumed to be BRL.
//
// Credit card accounts are excluded: their balance is owed debt, not cash
// on hand. accountIDs optionally scopes the result; empty means every
// account.
func CashOnHandByCurrency(ctx context.Context, q Querier, accountIDs []string) (money.Balances, error) {
	// The OR must stay parenthesized: without it, `... IS NULL OR ... AND id
	// IN (...)` binds as `... IS NULL OR (... AND id IN (...))`, letting
	// every untyped account escape the accountIDs filter.
	query := `SELECT id, currency_code FROM financial_accounts
	          WHERE (balance IS NOT NULL OR origin = 'manual') AND (account_type IS NULL OR account_type != 'CREDIT')`
	in, args := db.InClause(accountIDs)
	if in != "" {
		query += ` AND id IN (` + in + `)`
	}
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type account struct {
		id       string
		currency sql.NullString
	}
	var accounts []account
	for rows.Next() {
		var a account
		if err := rows.Scan(dbtypes.ID(&a.id), &a.currency); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	totals := money.Balances{}
	for _, a := range accounts {
		raw, err := AccountBalance(ctx, q, a.id)
		if err != nil {
			return nil, err
		}
		if raw == nil {
			continue
		}
		value, err := decimal.NewFromString(*raw)
		if err != nil {
			return nil, fmt.Errorf("parse account balance %q: %w", *raw, err)
		}
		var code *string
		if a.currency.Valid {
			code = &a.currency.String
		}
		currency, err := money.ParseCurrency(code)
		if err != nil {
			return nil, fmt.Errorf("account %s: %w", a.id, err)
		}
		if err := totals.Add(money.Amount{Value: value, Currency: currency}); err != nil {
			return nil, err
		}
	}
	return totals, nil
}

// CashOnHandIn is CashOnHandByCurrency's total for one named currency;
// cash in any other currency is left out, not converted.
func CashOnHandIn(ctx context.Context, q Querier, accountIDs []string, currency string) (decimal.Decimal, error) {
	code, err := money.ParseCurrency(&currency)
	if err != nil {
		return decimal.Decimal{}, err
	}
	totals, err := CashOnHandByCurrency(ctx, q, accountIDs)
	if err != nil {
		return decimal.Decimal{}, err
	}
	return totals.Get(code), nil
}

// CashOnHandValued is CashOnHandByCurrency valued in the BRL base: one
// conversion per currency, and a currency without a rate fails the call.
func CashOnHandValued(ctx context.Context, q Querier, accountIDs []string, valued *valuation.Valuation) (decimal.Decimal, error) {
	balances, err := CashOnHandByCurrency(ctx, q, accountIDs)
	if err != nil {
		return decimal.Zero, err
	}
	total := decimal.Zero
	for _, currency := range balances.Currencies() {
		value, err := valued.Convert(ctx, balances.Get(currency), currency)
		if err != nil {
			return decimal.Zero, err
		}
		total = total.Add(value)
	}
	return total, nil
}
