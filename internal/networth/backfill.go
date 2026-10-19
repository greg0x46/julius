package networth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/transactions"
	"github.com/greg0x46/julius/internal/valuation"
)

// Backfill reconstructs and stores net_worth_snapshots for every past day in
// [earliestCashTransactionDay, yesterday] that has no row yet — the full
// depth of available transaction history, not an arbitrary cap. It is safe
// to call on every request: days that already have a row (written by
// Snapshot, an earlier Backfill run, or anything else) are left untouched —
// the per-day INSERT below only ever fires for a captured_at gap, and even
// then uses ON CONFLICT DO NOTHING as a second guard rather than overwriting
// a real Compute-derived snapshot with a reconstructed approximation.
//
// Only three of Breakdown's four categories can be reconstructed for a past
// day:
//
//   - CashBalance: today's non-CREDIT account balances, walked backward by
//     reversing each eligible transaction's effect in turn (same
//     eligibility rules for actual cash, including ignored bank movements).
//   - CreditCardBalance: transactions.CreditCardTransactionTotalAt, which
//     already accepts an arbitrary reference date — reused as-is per bill
//     cycle, so it naturally reports 0 for a day before any known bill
//     closing.
//   - PayablesDebt: the same Links-based sum Compute uses, except a link is
//     only counted once its transaction's occurred_at is on or before the
//     day in question — a debt payment made next week hasn't happened yet
//     as of a backfilled day last week.
//
// InvestmentBalance is NOT reconstructed and is always 0 on a backfilled
// row. financial_investments.balance is a synced snapshot with no dated
// history, and the only other number available — net contributions — nets
// out to a residual gain/loss that was never observed at any specific past
// date (see internal/httpapi's contributionInfo). Any of "hold today's
// value flat", "use net-contributed as an approximation", or "reconstruct"
// would fabricate a number this app has no way to actually know, so
// backfilled points simply omit investments from both InvestmentBalance and
// TotalAssets rather than present a guess as fact. SnapshotRow.IsBackfilled
// is what lets the frontend flag this to the user instead of silently
// showing a lower total on past days.
func Backfill(ctx context.Context, q Querier, today time.Time) error {
	return BackfillWith(ctx, q, today, exchange.New(q, nil))
}

// BackfillWith is Backfill with the exchange rates supplied by the caller.
//
// Each past day is valued with that day's own rates, and its row records them.
// Past foreign balances are walked back in their native currency and converted
// only at the end, so a movement is never valued at another day's rate. When a
// rate cannot be resolved the day is skipped without storing anything, so a
// later read retries it. A day that can never be valued (a foreign movement
// with no amount in its account's currency lies after it) is stored once with
// status "unavailable" and no figures, never as zero.
func BackfillWith(ctx context.Context, q Querier, today time.Time, rates valuation.Resolver) error {
	todayDay := formatDate(today)
	todayBoundary, err := dbtypes.ParseDate(todayDay)
	if err != nil {
		return err
	}

	earliest, ok, err := earliestCashTransactionDay(ctx, q)
	if err != nil {
		return err
	}
	if !ok {
		// No non-CREDIT transaction has ever been recorded, so cash — the
		// one category every backfilled day requires — can't be
		// reconstructed for any day at all.
		return nil
	}

	windowStart := earliest
	yesterday := todayBoundary.AddDate(0, 0, -1)
	if windowStart.After(yesterday) {
		return nil
	}

	existing, err := existingCapturedDays(ctx, q, windowStart, yesterday)
	if err != nil {
		return err
	}

	// The common read has nothing to fill: skip loading the whole ledger.
	missing := false
	for d := yesterday; !d.Before(windowStart); d = d.AddDate(0, 0, -1) {
		if !existing[formatDate(d)] {
			missing = true
			break
		}
	}
	if !missing {
		return nil
	}

	currentCash, err := transactions.CashOnHandByCurrency(ctx, q, nil)
	if err != nil {
		return err
	}
	deltas, err := cashDeltasDescending(ctx, q)
	if err != nil {
		return err
	}

	// First pass: walk the window backward accumulating the cash deltas, and
	// collect the days that actually need a row. Nothing is queried per day
	// here — the point of the backward walk is that one ordered load of
	// every transaction answers every day at once.
	type gap struct {
		day, dayEnd time.Time
		cash        money.Balances
		// unreconstructable: a movement after this day cannot be reversed out
		// of its account (a foreign account with no amount in its own
		// currency), so no later-or-earlier figure of that cash is known.
		unreconstructable bool
	}
	var gaps []gap
	idx := 0
	runningDelta := money.Balances{}
	unreconstructable := false
	for d := yesterday; !d.Before(windowStart); d = d.AddDate(0, 0, -1) {
		dayEnd := d.AddDate(0, 0, 1)
		for idx < len(deltas) && !deltas[idx].occurredAt.Before(dayEnd) {
			if deltas[idx].unreconstructable {
				unreconstructable = true
			} else {
				runningDelta[deltas[idx].currency] = runningDelta[deltas[idx].currency].Add(deltas[idx].delta)
			}
			idx++
		}
		if existing[formatDate(d)] {
			continue
		}
		// Native balances per currency: nothing is converted until the day's
		// own rates are known, so a movement is never valued at another day's rate.
		cash := money.Balances{}
		for _, currency := range currentCash.Currencies() {
			cash[currency] = currentCash.Get(currency)
		}
		for _, currency := range runningDelta.Currencies() {
			cash[currency] = cash.Get(currency).Sub(runningDelta.Get(currency))
		}
		gaps = append(gaps, gap{day: d, dayEnd: dayEnd, cash: cash, unreconstructable: unreconstructable})
	}
	if len(gaps) == 0 {
		return nil
	}

	// The payables side gets the same treatment: one load of every payable
	// and every transaction behind its links answers all the days, instead
	// of re-reading them once per day (see payables.RemainingTotalsAsOf).
	dayEnds := make([]time.Time, len(gaps))
	for i, g := range gaps {
		dayEnds[i] = g.dayEnd
	}
	debts, err := payables.RemainingTotalsAsOf(ctx, q, payables.KindDebt, dayEnds)
	if err != nil {
		return err
	}

	for i, g := range gaps {
		valued := valuation.New(rates, g.day)
		if g.unreconstructable {
			// Deterministic, so it is stored: retrying cannot change it.
			if err := insertBackfillSnapshot(ctx, q, formatDate(g.day), Breakdown{}.TotalsFor(), valued, ValuationUnavailable); err != nil {
				return err
			}
			continue
		}
		cash, err := valueBalances(ctx, g.cash, valued)
		if errors.Is(err, valuation.ErrMissingRate) {
			// Possibly transient (provider down): store nothing so a later
			// read retries this day instead of freezing it as unavailable.
			if errors.Is(err, exchange.ErrProviderFailure) {
				return nil // every older day would hit the same outage
			}
			continue
		}
		if err != nil {
			return err
		}
		// The card side still costs a query per day: its cycle math keys off
		// time.Local calendar days and has no as-of-many-days form, so
		// there is nothing to hoist out of the loop here yet.
		loc := time.Local
		reference := time.Date(g.day.Year(), g.day.Month(), g.day.Day(), 12, 0, 0, 0, loc)
		creditCard, err := transactions.CreditCardBalanceValued(ctx, q, reference, loc, valued)
		if errors.Is(err, valuation.ErrMissingRate) {
			if errors.Is(err, exchange.ErrProviderFailure) {
				return nil
			}
			continue
		}
		if err != nil {
			return err
		}
		breakdown := Breakdown{CashBalance: cash, CreditCardBalance: creditCard, PayablesDebt: debts[i]}.TotalsFor()
		if err := insertBackfillSnapshot(ctx, q, formatDate(g.day), breakdown, valued, ValuationReconstructed); err != nil {
			return err
		}
	}
	return nil
}

// existingCapturedDays returns the set of captured_at values (YYYY-MM-DD)
// already stored in [from, to], so Backfill can skip them without an
// INSERT round trip per day.
func existingCapturedDays(ctx context.Context, q Querier, from, to time.Time) (map[string]bool, error) {
	rows, err := q.QueryContext(ctx, `SELECT captured_at FROM net_worth_snapshots WHERE captured_at >= ? AND captured_at <= ?`,
		formatDate(from), formatDate(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]bool)
	for rows.Next() {
		var capturedAt string
		if err := rows.Scan(dbtypes.Date(&capturedAt)); err != nil {
			return nil, err
		}
		result[capturedAt] = true
	}
	return result, rows.Err()
}

// valueBalances values native per-currency balances in the BRL base.
func valueBalances(ctx context.Context, balances money.Balances, valued *valuation.Valuation) (decimal.Decimal, error) {
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

func insertBackfillSnapshot(ctx context.Context, q Querier, capturedAt string, breakdown Breakdown, valued *valuation.Valuation, status string) error {
	now := db.FormatTime(time.Now())
	rates, err := json.Marshal(valued.Rates)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `
		INSERT INTO net_worth_snapshots (
			id, captured_at, total_assets, total_liabilities, net_worth,
			cash_balance, investment_balance, credit_card_balance, payables_debt,
			is_backfilled, created_at, updated_at,
			valuation_version, valuation_status, valued_at, rate_policy, valuation_rates
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, TRUE, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (captured_at) DO NOTHING`,
		uuid.NewString(), capturedAt,
		money.CanonicalDecimal(breakdown.TotalAssets), money.CanonicalDecimal(breakdown.TotalLiabilities), money.CanonicalDecimal(breakdown.NetWorth),
		money.CanonicalDecimal(breakdown.CashBalance), money.CanonicalDecimal(breakdown.InvestmentBalance), money.CanonicalDecimal(breakdown.CreditCardBalance),
		money.CanonicalDecimal(breakdown.PayablesDebt),
		now, now,
		ValuationVersion, status, db.FormatTime(valued.ValuedAt), valued.RatePolicy, string(rates),
	)
	return err
}

// cashTransactionDelta is one non-CREDIT-account transaction's effect on
// cash balance, sign-normalized so a positive delta always means "balance
// went up" — the opposite convention from consideredCardTransactionTotal's
// debt indicator, since cash is an asset and card balance is a liability.
type cashTransactionDelta struct {
	occurredAt time.Time
	// delta is in currency, the account's own currency.
	delta    decimal.Decimal
	currency string
	// unreconstructable: the movement is on a foreign account but carries no
	// amount in that account's currency, so it cannot be reversed.
	unreconstructable bool
}

// cashDeltasDescending loads every non-CREDIT-account transaction's real
// balance effect, newest first — the order Backfill's single backward walk
// over calendar days needs to accumulate "everything that happened after
// day D" without re-querying per day.
//
// This deliberately reverses every transaction with a usable amount,
// including ones the user has marked "ignored". Ignored only means "don't
// count this toward income/expense totals" (e.g. a transfer to an
// untracked account) — it still moved real money through the bank account,
// so today's real financial_accounts.balance already reflects it and a
// past day's reconstructed balance must reverse it out too, or every day
// before an ignored transaction ends up off by its full amount.
//
// Manual entries on synced accounts are excluded: they never changed the
// provider balance. Manual accounts reverse their posted movements and
// adjustment deltas; opening balance remains the historical baseline.
func cashDeltasDescending(ctx context.Context, q Querier) ([]cashTransactionDelta, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT ft.amount, ft.amount_in_account_currency, ft.currency_code, fa.currency_code,
		       ft.occurred_at, ft.provider_status, ft.movement_type
		FROM financial_transactions ft
		JOIN financial_accounts fa ON fa.id = ft.account_id
		WHERE (fa.account_type IS NULL OR fa.account_type != 'CREDIT') AND ft.deleted_at IS NULL AND (ft.origin = 'synced' OR (fa.origin = 'manual' AND ft.provider_status = 'POSTED'))
 UNION ALL SELECT aba.amount, aba.amount, fa.currency_code, fa.currency_code,
 aba.created_at, 'POSTED', CASE WHEN SUBSTR(CAST(aba.amount AS TEXT), 1, 1) = '-' THEN 'DEBIT' ELSE 'CREDIT' END
 FROM account_balance_adjustments aba JOIN financial_accounts fa ON fa.id = aba.account_id`)
	if err != nil {
		return nil, fmt.Errorf("query cash transactions: %w", err)
	}
	defer rows.Close()

	result := make([]cashTransactionDelta, 0)
	for rows.Next() {
		var amountRaw, amountInAccountCurrencyRaw, currencyCodeRaw, accountCurrencyRaw sql.NullString
		var occurredAtRaw, providerStatusRaw, movementTypeRaw sql.NullString
		if err := rows.Scan(
			dbtypes.Decimal(&amountRaw), dbtypes.Decimal(&amountInAccountCurrencyRaw), &currencyCodeRaw, &accountCurrencyRaw,
			dbtypes.Time(&occurredAtRaw), &providerStatusRaw, &movementTypeRaw,
		); err != nil {
			return nil, err
		}
		if !occurredAtRaw.Valid {
			// No occurred_at means there's no day to attribute this
			// transaction's effect to — it can never be reversed out of a
			// specific backfilled day, so it's left in every day's balance
			// (equivalent to treating it as having always already happened).
			continue
		}
		occurredAt, err := dbtypes.ParseTime(occurredAtRaw.String)
		if err != nil {
			return nil, err
		}
		amount, err := decimalFromNullString(amountRaw)
		if err != nil {
			return nil, err
		}
		amountInAccountCurrency, err := decimalFromNullString(amountInAccountCurrencyRaw)
		if err != nil {
			return nil, err
		}
		var currencyCode, accountCurrency, providerStatus, movementType *string
		if currencyCodeRaw.Valid {
			currencyCode = &currencyCodeRaw.String
		}
		if accountCurrencyRaw.Valid {
			accountCurrency = &accountCurrencyRaw.String
		}
		if providerStatusRaw.Valid {
			providerStatus = &providerStatusRaw.String
		}
		if movementTypeRaw.Valid {
			movementType = &movementTypeRaw.String
		}

		classification := money.Classify(movementType)
		effective := money.SelectEffectiveMoney(amountInAccountCurrency, accountCurrency, amount, currencyCode)
		// Considered and "" (no category kind) are both deliberate, for the
		// same reason the doc comment above gives: a transfer categorized as
		// such still moved real money out of this account — to another
		// account, tracked or not — so today's reported balance reflects it
		// and a past day's reconstruction must reverse it out too. Letting
		// the transfer rule reach here would leave every day before a
		// transfer off by its full amount.
		included, _ := money.Eligibility(classification, providerStatus, effective, money.Considered, "")
		if !included || effective == nil || accountCurrency == nil {
			continue
		}
		if effective.CurrencyCode != *accountCurrency {
			// Only the account-currency amount moved the account's balance. A
			// BRL account whose entry carries just a foreign amount never
			// counted it, as before; a foreign account is a gap in the
			// history, not something to guess at.
			if *accountCurrency != money.BRL {
				result = append(result, cashTransactionDelta{occurredAt: occurredAt, unreconstructable: true})
			}
			continue
		}

		delta := effective.Value.Abs()
		if classification == money.Outflow {
			delta = delta.Neg()
		}
		result = append(result, cashTransactionDelta{occurredAt: occurredAt, delta: delta, currency: *accountCurrency})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool { return result[i].occurredAt.After(result[j].occurredAt) })
	return result, nil
}

func decimalFromNullString(value sql.NullString) (*decimal.Decimal, error) {
	if !value.Valid {
		return nil, nil
	}
	amount, err := decimal.NewFromString(value.String)
	if err != nil {
		return nil, fmt.Errorf("parse decimal %q: %w", value.String, err)
	}
	return &amount, nil
}

// earliestCashTransactionDay returns the calendar day (UTC, matching
// formatDate) of the oldest non-CREDIT-account synced transaction on record,
// regardless of its eligibility — this is a proxy for "how far back do we
// actually have transaction coverage", which an ineligible (e.g. ignored)
// transaction still answers just as well as an eligible one. origin =
// 'synced' excludes lançamentos manuais for the same reason
// cashDeltasDescending does — they answer nothing about how far back
// Pluggy's own coverage reaches. ok is false when there is no such
// transaction at all.
func earliestCashTransactionDay(ctx context.Context, q Querier) (day time.Time, ok bool, err error) {
	var raw sql.NullString
	err = q.QueryRowContext(ctx, `
		SELECT MIN(occurred_at) FROM (SELECT ft.occurred_at
		FROM financial_transactions ft
		JOIN financial_accounts fa ON fa.id = ft.account_id
		WHERE (fa.account_type IS NULL OR fa.account_type != 'CREDIT')
		  AND ft.occurred_at IS NOT NULL AND ft.deleted_at IS NULL AND (ft.origin = 'synced' OR (fa.origin = 'manual' AND ft.provider_status = 'POSTED'))
 UNION ALL SELECT aba.created_at FROM account_balance_adjustments aba
 JOIN financial_accounts fa ON fa.id = aba.account_id) history`).Scan(dbtypes.Time(&raw))
	if err != nil || !raw.Valid {
		return time.Time{}, false, err
	}
	earliest, err := dbtypes.ParseTime(raw.String)
	if err != nil {
		return time.Time{}, false, err
	}
	dayString := formatDate(earliest)
	dayBoundary, err := dbtypes.ParseDate(dayString)
	if err != nil {
		return time.Time{}, false, err
	}
	return dayBoundary, true, nil
}
