package goals

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/investments"
)

const (
	// returnWindowMonths is the trailing window the average monthly return is
	// measured over.
	returnWindowMonths = 12
	// minReturnDays is the shortest measured history that is extrapolated to a
	// monthly rate; less is noise.
	minReturnDays = 30
	// maxProjectionMonths bounds the horizon of a projection.
	maxProjectionMonths = 1200
	// avgMonthDays is a mean calendar month (365.25 / 12).
	avgMonthDays = "30.4375"
	// rateScale keeps compounding exact enough without growing unboundedly.
	rateScale = 12
)

// Status says where the projection leaves the goal at its target date.
type Status string

const (
	// StatusReached: the metric is already at or above the target.
	StatusReached Status = "reached"
	// StatusOnTrack: appreciation alone carries the metric to the target.
	StatusOnTrack Status = "on_track"
	// StatusBehind: a monthly contribution is needed on top of appreciation.
	StatusBehind Status = "behind"
)

// Projection is the goal at its target date. It is computed on read and never
// stored. Appreciation is the portfolio's average monthly return over a
// trailing window; ProjectedValue assumes no new contributions, and
// RequiredMonthlyContribution is the end-of-month deposit that closes the gap.
type Projection struct {
	Status          Status
	MonthsRemaining int
	// MonthlyReturn is the compounded average monthly return (0.01 = 1%),
	// negative for a depreciation. Nil when the history is too short to state
	// one; the projection then assumes no appreciation.
	MonthlyReturn *decimal.Decimal
	// ReturnFrom is the first day of the history MonthlyReturn was measured
	// over; nil together with MonthlyReturn. ReturnPartial says some position
	// could not be measured and was left out.
	ReturnFrom    *time.Time
	ReturnPartial bool

	ProjectedValue              decimal.Decimal
	ProjectedProgress           decimal.Decimal
	RequiredMonthlyContribution decimal.Decimal
}

// ReturnEstimate is the average monthly return of the investments.
type ReturnEstimate struct {
	Monthly *decimal.Decimal
	From    *time.Time
	Partial bool
}

// estimateReturn measures the investments over the trailing window ending
// today with the period view, the authority on what a position earned.
func estimateReturn(ctx context.Context, q Querier, today time.Time) (ReturnEstimate, error) {
	from := dates.AddMonths(today, -returnWindowMonths)
	portfolio, err := investments.BuildPortfolio(ctx, q, investments.PortfolioQuery{From: &from, To: today})
	if err != nil {
		return ReturnEstimate{}, err
	}
	est := ReturnEstimate{Partial: portfolio.Totals.Partial()}
	if portfolio.Totals.YieldRate == nil {
		return est, nil
	}
	// The Dietz rate averages capital over the whole requested window; when no
	// position held capital from its start, rescale it to the history actually
	// held (Dietz over [start, today]).
	opening := from.AddDate(0, 0, -1)
	start := today
	for _, group := range portfolio.Groups {
		for _, p := range group.Positions {
			if since, ok := p.CapitalSince(); ok && p.UnavailableReason == "" && since.Before(start) {
				start = since
			}
		}
	}
	windowDays, days := daysBetween(opening, today), daysBetween(start, today)
	if days < minReturnDays || windowDays <= 0 {
		return est, nil
	}
	period := portfolio.Totals.YieldRate.Mul(decimal.NewFromInt(int64(days))).DivRound(decimal.NewFromInt(int64(windowDays)), rateScale)
	if monthly, ok := monthlyRate(period, days); ok {
		est.Monthly, est.From = &monthly, &start
	}
	return est, nil
}

func daysBetween(from, to time.Time) int { return int(to.Sub(from) / (24 * time.Hour)) }

// monthlyRate converts a return over `days` days to its compounded monthly
// equivalent. It is not defined for a total loss or worse.
func monthlyRate(period decimal.Decimal, days int) (decimal.Decimal, bool) {
	growth := period.Add(decimal.NewFromInt(1))
	if days <= 0 || !growth.IsPositive() {
		return decimal.Zero, false
	}
	exponent := decimal.RequireFromString(avgMonthDays).DivRound(decimal.NewFromInt(int64(days)), rateScale)
	monthly, err := growth.PowWithPrecision(exponent, rateScale)
	if err != nil {
		return decimal.Zero, false
	}
	return monthly.Sub(decimal.NewFromInt(1)).Round(rateScale), true
}

// monthsUntil is the number of whole months from today that reaches the
// target day, counting a partial last month as a month. Zero when the target
// is not after today.
func monthsUntil(today, target time.Time) int {
	n := 0
	for dates.AddMonths(today, n).Before(target) {
		n++
		if n > maxProjectionMonths {
			return n
		}
	}
	return n
}

// project compounds current at the monthly rate (nil is no appreciation) for
// `months` months and solves the end-of-month contribution that reaches the
// target. months must be positive.
func project(current, target decimal.Decimal, monthly *decimal.Decimal, months int) Projection {
	rate := decimal.Zero
	if monthly != nil {
		rate = *monthly
	}
	growth := decimal.NewFromInt(1)
	step := rate.Add(growth)
	for i := 0; i < months; i++ {
		growth = growth.Mul(step).Round(rateScale)
	}
	projected := current.Mul(growth)
	p := Projection{
		MonthsRemaining:   months,
		MonthlyReturn:     monthly,
		ProjectedValue:    projected,
		ProjectedProgress: projected.Div(target),
	}
	switch {
	case current.GreaterThanOrEqual(target):
		p.Status = StatusReached
		return p
	case projected.GreaterThanOrEqual(target):
		p.Status = StatusOnTrack
		return p
	}
	p.Status = StatusBehind
	gap := target.Sub(projected)
	annuity := growth.Sub(decimal.NewFromInt(1))
	if rate.IsZero() || annuity.IsZero() {
		p.RequiredMonthlyContribution = gap.DivRound(decimal.NewFromInt(int64(months)), 10)
	} else {
		p.RequiredMonthlyContribution = gap.Mul(rate).DivRound(annuity, 10)
	}
	return p
}

// projectGoal builds the projection of a goal with a future target date, or
// nil when there is nothing to project.
func projectGoal(g Goal, est ReturnEstimate, today time.Time) *Projection {
	if g.TargetDate == nil {
		return nil
	}
	months := monthsUntil(today, dates.Day(*g.TargetDate))
	if months == 0 || months > maxProjectionMonths {
		return nil
	}
	p := project(g.CurrentValue, g.TargetAmount, est.Monthly, months)
	p.ReturnFrom, p.ReturnPartial = est.From, est.Partial
	return &p
}

// needsProjection says whether any goal has a target date to project to.
func needsProjection(list []Goal) bool {
	for _, g := range list {
		if g.TargetDate != nil {
			return true
		}
	}
	return false
}
