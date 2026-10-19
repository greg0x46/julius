package investments

import (
	"context"
	"time"

	"github.com/greg0x46/julius/internal/dates"
	"github.com/shopspring/decimal"
)

// ContributionMonth separates external funding from trading and returns. Unknown
// provider funding and opening balances inside the month cannot establish a plan.
type ContributionMonth struct {
	OpeningValue decimal.Decimal
	NetPrincipal decimal.Decimal
	Available    bool
}

func ReadContributionMonth(ctx context.Context, q Querier, today time.Time) (ContributionMonth, error) {
	today = dates.Day(today)
	start := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	opening := start.AddDate(0, 0, -1)
	result := ContributionMonth{}
	accounts, err := ListAccounts(ctx, q)
	if err != nil {
		return result, err
	}
	for _, a := range accounts {
		if a.Kind != AccountKindManual || a.FinancialAccountID != nil || (a.CurrencyCode != nil && *a.CurrencyCode != "BRL") {
			return result, nil
		}
		book, err := replayAccountUntil(ctx, q, a.ID, &opening)
		if err != nil {
			return result, err
		}
		result.OpeningValue = result.OpeningValue.Add(book.cash)
	}
	portfolio, err := BuildPortfolio(ctx, q, PortfolioQuery{To: opening})
	if err != nil {
		return result, err
	}
	if portfolio.Totals.Partial() {
		return result, nil
	}
	result.OpeningValue = result.OpeningValue.Add(portfolio.Totals.EndValue)
	ops, err := ListOperations(ctx, q, OperationFilter{From: &start, To: &today})
	if err != nil {
		return result, err
	}
	for _, op := range ops {
		if op.Source != "manual" || op.Kind == OperationInitialBalance {
			return result, nil
		}
		switch op.Kind {
		case OperationDeposit:
			result.NetPrincipal = result.NetPrincipal.Add(op.Amount)
		case OperationWithdrawal:
			result.NetPrincipal = result.NetPrincipal.Sub(op.Amount)
		}
	}
	result.Available = true
	return result, nil
}
