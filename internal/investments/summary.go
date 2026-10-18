package investments

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/exchange"
	"github.com/greg0x46/julius/internal/valuation"
)

// BuildSummary is the single reading of the investment book. TotalValue
// includes provider holdings and the effective custody cash once. Net worth
// uses ManualNetWorth instead because linked cash is already bank cash there.
//
// Holdings and cash in other currencies are valued in BRL at today's rate and
// stay native in their own records; a currency without a rate fails the
// summary instead of being left out of it.
func BuildSummary(ctx context.Context, q Querier) (Summary, error) {
	return BuildSummaryValued(ctx, q, valuation.New(exchange.New(q, nil), time.Now()))
}

func BuildSummaryValued(ctx context.Context, q Querier, valued *valuation.Valuation) (Summary, error) {
	accounts, err := ListAccounts(ctx, q)
	if err != nil {
		return Summary{}, err
	}
	positions, err := ListPositions(ctx, q, PositionFilter{IncludeClosed: true})
	if err != nil {
		return Summary{}, err
	}

	summary := Summary{Valuation: valued.Metadata, CurrencyCode: valuation.BaseCurrency, Accounts: []SummaryAccount{}}
	valueByAccount := map[string]decimal.Decimal{}
	for _, position := range positions {
		currency := valuation.Currency(position.CurrencyCode)
		value, err := valued.Convert(ctx, position.CurrentValue, currency)
		if err != nil {
			return Summary{}, err
		}
		if position.TotalCost != nil {
			cost, err := valued.Convert(ctx, *position.TotalCost, currency)
			if err != nil {
				return Summary{}, err
			}
			position.TotalCost = &cost
		}
		position.CurrentValue = value
		valueByAccount[position.AccountID] = valueByAccount[position.AccountID].Add(position.CurrentValue)
		if position.Source == PositionSourceManual {
			summary.ManualValue = summary.ManualValue.Add(position.CurrentValue)
		} else {
			summary.SyncedValue = summary.SyncedValue.Add(position.CurrentValue)
		}
		// A holding with no known cost basis reports no gain rather than
		// treating its whole value as profit.
		if position.TotalCost != nil {
			summary.UnrealizedGain = summary.UnrealizedGain.Add(position.CurrentValue.Sub(*position.TotalCost))
		}
	}

	summary.TotalValue = summary.ManualValue.Add(summary.SyncedValue)
	for _, account := range accounts {
		cash, err := valued.Convert(ctx, accountEffectiveCash(account), valuation.Currency(account.CurrencyCode))
		if err != nil {
			return Summary{}, err
		}
		summary.CashBalance = summary.CashBalance.Add(cash)
		summary.TotalValue = summary.TotalValue.Add(cash)
		summary.Accounts = append(summary.Accounts, SummaryAccount{
			AccountID:    account.ID,
			Name:         account.Name,
			Kind:         account.Kind,
			CurrentValue: valueByAccount[account.ID],
			CashBalance:  cash,
		})
	}

	return summary, nil
}
