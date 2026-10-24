package investments

import (
	"context"
	"database/sql"
	"time"

	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/valuation"
)

// DayValues is what the investment book held at the end of one day, by native
// currency. Complete is false when some holding could not be priced that day,
// so the sum would understate it.
type DayValues struct {
	Balances money.Balances
	Complete bool
}

// ValuesOn is the investment book at the end of each of days (ascending, one
// calendar day each), derived from the operation ledgers, the price series and
// the provider movements. It applies the same rules as ManualNetWorth for the
// manual book — a dated valuation or the latest quote on or before the day,
// else cost, plus book cash that is not already a linked bank account — so
// its last day agrees with today's figure. Provider holdings have no dated
// balance: they are today's quantity, rebuilt backwards, times the latest
// price known that day, and a day before any price is incomplete rather than
// zero.
func ValuesOn(ctx context.Context, q Querier, days []time.Time) ([]DayValues, error) {
	values := make([]DayValues, len(days))
	for i := range values {
		values[i] = DayValues{Balances: money.Balances{}, Complete: true}
	}
	if len(days) == 0 {
		return values, nil
	}
	if err := addManualValues(ctx, q, days, values); err != nil {
		return nil, err
	}
	if err := addSyncedValues(ctx, q, days, values); err != nil {
		return nil, err
	}
	return values, nil
}

type manualHolding struct {
	positionID, assetID string
	quoted              bool
}

func addManualValues(ctx context.Context, q Querier, days []time.Time, values []DayValues) error {
	accounts, err := ListAccounts(ctx, q)
	if err != nil {
		return err
	}
	rows, err := q.QueryContext(ctx, `
		SELECT p.id, p.account_id, p.asset_id, a.quote_source
		FROM investment_positions p JOIN investment_assets a ON a.id = p.asset_id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	holdings := map[string][]manualHolding{}
	for rows.Next() {
		var h manualHolding
		var accountID string
		var quoteSource sql.NullString
		if err := rows.Scan(dbtypes.ID(&h.positionID), dbtypes.ID(&accountID), dbtypes.ID(&h.assetID), &quoteSource); err != nil {
			return err
		}
		h.quoted = quoteSource.Valid && quoteSource.String != ""
		holdings[accountID] = append(holdings[accountID], h)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	quotes := map[string][]AssetQuote{}
	for _, account := range accounts {
		// A linked custody's cash is already the bank account's balance.
		countsCash := account.Kind == AccountKindManual && account.FinancialAccountID == nil
		currency := valuation.Currency(account.CurrencyCode)
		if len(holdings[account.ID]) == 0 && !countsCash {
			continue
		}
		for _, h := range holdings[account.ID] {
			if _, ok := quotes[h.assetID]; ok || !h.quoted {
				continue
			}
			if quotes[h.assetID], err = ListAssetQuotes(ctx, q, h.assetID, nil); err != nil {
				return err
			}
		}
		operations, cashless, err := accountOperations(ctx, q, account.ID)
		if err != nil {
			return err
		}
		ledger := newAccountLedger()
		ledger.cashless = cashless
		next := 0
		for i, day := range days {
			for next < len(operations) && !Day(operations[next].OccurredOn).After(day) {
				if err := applyOperation(&ledger, operations[next]); err != nil {
					return err
				}
				next++
			}
			total := values[i].Balances.Get(currency)
			for _, h := range holdings[account.ID] {
				var quote *AssetQuote
				if found, ok := latestQuoteOn(quotes[h.assetID], day); h.quoted && ok {
					quote = &found
				}
				position := Position{}
				if err := applyLedgerState(&position, ledger.positions[h.positionID], quote); err != nil {
					return err
				}
				total = total.Add(position.CurrentValue)
			}
			if countsCash {
				total = total.Add(ledger.cash)
			}
			if !total.IsZero() {
				values[i].Balances[currency] = total
			}
		}
	}
	return nil
}

func addSyncedValues(ctx context.Context, q Querier, days []time.Time, values []DayValues) error {
	rows, err := q.QueryContext(ctx, `SELECT fi.id, `+syncedHoldingColumns+` FROM financial_investments fi WHERE fi.balance IS NOT NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type synced struct {
		id      string
		holding SyncedHolding
	}
	var holdings []synced
	for rows.Next() {
		var s synced
		var row syncedHoldingRow
		if err := rows.Scan(append([]any{dbtypes.ID(&s.id)}, row.dests()...)...); err != nil {
			return err
		}
		if s.holding, err = row.holding(); err != nil {
			return err
		}
		holdings = append(holdings, s)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	cache := NewYieldCache()
	for _, s := range holdings {
		series, err := loadSyncedSeries(ctx, q, cache, s.id, s.holding, days[len(days)-1])
		if err != nil {
			return err
		}
		currency := s.holding.currency()
		for i, day := range days {
			value, ok := series.valueHeld(day)
			if !ok {
				values[i].Complete = false
				continue
			}
			values[i].Balances[currency] = values[i].Balances.Get(currency).Add(value)
		}
	}
	return nil
}
