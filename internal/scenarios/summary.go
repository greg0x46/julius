package scenarios

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// TransactionSummary is one planned installment together with everything
// derived from its realizations — none of it stored on the
// scenario_transactions row itself.
type TransactionSummary struct {
	Transaction   ScenarioTransaction
	Realizations  []ScenarioTransactionRealization
	RealizedTotal decimal.Decimal
	Status        Status
}

// Summary is a whole scenario's derived state.
//
// AccumulatedDeviation is Σ |amount| − Σ realized across every installment
// whose ProjectedAt is on or before today. A positive value means the plan
// is behind (less was actually allocated than planned so far); negative
// means ahead. Installments not yet due don't count toward it: only what
// should already have happened says whether the plan is on track.
type Summary struct {
	Transactions         []TransactionSummary
	AccumulatedDeviation decimal.Decimal
}

// Summarize derives a scenario's installment statuses and accumulated
// deviation. It is the one place that recomputation lives, so the HTTP
// layer only formats what it returns (see docs/development/architecture.md,
// Cross-cutting principles).
func Summarize(ctx context.Context, q Querier, scenarioID string, today time.Time) (Summary, error) {
	list, err := ListScenarioTransactions(ctx, q, scenarioID)
	if err != nil {
		return Summary{}, err
	}
	ids := make([]string, len(list))
	for i, st := range list {
		ids[i] = st.ID
	}
	// One query for the whole scenario's allocations, not one per
	// installment: a 48-parcel plan is 2 round trips, not 49.
	byTransaction, err := realizationsFor(ctx, q, ids)
	if err != nil {
		return Summary{}, err
	}

	out := Summary{
		Transactions:         make([]TransactionSummary, len(list)),
		AccumulatedDeviation: decimal.Zero,
	}
	for i, st := range list {
		summary := summarizeTransaction(st, byTransaction[st.ID], today)
		out.Transactions[i] = summary
		if !st.ProjectedAt.After(today) {
			out.AccumulatedDeviation = out.AccumulatedDeviation.Add(st.Amount.Abs().Sub(summary.RealizedTotal))
		}
	}
	return out, nil
}

// SummarizeFresh is SummarizeTransaction for an installment that was just
// created: it cannot have realizations yet, so its derived state is known
// without querying for them.
func SummarizeFresh(st ScenarioTransaction, today time.Time) TransactionSummary {
	return summarizeTransaction(st, nil, today)
}

// SummarizeTransaction derives a single installment's realizations, their
// total, and the status they imply — the per-installment half of Summarize,
// exposed for callers holding one installment rather than a whole scenario.
func SummarizeTransaction(ctx context.Context, q Querier, st ScenarioTransaction, today time.Time) (TransactionSummary, error) {
	realizations, err := listRealizationsForTransaction(ctx, q, st.ID)
	if err != nil {
		return TransactionSummary{}, err
	}
	return summarizeTransaction(st, realizations, today), nil
}

// summarizeTransaction is the pure derivation every path above shares:
// given an installment and the allocations already read for it, the total
// and the status they imply. Keeping it free of ctx/Querier is what lets
// Summarize derive a whole scenario from one batched load and SummarizeFresh
// skip the database entirely, without either growing its own copy of the
// rule.
func summarizeTransaction(st ScenarioTransaction, realizations []ScenarioTransactionRealization, today time.Time) TransactionSummary {
	total := sumAllocated(realizations)
	return TransactionSummary{
		Transaction:   st,
		Realizations:  realizations,
		RealizedTotal: total,
		Status:        st.Status(today, total),
	}
}
