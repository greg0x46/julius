package scenarios

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// ReadjustStrategy selects how Readjust regenerates unallocated installments.
type ReadjustStrategy string

const (
	// StrategyReduceTerm ("Abater do final") keeps the installment value
	// (the last affected installment's amount) and lets the number of
	// installments — and so the payoff date — move: fewer if the plan is
	// ahead, more if it's behind.
	StrategyReduceTerm ReadjustStrategy = "abater_do_final"
	// StrategyRedistribute ("Redistribuir entre os meses") keeps the same
	// number of installments (and so the same payoff date) and lets the
	// per-installment value move up or down to absorb the deviation.
	StrategyRedistribute ReadjustStrategy = "redistribuir"
)

// ErrNoAffectedInstallments is returned by Readjust when every installment
// already has some allocation (realizedTotal > 0) — there is nothing left
// for a readjustment to replace.
var ErrNoAffectedInstallments = errors.New("no unallocated installments to readjust")

// ErrNothingToRedistribute is returned by Readjust when the balance left to
// redistribute (remainingAmount minus what's already reserved by partially
// realized installments) isn't positive — regenerating installments for a
// zero or negative balance would only produce zero or negative amounts, so
// Readjust refuses rather than doing that.
var ErrNothingToRedistribute = errors.New("nothing left to redistribute")

// ErrInvalidStrategy is returned by Readjust for any strategy value other
// than the two defined constants.
var ErrInvalidStrategy = errors.New("invalid readjustment strategy")

// Readjust mirrors the spec's "Reajustar parcelas restantes": it replaces
// every installment with realizedTotal = 0 (no allocation at all, whether
// "projetada" or "atrasada") with a fresh set that sums to
// remainingAmount − Σ(amount − realizedTotal) of the installments left
// untouched — every installment with *any* allocation, even partial, is
// preserved exactly as-is and never touched here. remainingAmount is the
// caller-supplied remaining amount for the scenario's payable (see
// payables.Summarize); this package has no dependency on internal/payables,
// so it's passed in rather than recomputed here.
func Readjust(ctx context.Context, conn *sql.DB, scenarioID string, remainingAmount decimal.Decimal, strategy ReadjustStrategy) ([]ScenarioTransaction, error) {
	if strategy != StrategyReduceTerm && strategy != StrategyRedistribute {
		return nil, ErrInvalidStrategy
	}

	list, err := ListScenarioTransactions(ctx, conn, scenarioID)
	if err != nil {
		return nil, err
	}

	ids := make([]string, len(list))
	for i, st := range list {
		ids[i] = st.ID
	}
	// One load of the whole scenario's allocations decides both which
	// installments are replaceable and how much the kept ones reserve.
	realizations, err := realizationsFor(ctx, conn, ids)
	if err != nil {
		return nil, err
	}

	var affected []ScenarioTransaction
	reserved := decimal.Zero
	for _, st := range list {
		realized := sumAllocated(realizations[st.ID])
		if realized.IsZero() {
			affected = append(affected, st)
		} else if shortfall := st.Amount.Sub(realized); shortfall.IsPositive() {
			// Only an unpaid shortfall needs to be reserved so it isn't
			// double-counted against remainingAmount. An overpaid kept
			// installment (shortfall negative) must NOT reduce reserved
			// below zero — remainingAmount already gives full credit for
			// the overpayment, so letting a negative shortfall inflate
			// balance here would double-credit it and cancel out the very
			// abatement the user is asking for.
			reserved = reserved.Add(shortfall)
		}
	}
	if len(affected) == 0 {
		return nil, ErrNoAffectedInstallments
	}

	balance := remainingAmount.Sub(reserved)
	if !balance.IsPositive() {
		return nil, ErrNothingToRedistribute
	}

	sort.Slice(affected, func(i, j int) bool { return affected[i].ProjectedAt.Before(affected[j].ProjectedAt) })

	var drafts []GeneratedInstallment
	switch strategy {
	case StrategyReduceTerm:
		drafts = reduceTermInstallments(balance, affected, inferCadence(list))
	case StrategyRedistribute:
		drafts = redistributeInstallments(balance, affected)
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	for _, st := range affected {
		if err := DeleteScenarioTransaction(ctx, tx, st.ID); err != nil {
			return nil, err
		}
	}
	created := make([]ScenarioTransaction, len(drafts))
	for i, d := range drafts {
		st, err := CreateScenarioTransaction(ctx, tx, scenarioID, d.Description, d.Amount, d.ProjectedAt, nil)
		if err != nil {
			return nil, err
		}
		created[i] = st
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

// reduceTermInstallments implements "abater do final": every new
// installment is worth referenceAmount except the last, which absorbs
// whatever floor-division left over; the count is however many of those it
// takes to cover balance, starting from the earliest affected installment's
// date. referenceAmount is the max amount among affected rather than simply
// the last-dated one: splitByAmount always makes its last (remainder)
// installment <= the regular amount, so if a previous readjustment's
// remainder installment is still around, picking "last by date" can lock
// onto that tiny remainder and blow the count up (e.g. 24 -> 95) instead of
// the plan's actual installment value.
func reduceTermInstallments(balance decimal.Decimal, affected []ScenarioTransaction, cadence Cadence) []GeneratedInstallment {
	referenceAmount := affected[0].Amount
	for _, st := range affected[1:] {
		if st.Amount.GreaterThan(referenceAmount) {
			referenceAmount = st.Amount
		}
	}
	startDate := affected[0].ProjectedAt

	amounts := splitByAmount(balance, referenceAmount)
	n := len(amounts)
	installments := make([]GeneratedInstallment, n)
	for i, amount := range amounts {
		installments[i] = GeneratedInstallment{
			Description: fmt.Sprintf("Parcela %d/%d (reajustada)", i+1, n),
			Amount:      amount,
			ProjectedAt: cadence.step(startDate, i),
		}
	}
	return installments
}

// inferCadence recovers a plan's cadence from the gaps between its
// installments, since it is not stored: the most common gap decides (up to 7
// days weekly, up to 14 biweekly, otherwise monthly) and ties, or fewer than
// two installments, fall back to monthly.
func inferCadence(installments []ScenarioTransaction) Cadence {
	days := make([]time.Time, len(installments))
	for i, st := range installments {
		days[i] = st.ProjectedAt
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })
	var weekly, biweekly, monthly int
	for i := 1; i < len(days); i++ {
		switch gap := int(days[i].Sub(days[i-1]).Hours() / 24); {
		case gap <= 0:
		case gap <= 7:
			weekly++
		case gap <= 14:
			biweekly++
		default:
			monthly++
		}
	}
	switch {
	case weekly > biweekly && weekly > monthly:
		return CadenceWeekly
	case biweekly > weekly && biweekly > monthly:
		return CadenceBiweekly
	default:
		return CadenceMonthly
	}
}

// redistributeInstallments implements "redistribuir entre os meses": the
// same dates the affected installments already had are kept (so the payoff
// date doesn't move), and balance is split evenly across that same count,
// the last installment absorbing the remainder.
func redistributeInstallments(balance decimal.Decimal, affected []ScenarioTransaction) []GeneratedInstallment {
	n := len(affected)
	amounts := splitEvenly(balance, n)
	installments := make([]GeneratedInstallment, n)
	for i, amount := range amounts {
		installments[i] = GeneratedInstallment{
			Description: fmt.Sprintf("Parcela %d/%d (reajustada)", i+1, n),
			Amount:      amount,
			ProjectedAt: affected[i].ProjectedAt,
		}
	}
	return installments
}
