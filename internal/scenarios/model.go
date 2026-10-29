// Package scenarios implements hypothetical, user-authored projections
// layered on top of real data — see
// docs/development/contexts/scenarios.md. A Scenario groups a set of
// ScenarioTransaction "planned installments" attached to a
// payables.Payable. Nothing here changes how internal/payables computes
// settled/remaining amounts — a scenario is a read-layer projection, never
// a source of real totals, unless a caller explicitly opts in (see the
// transactions package's scenario_id union).
package scenarios

import (
	"time"

	"github.com/shopspring/decimal"
)

// Kind is the scenario's flavor. Recurring cash flows are scenarios too; the
// recurring schedule lives in scenario_recurring_schedules rather than on
// this row.
type Kind string

const (
	KindDebtPlan       Kind = "debt_plan"
	KindReceivablePlan Kind = "receivable_plan"
	KindStandalone     Kind = "standalone"
	KindRecurring      Kind = "recurring"
)

// Scenario mirrors the scenarios table. IsActive controls inclusion in the
// default projection read. IsAccountingSource marks the one payable-backed
// scenario whose real settlements define the payable's official Settled
// amount; simulation scenarios may point at the same payable without that
// flag.
type Scenario struct {
	ID                 string
	Kind               Kind
	Name               string
	PayableID          *string
	IsActive           bool
	IsAccountingSource bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ScenarioTransaction mirrors the scenario_transactions table: a single
// planned/projected installment, with a planned Amount and ProjectedAt date
// — never a real financial_transactions row.
type ScenarioTransaction struct {
	ID          string
	ScenarioID  string
	Description string
	Amount      decimal.Decimal
	ProjectedAt time.Time
	Category    *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ScenarioTransactionRealization is the installment-allocation view of an
// allocation row in scenario_realizations: (part of) a real
// payable_transaction_links row allocated to a planned installment.
// PayableLinkID is derived from the realized transaction on read, so it is
// nil for a standalone scenario, which has no payable to link against.
type ScenarioTransactionRealization struct {
	ID                    string
	ScenarioTransactionID string
	PayableLinkID         *string
	AllocatedAmount       decimal.Decimal
	CreatedAt             time.Time
}

// Status is a ScenarioTransaction's recomputed-on-read state — never
// persisted, mirroring payables.Status's pattern.
type Status string

const (
	StatusAtrasada         Status = "atrasada"
	StatusProjetada        Status = "projetada"
	StatusPagaParcialmente Status = "paga_parcialmente"
	StatusPaga             Status = "paga"
	StatusPagaAMais        Status = "paga_a_mais"
)

// Status computes the installment's status from today's date and
// realizedTotal (the sum of the allocated_amount of every allocation
// realization on it) — both supplied by the caller, never read here, so this
// stays a pure function callable from tests without a database.
//
// Allocations are positive magnitudes, so the planned amount is compared by
// magnitude too: a standalone expense is authored negative.
func (s ScenarioTransaction) Status(today time.Time, realizedTotal decimal.Decimal) Status {
	planned := s.Amount.Abs()
	switch {
	case realizedTotal.IsZero() && today.After(s.ProjectedAt):
		return StatusAtrasada
	case realizedTotal.IsZero():
		return StatusProjetada
	case realizedTotal.LessThan(planned):
		return StatusPagaParcialmente
	case realizedTotal.Equal(planned):
		return StatusPaga
	default: // realizedTotal > planned
		return StatusPagaAMais
	}
}

// SignedAmount applies a plan's cash-flow direction to a planned amount:
// a debt plan's installment is money leaving (negated), a receivable plan's
// is money arriving (kept as authored). A standalone scenario's amount is
// already authored with its own sign and is returned untouched.
//
// This is what lets a caller project a plan installment onto a cash-flow
// timeline without consulting the backing payables.Payable: Scenario.Kind
// is set from the payable's Kind at creation and the two never diverge, so
// the direction is derivable from the scenario alone.
//
// Note: this negates rather than forcing a sign, preserving the behavior
// this rule had while it lived in internal/timeline. A plan installment
// stored with a negative Amount (nothing rejects one today) therefore reads
// as an inflow on a debt plan — recorded as open note 5 in
// docs/development/architecture.md rather than "fixed" here, since deciding it
// either way changes real totals.
func SignedAmount(kind Kind, amount decimal.Decimal) decimal.Decimal {
	switch kind {
	case KindDebtPlan:
		return amount.Neg()
	default:
		// KindReceivablePlan is already an inflow; KindStandalone carries
		// its own sign.
		return amount
	}
}
