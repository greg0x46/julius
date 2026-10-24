// Package money ports the pure decision rules from the Python reference's
// app/transactions/rules.py: classifying a transaction's movement direction,
// picking which of the two amount/currency pairs a transaction carries is the
// "effective" one, and deciding whether a transaction counts toward totals.
// None of this touches the database — it exists so the same rules can be
// unit-tested in isolation and reused by the query engine, debt linking, and
// the sync pipeline alike.
package money

import (
	"github.com/shopspring/decimal"
)

// Classification is the direction of a transaction's movement.
type Classification string

const (
	Inflow       Classification = "inflow"
	Outflow      Classification = "outflow"
	Unclassified Classification = "unclassified"
)

// Classify mirrors rules.classify: only CREDIT/DEBIT movement types are
// meaningful, everything else (including an absent movement type) is
// unclassified.
func Classify(movementType *string) Classification {
	if movementType == nil {
		return Unclassified
	}
	switch *movementType {
	case "CREDIT":
		return Inflow
	case "DEBIT":
		return Outflow
	default:
		return Unclassified
	}
}

// InclusionState is whether a transaction counts toward totals ("considered")
// or has been manually or automatically excluded ("ignored").
type InclusionState string

const (
	Considered InclusionState = "considered"
	Ignored    InclusionState = "ignored"
)

// Source identifies which amount/currency pair an EffectiveMoney came from.
type Source string

const (
	AccountCurrency     Source = "account_currency"
	TransactionCurrency Source = "transaction_currency"
)

// EffectiveMoney is the (value, currency, source) triple a transaction should
// be treated as worth, once the two possible amount/currency pairs a provider
// can supply have been reconciled into one.
type EffectiveMoney struct {
	Value        decimal.Decimal
	CurrencyCode string
	Source       Source
}

// SelectEffectiveMoney mirrors rules.select_effective_money: the
// account-currency amount wins whenever both it and a non-empty account
// currency are present, otherwise the transaction-currency amount is used if
// available, otherwise there is no effective money at all.
func SelectEffectiveMoney(
	amountInAccountCurrency *decimal.Decimal,
	accountCurrency *string,
	amount *decimal.Decimal,
	transactionCurrency *string,
) *EffectiveMoney {
	if amountInAccountCurrency != nil && isTruthyCurrency(accountCurrency) {
		return &EffectiveMoney{Value: *amountInAccountCurrency, CurrencyCode: *accountCurrency, Source: AccountCurrency}
	}
	if amount != nil && isTruthyCurrency(transactionCurrency) {
		return &EffectiveMoney{Value: *amount, CurrencyCode: *transactionCurrency, Source: TransactionCurrency}
	}
	return nil
}

func isTruthyCurrency(s *string) bool {
	return s != nil && *s != ""
}

// CategoryKind is the internal category's role. Expense/Income are labels
// only; Transfer is the one kind that carries a rule with it — a transaction
// categorized as a transfer between the user's own accounts is excluded from
// income/expense totals, because otherwise the same money is counted twice
// (an outflow on the origin account and an inflow on the destination one).
// See Eligibility for the exact scope of that exclusion.
type CategoryKind string

const (
	Expense  CategoryKind = "expense"
	Income   CategoryKind = "income"
	Transfer CategoryKind = "transfer"
)

// EligibilityReason explains why a transaction was excluded from totals; nil
// (no reason) means it was included.
type EligibilityReason string

const (
	ReasonIgnored          EligibilityReason = "ignored"
	ReasonUnclassified     EligibilityReason = "unclassified"
	ReasonIneligibleStatus EligibilityReason = "ineligible_status"
	ReasonMissingMoneyPair EligibilityReason = "missing_money_pair"
	ReasonZeroValue        EligibilityReason = "zero_value"
	// Listed last to match Eligibility's check order, which is load-bearing:
	// see the ordering note there and MovedCash below.
	ReasonTransferCategory   EligibilityReason = "transfer_category"
	ReasonInvestmentTransfer EligibilityReason = "investment_transfer"
	ReasonLinkedTransfer     EligibilityReason = "linked_transfer"
)

var eligibleProviderStatuses = map[string]bool{"POSTED": true, "PENDING": true}

// Eligibility accepts POSTED and PENDING: an unsettled card purchase can
// already be a valid expense. Status alone neither assigns a report period
// nor proves bank cash moved; consumers apply account and date rules.
//
// Checks run in a fixed order: only the first failing reason is reported.
// An ignored transaction reports "ignored" even if a later check also fails.
//
// Two things keep a transaction out of totals, and they are not the same
// claim. An "ignored" inclusion decision means "this shouldn't be here at
// all" (a reversal, a duplicate). A Transfer categoryKind means "this is
// here, and it is real, but it is my own money moving between my own
// accounts" — counting it would double-count the same money. Ignored is
// checked first because it is the user's most explicit statement, and
// because the frontend contract requires an ignored transaction to report
// exactly "ignored" (see totals_eligibility in frontend/src/api/contracts.ts).
//
// The transfer check is deliberately *last*, after every check that asks
// whether the entry is otherwise eligible. MovedCash reads
// ReasonTransferCategory as a cash-flow candidate, so that reason must never
// be reached by an unsupported status (such as PROCESSING) or an
// unclassified movement. Reporting those as cash movement made
// every day before it read wrong in the timeline and in wealth (a
// PROCESSING transfer was reversed out of the anchor; an unclassified one was
// reversed out with the sign flipped). Ordering it last makes
// "transfer_category" mean exactly what MovedCash claims: otherwise eligible,
// excluded only because both legs would count the same money twice.
//
// categoryKind is the kind of the transaction's assigned internal category,
// or "" when it has none. Callers that compute *balances* rather than
// income/expense flows must pass "" — the transfer rule is a reporting
// decision and never corrects an account balance. See the comments at the
// call sites in networth/series.go and transactions/cardtotal.go, which
// pass Considered for the very same reason.
//
// Callers that receive an already-evaluated result instead of calling this
// themselves face the same distinction, and must not read the boolean as
// "this money did not move": see MovedCash below, and the
// transactions.TotalsEligibility.MovesCash method built on it.
func Eligibility(
	classification Classification,
	providerStatus *string,
	money *EffectiveMoney,
	inclusionState InclusionState,
	categoryKind CategoryKind,
) (bool, *EligibilityReason) {
	reason := func(r EligibilityReason) (bool, *EligibilityReason) { return false, &r }

	if inclusionState == Ignored {
		return reason(ReasonIgnored)
	}
	if classification == Unclassified {
		return reason(ReasonUnclassified)
	}
	if providerStatus == nil || !eligibleProviderStatuses[*providerStatus] {
		return reason(ReasonIneligibleStatus)
	}
	if money == nil {
		return reason(ReasonMissingMoneyPair)
	}
	if money.Value.IsZero() {
		return reason(ReasonZeroValue)
	}
	// Last on purpose — see the ordering note above.
	if categoryKind == Transfer {
		return reason(ReasonTransferCategory)
	}
	return true, nil
}

// MovedCash reports whether a transaction Eligibility excluded still moved
// money in a cash-flow calculation. It is not proof of bank settlement;
// PENDING is supported, and consumers still apply account and date rules.
//
// Transfer exclusions (category or investment principal) retain cash effect.
// Other reasons describe an unusable movement (no amount, a zero
// amount, an unclassified movement, an unsupported provider status)
// or one the user declared should not be here at all (ignored: a reversal, a
// duplicate — a row that is not a distinct movement of the account's reported
// balance either, which is why dropping it does not contradict
// transactions.CashOnHandByCurrency's refusal to back ignored rows *out* of that
// balance). A transfer is the odd one out precisely because it is real —
// it is excluded from income/expense totals only to stop the same money
// being counted on both legs, and the money still left the origin account.
//
// This only holds because Eligibility runs the transfer check last: a
// transfer that would also fail one of those other checks is reported under
// that check's reason instead, and so never claims to have moved cash.
//
// Anything reconstructing a *balance* from transactions must consult this
// rather than the included flag, or every day before a transfer comes out
// short by its full amount. That is the same trap the direct Eligibility
// callers in networth/series.go and transactions/cardtotal.go sidestep by
// passing an empty categoryKind.
func MovedCash(reason *EligibilityReason) bool {
	return reason != nil && (*reason == ReasonTransferCategory || *reason == ReasonInvestmentTransfer || *reason == ReasonLinkedTransfer)
}

// CanonicalDecimal renders value the way the reference API does: fixed-point
// with the value's own scale preserved (decimal.String() trims trailing
// zeros, which would silently change "1.50" to "1.5"), never scientific
// notation, and never a signed zero.
func CanonicalDecimal(value decimal.Decimal) string {
	if value.IsZero() {
		value = value.Abs()
	}
	var places int32
	if exp := value.Exponent(); exp < 0 {
		places = -exp
	}
	return value.StringFixed(places)
}

// GroupBy is the time bucket a transaction query groups by.
type GroupBy string

const (
	GroupNone  GroupBy = "none"
	GroupDay   GroupBy = "day"
	GroupWeek  GroupBy = "week"
	GroupMonth GroupBy = "month"
	GroupYear  GroupBy = "year"
)

// PeriodKind is a Period's bucket kind, which also covers "undated" — a
// possibility GroupBy itself does not need to express, since it only selects
// the bucketing strategy, not a specific bucket's outcome.
type PeriodKind string

const (
	PeriodNone    PeriodKind = "none"
	PeriodDay     PeriodKind = "day"
	PeriodWeek    PeriodKind = "week"
	PeriodMonth   PeriodKind = "month"
	PeriodYear    PeriodKind = "year"
	PeriodUndated PeriodKind = "undated"
)

// Period is the time bucket a transaction falls into for a given GroupBy.
type Period struct {
	Key       string
	Kind      PeriodKind
	StartDate *Date
	EndDate   *Date
}

func (p EligibilityReason) String() string { return string(p) }
