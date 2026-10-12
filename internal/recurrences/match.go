package recurrences

import (
	"sort"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/rules"
	"github.com/greg0x46/julius/internal/transactions"
)

func matchCandidateFor(item transactions.Item) (rules.MatchCandidate, bool) {
	if item.OccurredAt == nil || item.EffectiveMoney == nil {
		return rules.MatchCandidate{}, false
	}
	amount, err := decimal.NewFromString(item.EffectiveMoney.Value)
	if err != nil {
		return rules.MatchCandidate{}, false
	}
	amount = amount.Abs()
	day := item.OccurredAt.Day()
	candidate := rules.MatchCandidate{
		Description:        item.Description,
		AccountName:        item.Account.Name,
		AccountInstitution: item.Account.Institution,
		Amount:             &amount,
		DayOfMonth:         &day,
	}
	if item.Card != nil && item.Card.Number != "" {
		candidate.CardNumber = &item.Card.Number
	}
	return candidate, true
}

// ResolveOccurrence decides whether an occurrence of the commitment behind
// conditions/logic has already been satisfied by a real transaction among
// candidates (expected to already be filtered by the caller to the
// commitment's category/account and the occurrence's month via
// transactions.Query). conditions/logic come from the automation rule
// linked to the commitment via a "reconcile" action
// (internal/automation.ListActiveReconcileTargets) — callers with no linked
// rule should not call this at all, since there's nothing to match against.
// Every condition is self-contained (amount and day_of_month carry their own
// reference value, see internal/rules), so the same conditions apply
// unchanged across every occurrence of the commitment. Candidates are tried
// in date order and the first one to match wins — the same "first match
// wins" rule automation.ApplyToNewTransaction uses.
func ResolveOccurrence(conditions []rules.Condition, logic rules.LogicOperator, candidates []transactions.Item) (matched *transactions.Item, ok bool) {
	sorted := make([]transactions.Item, len(candidates))
	copy(sorted, candidates)
	sort.SliceStable(sorted, func(i, j int) bool {
		ti, tj := sorted[i].OccurredAt, sorted[j].OccurredAt
		if ti == nil || tj == nil {
			return false
		}
		return ti.Before(*tj)
	})

	for i := range sorted {
		candidate, valid := matchCandidateFor(sorted[i])
		if !valid {
			continue
		}
		if rules.Matches(candidate, conditions, logic) {
			return &sorted[i], true
		}
	}
	return nil, false
}
