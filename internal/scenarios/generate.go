package scenarios

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// ErrInvalidInstallmentPlan is returned by GenerateInstallments when
// totalAmount, months, or cadence can't produce a sensible plan.
var ErrInvalidInstallmentPlan = errors.New("invalid installment plan")

// MaxInstallments bounds the work and memory of a generated plan.
const MaxInstallments = 10_000

var ErrTooManyInstallments = errors.New("too many installments")

// GeneratedInstallment is one row GenerateInstallments proposes — not yet
// persisted.
type GeneratedInstallment struct {
	Description string
	Amount      decimal.Decimal
	ProjectedAt time.Time
}

// Cadence is how often generated installments repeat.
type Cadence string

const (
	CadenceMonthly  Cadence = "mensal"
	CadenceWeekly   Cadence = "semanal"
	CadenceBiweekly Cadence = "quinzenal"
)

// IsValid reports whether c is one of the defined cadences.
func (c Cadence) IsValid() bool {
	switch c {
	case CadenceMonthly, CadenceWeekly, CadenceBiweekly:
		return true
	default:
		return false
	}
}

// step advances t by n cadence periods (n=0 returns t unchanged).
func (c Cadence) step(t time.Time, n int) time.Time {
	switch c {
	case CadenceWeekly:
		return t.AddDate(0, 0, 7*n)
	case CadenceBiweekly:
		return t.AddDate(0, 0, 14*n)
	default: // CadenceMonthly
		return t.AddDate(0, n, 0)
	}
}

// splitEvenly divides totalAmount into n shares at 2-decimal (BRL cents)
// precision: every share but the last is the floor of the even split, and
// the last absorbs whatever floor-division left over — so the shares always
// sum to exactly totalAmount. Shared by GenerateInstallments and the
// readjustment strategies (Readjust), which both need this "last absorbs
// the remainder" rule.
func splitEvenly(totalAmount decimal.Decimal, n int) []decimal.Decimal {
	wholeCents, _ := totalAmount.Shift(2).QuoRem(decimal.NewFromInt(int64(n)), 0)
	share := wholeCents.Shift(-2)
	shares := make([]decimal.Decimal, n)
	runningTotal := decimal.Zero
	for i := 0; i < n; i++ {
		amount := share
		if i == n-1 {
			amount = totalAmount.Sub(runningTotal)
		}
		shares[i] = amount
		runningTotal = runningTotal.Add(amount)
	}
	return shares
}

// GenerateInstallments splits totalAmount into count equal installments,
// spaced by cadence starting at startDate.
func GenerateInstallments(totalAmount decimal.Decimal, count int, startDate time.Time, cadence Cadence) ([]GeneratedInstallment, error) {
	if !totalAmount.IsPositive() {
		return nil, ErrInvalidInstallmentPlan
	}
	if count < 1 {
		return nil, ErrInvalidInstallmentPlan
	}
	if !cadence.IsValid() {
		return nil, ErrInvalidInstallmentPlan
	}
	if count > MaxInstallments {
		return nil, ErrTooManyInstallments
	}

	shares := splitEvenly(totalAmount, count)
	installments := make([]GeneratedInstallment, count)
	for i, amount := range shares {
		installments[i] = GeneratedInstallment{
			Description: fmt.Sprintf("Parcela %d/%d", i+1, count),
			Amount:      amount,
			ProjectedAt: cadence.step(startDate, i),
		}
	}
	return installments, nil
}

// splitByAmount divides totalAmount into installments of installmentAmount
// each — however many it takes to cover it (ceil(totalAmount /
// installmentAmount)) — with the last absorbing whatever floor-division
// left over, so the installments always sum to exactly totalAmount. Shared
// with the "abater do final" readjustment strategy (Readjust), which needs
// the same "fixed value, derived count" math.
func splitByAmount(totalAmount, installmentAmount decimal.Decimal) ([]decimal.Decimal, error) {
	quotient, remainder := totalAmount.QuoRem(installmentAmount, 0)
	limit := decimal.NewFromInt(MaxInstallments)
	if quotient.GreaterThan(limit) || (quotient.Equal(limit) && remainder.IsPositive()) {
		return nil, ErrTooManyInstallments
	}
	count := int(quotient.IntPart())
	if remainder.IsPositive() {
		count++
	}
	if count < 1 {
		count = 1
	}
	amounts := make([]decimal.Decimal, count)
	for i := 0; i < count-1; i++ {
		amounts[i] = installmentAmount
	}
	amounts[count-1] = totalAmount.Sub(installmentAmount.Mul(decimal.NewFromInt(int64(count - 1))))
	return amounts, nil
}

// GenerateInstallmentsForAmount splits totalAmount into installments worth
// installmentAmount each, spaced by cadence starting at startDate — the
// count is derived rather than chosen (see splitByAmount).
func GenerateInstallmentsForAmount(totalAmount, installmentAmount decimal.Decimal, startDate time.Time, cadence Cadence) ([]GeneratedInstallment, error) {
	if !totalAmount.IsPositive() || !installmentAmount.IsPositive() {
		return nil, ErrInvalidInstallmentPlan
	}
	if !cadence.IsValid() {
		return nil, ErrInvalidInstallmentPlan
	}

	amounts, err := splitByAmount(totalAmount, installmentAmount)
	if err != nil {
		return nil, err
	}
	count := len(amounts)
	installments := make([]GeneratedInstallment, count)
	for i, amount := range amounts {
		installments[i] = GeneratedInstallment{
			Description: fmt.Sprintf("Parcela %d/%d", i+1, count),
			Amount:      amount,
			ProjectedAt: cadence.step(startDate, i),
		}
	}
	return installments, nil
}

// CreateGeneratedInstallments persists every installment GenerateInstallments
// proposed for scenarioID as scenario_transactions, all inside one
// transaction so a mid-batch failure leaves no partial plan behind.
func CreateGeneratedInstallments(ctx context.Context, conn *sql.DB, scenarioID string, installments []GeneratedInstallment) ([]ScenarioTransaction, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	created := make([]ScenarioTransaction, len(installments))
	for i, ins := range installments {
		st, err := CreateScenarioTransaction(ctx, tx, scenarioID, ins.Description, ins.Amount, ins.ProjectedAt, nil)
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
