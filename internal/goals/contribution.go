package goals

import (
	"context"
	"time"

	"github.com/greg0x46/julius/internal/investments"
	"github.com/shopspring/decimal"
)

// MonthlyContribution uses the latest goal terms against a month-opening plan.
// Nil figures disclose unavailable funding/history rather than an assumed zero.
type MonthlyContribution struct {
	Month                             string
	Target, NetContributed, Remaining *decimal.Decimal
	AssumesZeroReturn                 bool
}

func nextContributionGoal(list []Goal) int {
	best := -1
	for i, g := range list {
		if g.Projection == nil || g.CurrentValue.GreaterThanOrEqual(g.TargetAmount) {
			continue
		}
		if best < 0 {
			best = i
			continue
		}
		b := list[best]
		if g.TargetDate.Before(*b.TargetDate) || (g.TargetDate.Equal(*b.TargetDate) &&
			(g.TargetAmount.LessThan(b.TargetAmount) || (g.TargetAmount.Equal(b.TargetAmount) && g.ID < b.ID))) {
			best = i
		}
	}
	return best
}

func withMonthlyContribution(ctx context.Context, q Querier, list []Goal, today time.Time) error {
	i := nextContributionGoal(list)
	if i < 0 {
		return nil
	}
	m := &MonthlyContribution{Month: today.Format("2006-01")}
	list[i].MonthlyContribution = m
	funding, err := investments.ReadContributionMonth(ctx, q, today)
	if err != nil {
		return err
	}
	if !funding.Available {
		return nil
	}
	start := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	est, err := estimateReturn(ctx, q, start.AddDate(0, 0, -1))
	if err != nil {
		return err
	}
	if est.Partial {
		return nil
	}
	// Apply the existing partial-month horizon from month start. Target edits
	// apply to the whole current month, without proration.
	targetDate := list[i].TargetDate
	months := monthsUntil(start, *targetDate)
	if months > maxProjectionMonths {
		return nil
	}
	p := project(funding.OpeningValue, list[i].TargetAmount, est.Monthly, months)
	target := p.RequiredMonthlyContribution
	remaining := decimal.Max(decimal.Zero, target.Sub(funding.NetPrincipal))
	m.Target, m.NetContributed, m.Remaining = &target, &funding.NetPrincipal, &remaining
	m.AssumesZeroReturn = est.Monthly == nil
	return nil
}
