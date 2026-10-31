package networth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/dates"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/transactions"
	"github.com/greg0x46/julius/internal/valuation"
)

// Series is the wealth at the end of every day from the first cash transaction
// through today, oldest first. That is as far back as cash can be rebuilt:
// before it the balances are not known, and holding them flat would invent a
// figure, so investments older than the first transaction are not plotted. It
// is derived on every call and stores nothing. It reaches back at most
// dates.MaxSpanDays. Today is Compute; each earlier
// day is rebuilt from the same sources:
//
//   - Cash: today's native balances walked backward, reversing every real
//     movement of money in turn (see cashDeltasDescending).
//   - Investments: investments.ValuesOn, the operation ledgers replayed with
//     that day's valuation or quote, and provider holdings priced that day.
//
// Liabilities and future bills are not part of wealth, so nothing else is
// reconstructed.
func Series(ctx context.Context, q Querier, today time.Time) ([]Point, error) {
	return SeriesWith(ctx, q, today, exchange.New(q, nil))
}

// SeriesWith is Series with the exchange rates supplied by the caller.
//
// Each past day is valued with that day's own rates. Foreign balances are
// walked back in their native currency and converted only at the end, so a
// movement is never valued at another day's rate. A day that cannot be valued
// is returned as "unavailable" with no figures, never as zero: a rate that is
// missing (or a provider that is down), a foreign movement with no amount in
// its account's currency lying after it, or a holding with no price that day.
// Today's point is the exception: it fails the call, as Compute does.
func SeriesWith(ctx context.Context, q Querier, today time.Time, rates valuation.Resolver) ([]Point, error) {
	todayDay := dates.Day(today)
	// The rate of "today" is the latest one that exists: a rate source has none
	// for a day still ahead in UTC, which a clock east of UTC reaches first.
	valuedDay := todayDay
	if utcToday := dates.Day(today.UTC()); valuedDay.After(utcToday) {
		valuedDay = utcToday
	}
	valued := valuation.New(rates, valuedDay)
	breakdown, err := ComputeValued(ctx, q, valued)
	if err != nil {
		return nil, err
	}
	live := Point{Day: todayDay, Status: statusOf(valued), Breakdown: breakdown, Valuation: &valued.Metadata}

	start, ok, err := earliestCashTransactionDay(ctx, q)
	if err != nil {
		return nil, err
	}
	yesterday := todayDay.AddDate(0, 0, -1)
	if !ok || start.After(yesterday) {
		return []Point{live}, nil
	}
	// A stray old date (a mistyped year) must not stretch the series: it never
	// reaches back more than dates.MaxSpanDays from today.
	start = dates.ClampSpanStart(start, todayDay)

	currentCash, err := transactions.CashOnHandByCurrency(ctx, q, nil)
	if err != nil {
		return nil, err
	}
	deltas, err := cashDeltasDescending(ctx, q)
	if err != nil {
		return nil, err
	}

	// Walk the window backward accumulating the cash deltas: one ordered load
	// of every transaction answers every day at once.
	type day struct {
		day  time.Time
		cash money.Balances
		// unreconstructable: a movement after this day cannot be reversed out
		// of its account (a foreign account with no amount in its own
		// currency), so no earlier figure of that cash is known.
		unreconstructable bool
	}
	var days []day
	idx := 0
	runningDelta := money.Balances{}
	unreconstructable := false
	for d := yesterday; !d.Before(start); d = d.AddDate(0, 0, -1) {
		dayEnd := d.AddDate(0, 0, 1)
		for idx < len(deltas) && !deltas[idx].occurredAt.Before(dayEnd) {
			if deltas[idx].unreconstructable {
				unreconstructable = true
			} else {
				runningDelta[deltas[idx].currency] = runningDelta[deltas[idx].currency].Add(deltas[idx].delta)
			}
			idx++
		}
		cash := money.Balances{}
		for _, currency := range currentCash.Currencies() {
			cash[currency] = currentCash.Get(currency)
		}
		for _, currency := range runningDelta.Currencies() {
			cash[currency] = cash.Get(currency).Sub(runningDelta.Get(currency))
		}
		days = append(days, day{day: d, cash: cash, unreconstructable: unreconstructable})
	}
	// Built newest first; the series and the investment ledgers run oldest first.
	for i, j := 0, len(days)-1; i < j; i, j = i+1, j-1 {
		days[i], days[j] = days[j], days[i]
	}
	dayList := make([]time.Time, len(days))
	for i, d := range days {
		dayList[i] = d.day
	}
	held, err := investments.ValuesOn(ctx, q, dayList)
	if err != nil {
		return nil, err
	}

	points := make([]Point, 0, len(days)+1)
	for i, d := range days {
		valued := valuation.New(rates, d.day)
		point := Point{Day: d.day, Status: ValuationUnavailable, Valuation: &valued.Metadata}
		if d.unreconstructable || !held[i].Complete {
			points = append(points, point)
			continue
		}
		cash, err := valueBalances(ctx, d.cash, valued)
		var invested decimal.Decimal
		if err == nil {
			invested, err = valueBalances(ctx, held[i].Balances, valued)
		}
		switch {
		case err == nil:
			point.Breakdown = Breakdown{CashBalance: cash, InvestmentBalance: invested}.TotalsFor()
			point.Status = statusOf(valued)
		case !errors.Is(err, valuation.ErrMissingRate):
			return nil, err
		}
		points = append(points, point)
	}
	return append(points, live), nil
}

func statusOf(valued *valuation.Valuation) string {
	if valued.Stale() {
		return ValuationStale
	}
	return ValuationComplete
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
	return dates.Day(earliest.UTC()), true, nil
}
