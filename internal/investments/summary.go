package investments

import (
	"context"
	"github.com/greg0x46/julius/internal/fx"
	"time"

	"github.com/shopspring/decimal"
)

const unassignedPortfolioName = "Sem objetivo"

// BuildSummary is the single reading of the investment book. TotalValue
// includes provider holdings and the effective custody cash once. Net worth
// uses ManualNetWorth instead because linked cash is already bank cash there.
// Goals only regroup value that positions already carry.
func BuildSummary(ctx context.Context, q Querier) (Summary, error) {
	return BuildSummaryValued(ctx, q, fx.New(time.Now()))
}

func BuildSummaryValued(ctx context.Context, q Querier, valuation *fx.Valuation) (Summary, error) {
	accounts, err := ListAccounts(ctx, q)
	if err != nil {
		return Summary{}, err
	}
	positions, err := ListPositions(ctx, q, PositionFilter{IncludeClosed: true})
	if err != nil {
		return Summary{}, err
	}
	portfolios, err := listRawPortfolios(ctx, q)
	if err != nil {
		return Summary{}, err
	}

	summary := Summary{Valuation: valuation.Metadata, CurrencyCode: "BRL", Portfolios: []SummaryPortfolio{}, Accounts: []SummaryAccount{}}
	valueByAccount := map[string]decimal.Decimal{}
	unassigned := decimal.Zero
	hasUnassigned := false
	valuedPositions := make([]Position, 0, len(positions))
	for _, position := range positions {
		value, err := valuation.Convert(ctx, q, position.CurrentValue, fx.Currency(position.CurrencyCode))
		if err != nil {
			return Summary{}, err
		}
		position.CurrentValue = value
		if position.TotalCost != nil {
			cost, err := valuation.Convert(ctx, q, *position.TotalCost, fx.Currency(position.CurrencyCode))
			if err != nil {
				return Summary{}, err
			}
			position.TotalCost = &cost
		}
		position.CurrencyCode = nil
		valuedPositions = append(valuedPositions, position)
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
		if position.PortfolioID == nil {
			unassigned = unassigned.Add(position.CurrentValue)
			hasUnassigned = true
		}
	}

	summary.TotalValue = summary.ManualValue.Add(summary.SyncedValue)
	for _, account := range accounts {
		cash, err := valuation.Convert(ctx, q, accountEffectiveCash(account), fx.Currency(account.CurrencyCode))
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

	values := portfolioValues(valuedPositions)
	for _, portfolio := range portfolios {
		id := portfolio.ID
		summary.Portfolios = append(summary.Portfolios, SummaryPortfolio{
			PortfolioID:  &id,
			Name:         portfolio.Name,
			CurrentValue: values[id],
			TargetAmount: portfolio.TargetAmount,
			Progress:     progress(values[id], portfolio.TargetAmount),
		})
	}
	if hasUnassigned {
		summary.Portfolios = append(summary.Portfolios, SummaryPortfolio{
			Name:         unassignedPortfolioName,
			CurrentValue: unassigned,
		})
	}
	return summary, nil
}
