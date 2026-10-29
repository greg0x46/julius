package scenarios

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/dates"
)

// PlanInstallment is one payable-plan installment that still represents
// future cash flow: its Amount already carries the plan's direction (see
// SignedAmount), and installments with any realization at all are left out,
// because the real transaction behind that realization already accounts for
// the money.
type PlanInstallment struct {
	ScenarioKind  Kind
	PayableID     string
	TransactionID string
	Description   string
	Category      *string
	ProjectedAt   time.Time
	Amount        decimal.Decimal
}

// ListPlanInstallments returns the unrealized installments of every
// payable-backed plan (KindDebtPlan/KindReceivablePlan) projected onto a
// calendar day within [from, to].
//
// It exists so a caller building a cash-flow view does not have to reach
// into internal/payables to learn a plan's direction: the link to a Payable
// stays encapsulated here, in Scenario.PayableID/Kind, which is what
// docs/development/architecture.md (Domain engines) describes.
//
// from/to are compared as calendar days; a caller passing a timestamp gets
// its day.
func ListPlanInstallments(ctx context.Context, q Querier, from, to time.Time) ([]PlanInstallment, error) {
	plans, err := listPayablePlanScenarios(ctx, q)
	if err != nil {
		return nil, err
	}

	// A payable-backed plan without a payable_id cannot say which direction
	// its cash flows, so it is dropped here rather than guessed at — the
	// schema's CHECK already requires one, making this defensive.
	backed := make([]Scenario, 0, len(plans))
	planIDs := make([]string, 0, len(plans))
	for _, plan := range plans {
		if plan.PayableID == nil {
			continue
		}
		backed = append(backed, plan)
		planIDs = append(planIDs, plan.ID)
	}
	installmentsByPlan, err := ListScenarioTransactionsFor(ctx, q, planIDs)
	if err != nil {
		return nil, err
	}

	// Only the installments inside the window can produce an entry, so only
	// those need their allocations loaded — and all of them across every
	// plan in one query, so the cost is three round trips whether the user
	// has one open plan or forty.
	from, to = dates.Day(from), dates.Day(to)
	type windowed struct {
		plan        Scenario
		installment ScenarioTransaction
		day         time.Time
	}
	var inWindow []windowed
	var ids []string
	for _, plan := range backed {
		for _, installment := range installmentsByPlan[plan.ID] {
			day := dates.Day(installment.ProjectedAt)
			if day.Before(from) || day.After(to) {
				continue
			}
			inWindow = append(inWindow, windowed{plan: plan, installment: installment, day: day})
			ids = append(ids, installment.ID)
		}
	}
	realizations, err := realizationsFor(ctx, q, ids)
	if err != nil {
		return nil, err
	}

	var out []PlanInstallment
	for _, w := range inWindow {
		if !sumAllocated(realizations[w.installment.ID]).IsZero() {
			continue
		}
		out = append(out, PlanInstallment{
			ScenarioKind:  w.plan.Kind,
			PayableID:     *w.plan.PayableID,
			TransactionID: w.installment.ID,
			Description:   w.installment.Description,
			Category:      w.installment.Category,
			ProjectedAt:   w.day,
			Amount:        SignedAmount(w.plan.Kind, w.installment.Amount),
		})
	}
	return out, nil
}
