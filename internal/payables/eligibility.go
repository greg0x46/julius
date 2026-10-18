package payables

import "github.com/greg0x46/julius/internal/money"

// LinkIneligibilityReason mirrors LinkIneligibilityReason.
type LinkIneligibilityReason string

const (
	ReasonIgnored        LinkIneligibilityReason = "ignored"
	ReasonNotOutflow     LinkIneligibilityReason = "not_outflow"
	ReasonNotInflow      LinkIneligibilityReason = "not_inflow"
	ReasonMissingBRLPair LinkIneligibilityReason = "missing_brl_pair"
	ReasonAlreadyLinked  LinkIneligibilityReason = "already_linked"
)

// LinkEligibility mirrors LinkEligibility.
type LinkEligibility struct {
	Eligible bool
	Reason   *LinkIneligibilityReason
}

// EligibilityForLink mirrors eligibility_for_link: only BRL transactions
// moving in the direction kind requires (outflow to pay down a debt, inflow
// to settle a receivable) that aren't ignored and aren't already linked to
// some other payable can be linked — payables are a BRL-only concept in
// this app (per the reference).
func EligibilityForLink(kind Kind, classification money.Classification, inclusionState money.InclusionState, eff *money.EffectiveMoney, alreadyLinked bool) LinkEligibility {
	reason := func(r LinkIneligibilityReason) LinkEligibility { return LinkEligibility{Eligible: false, Reason: &r} }

	wantClassification := money.Outflow
	if kind == KindReceivable {
		wantClassification = money.Inflow
	}

	if r := money.BRLLinkIneligibility(classification, wantClassification, inclusionState, eff); r != nil {
		return reason(LinkIneligibilityReason(*r))
	}
	if alreadyLinked {
		return reason(ReasonAlreadyLinked)
	}
	return LinkEligibility{Eligible: true}
}
