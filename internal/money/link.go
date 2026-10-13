package money

// LinkIneligibilityReason describes the common BRL planning-link checks.
type LinkIneligibilityReason string

const (
	LinkReasonIgnored        LinkIneligibilityReason = "ignored"
	LinkReasonNotOutflow     LinkIneligibilityReason = "not_outflow"
	LinkReasonNotInflow      LinkIneligibilityReason = "not_inflow"
	LinkReasonMissingBRLPair LinkIneligibilityReason = "missing_brl_pair"
)

// BRLLinkIneligibility returns nil when a transaction can carry a BRL
// obligation in the required direction. Consumers check existing links next.
// Unlike totals eligibility, status, category and amount do not gate a link.
func BRLLinkIneligibility(classification, required Classification, inclusion InclusionState, effective *EffectiveMoney) *LinkIneligibilityReason {
	reason := func(r LinkIneligibilityReason) *LinkIneligibilityReason { return &r }
	if inclusion == Ignored {
		return reason(LinkReasonIgnored)
	}
	if classification != required {
		if required == Inflow {
			return reason(LinkReasonNotInflow)
		}
		return reason(LinkReasonNotOutflow)
	}
	if effective == nil || effective.CurrencyCode != "BRL" {
		return reason(LinkReasonMissingBRLPair)
	}
	return nil
}
