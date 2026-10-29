package recurrences

import (
	"time"

	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/rules"
	"github.com/greg0x46/julius/internal/transactions"
)

// OverrideState is what a user decided about one occurrence.
type OverrideState string

const (
	// StateLinked: the user picked this exact transaction for the occurrence.
	StateLinked OverrideState = "linked"
	// StateDetached: the user said this occurrence is NOT reconciled, and the
	// automation rule must stop claiming otherwise.
	StateDetached OverrideState = "detached"
)

// Override mirrors one reconciliation row of scenario_realizations: the only
// thing this feature persists. The reconciliation *state* is never stored —
// see Reconciliation.
type Override struct {
	ID             string
	ScenarioID     string
	OccurrenceDate time.Time
	State          OverrideState
	TransactionID  *string // set iff State == StateLinked
	// Origin is who wrote the row. A decision recorded through either write
	// path is "manual" by construction — the type exists so a reader can
	// report the stored origin instead of assuming one.
	Origin    Origin
	CreatedAt time.Time
}

// ManualLinkReachDays is how far from its occurrence a hand-picked
// transaction may sit. The automatic matcher is scoped to the occurrence's
// own calendar month (CandidatesByMonth), which is right for a pattern match
// but too tight for a human one: a bill due on the 1st is often paid on the
// last business day of the previous month.
//
// It is a domain fact, not a UI limit: the HTTP layer offers candidates
// within this reach, and every reader that needs a complete picture of the
// manual links touching a period — the Timeline included — has to widen its
// override lookup by the same margin, or a transaction linked just outside
// the period stays invisible to the rule-exclusion logic.
const ManualLinkReachDays = 45

// Origin says who decided a reconciliation that a read resolved.
type Origin string

const (
	OriginRule   Origin = "rule"
	OriginManual Origin = "manual"
)

// Reconciliation is the resolved status of one occurrence, recomputed on
// every read from the commitment's schedule, the user's overrides and the
// automation rule — never stored (principle 1 of docs/development/architecture.md).
//
// TransactionID is separate from Transaction on purpose: a manual link can
// point at a transaction outside whatever candidate window the caller
// loaded. The Timeline only needs the boolean, so it never pays to fetch the
// item; the HTTP layer batch-loads the ones it actually displays.
type Reconciliation struct {
	Occurrence    Occurrence
	TransactionID *string
	Transaction   *transactions.Item
	Origin        Origin // "" when unreconciled
	Detached      bool   // the user explicitly detached this occurrence
}

// Reconciled reports whether some real transaction already carries this
// occurrence's money — the question the Timeline asks before emitting a
// projected entry.
func (r Reconciliation) Reconciled() bool { return r.TransactionID != nil }

// Reconciler resolves the occurrences of ONE commitment. Build it once per
// commitment and resolve as many occurrences as needed: the per-commitment
// work (bucketing candidates by month, indexing overrides) happens once
// instead of per occurrence.
//
// It takes conditions/logic rather than an automation.Rule so this package
// stays independent of internal/automation — the same reason
// ResolveOccurrence has the signature it has.
type Reconciler struct {
	commitment RecurringCommitment
	conditions []rules.Condition
	logic      rules.LogicOperator
	hasRule    bool

	overrides  map[time.Time]Override
	byID       map[string]transactions.Item
	candidates map[time.Time][]transactions.Item
	// manuallyTaken holds every transaction id this commitment has a manual
	// link to, in any occurrence — see Resolve for why the rule must skip them.
	manuallyTaken map[string]bool
}

// NewReconciler indexes everything the resolution of a single occurrence
// needs. hasRule is false for a commitment with no automation rule targeting
// it, which is what lets a recurring commitment exist independent of any
// automation: with no rule and no override, nothing ever reconciles it.
func NewReconciler(
	commitment RecurringCommitment,
	conditions []rules.Condition,
	logic rules.LogicOperator,
	hasRule bool,
	overrides []Override,
	realCandidates []transactions.Item,
) Reconciler {
	r := Reconciler{
		commitment:    commitment,
		conditions:    conditions,
		logic:         logic,
		hasRule:       hasRule,
		overrides:     make(map[time.Time]Override, len(overrides)),
		byID:          make(map[string]transactions.Item, len(realCandidates)),
		manuallyTaken: map[string]bool{},
	}
	for _, override := range overrides {
		r.overrides[dates.Day(override.OccurrenceDate)] = override
		if override.State == StateLinked && override.TransactionID != nil {
			r.manuallyTaken[*override.TransactionID] = true
		}
	}
	for _, item := range realCandidates {
		r.byID[item.ID] = item
	}
	if hasRule {
		r.candidates = CandidatesByMonth(commitment, realCandidates)
	}
	return r
}

// Resolve decides the status of one occurrence. The order is the whole
// feature:
//
//  1. a "detached" override wins over everything — the user said this
//     occurrence is not reconciled, and the rule does not get to re-claim it
//     on the next read (which is exactly what would happen if detaching only
//     deleted a row, since the rule is re-evaluated from scratch every time);
//  2. a "linked" override wins over the rule — an explicit pick beats a
//     pattern match;
//  3. otherwise the automation rule matches against the occurrence's own
//     month, as it did before manual reconciliation existed.
//
// Step 3 skips transactions this commitment already has a manual link to.
// Without that, a transaction hand-linked to March's occurrence would still
// be matched by the rule for April's, and the same money would suppress two
// projections.
func (r Reconciler) Resolve(occurrence Occurrence) Reconciliation {
	day := dates.Day(occurrence.Date)
	result := Reconciliation{Occurrence: occurrence}

	if override, ok := r.overrides[day]; ok {
		if override.State == StateDetached {
			result.Detached = true
			return result
		}
		if override.TransactionID != nil {
			result.TransactionID = override.TransactionID
			result.Origin = OriginManual
			if item, ok := r.byID[*override.TransactionID]; ok {
				result.Transaction = &item
			}
			return result
		}
	}

	if !r.hasRule {
		return result
	}
	inMonth := r.candidates[MonthKey(occurrence.Date)]
	if len(r.manuallyTaken) > 0 {
		available := make([]transactions.Item, 0, len(inMonth))
		for _, item := range inMonth {
			if !r.manuallyTaken[item.ID] {
				available = append(available, item)
			}
		}
		inMonth = available
	}
	matched, ok := ResolveOccurrence(r.conditions, r.logic, inMonth)
	if !ok {
		return result
	}
	id := matched.ID
	result.TransactionID = &id
	result.Transaction = matched
	result.Origin = OriginRule
	return result
}

// ResolveRange resolves every occurrence of the commitment in [from, to].
func (r Reconciler) ResolveRange(from, to time.Time) []Reconciliation {
	occurrences := r.commitment.Occurrences(from, to)
	resolved := make([]Reconciliation, 0, len(occurrences))
	for _, occurrence := range occurrences {
		resolved = append(resolved, r.Resolve(occurrence))
	}
	return resolved
}

// MonthKey buckets a date by its calendar month, year included — the scope
// ResolveOccurrence's contract requires. It carries the year so September
// 2027 never reconciles September 2026.
func MonthKey(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// CandidatesByMonth selects the real transactions a commitment could
// reconcile against — same account when it names one —
// bucketed by MonthKey, which is the scope ResolveOccurrence needs: its
// conditions (amount, day-of-month) are month-agnostic, so one month's
// transaction would otherwise reconcile every other month's occurrence.
//
// This is the *automatic* candidate set only. A manual pick deliberately
// can use another account or a nearby month (see EligibilityForReconciliation).
func CandidatesByMonth(commitment RecurringCommitment, realCandidates []transactions.Item) map[time.Time][]transactions.Item {
	byMonth := map[time.Time][]transactions.Item{}
	for _, item := range realCandidates {
		if item.OccurredAt == nil {
			continue
		}
		if commitment.AccountID != nil && item.Account.ID != *commitment.AccountID {
			continue
		}
		month := MonthKey(*item.OccurredAt)
		byMonth[month] = append(byMonth[month], item)
	}
	return byMonth
}
