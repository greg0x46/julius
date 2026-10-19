// Package rules implements a small, combinable condition-matching core,
// shared by internal/automation (auto-ignore rules matching on transaction
// text) and internal/recurrences (reconciling a RecurringCommitment's
// expected monthly occurrence against real transactions by category/amount/
// day). It started as internal/automation/matching.go and was generalized
// here so both packages share one matching engine instead of two parallel
// implementations — see .specs/motores-de-dominio.md section 2.
package rules

import (
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

type ConditionField string

const (
	FieldDescription ConditionField = "description"
	FieldCard        ConditionField = "card"
	FieldAccount     ConditionField = "account"
	FieldAmount      ConditionField = "amount"
	FieldDayOfMonth  ConditionField = "day_of_month"
)

type ConditionOperator string

const (
	OperatorContains ConditionOperator = "contains"
	OperatorEquals   ConditionOperator = "equals"
	// OperatorWithinPercent carries its reference value inside
	// Condition.Value as "<reference>:<tolerance>" (see package doc and
	// m0-motor-de-regras.md) because Matches only ever sees the candidate
	// being tested, never a second "expected" value.
	OperatorWithinPercent ConditionOperator = "within_percent"
	// OperatorDayRange carries its bounds inside Condition.Value as
	// "<min>:<max>", both 1-31. It's a plain absolute day-of-month window —
	// unlike OperatorWithinPercent it has no "expected" reference to inject
	// at resolve time, so the same condition applies unchanged regardless of
	// which occurrence it's tested against. min > max wraps around the
	// month boundary (e.g. "28:5" covers day 28 through day 5).
	OperatorDayRange ConditionOperator = "day_range"
)

type LogicOperator string

const (
	LogicAnd LogicOperator = "and"
	LogicOr  LogicOperator = "or"
)

// Condition is one field/operator/value test within a rule.
type Condition struct {
	Field    ConditionField
	Operator ConditionOperator
	Value    string
}

// MatchCandidate is the set of fields a Condition can test against.
type MatchCandidate struct {
	Description        *string
	CardNumber         *string
	AccountName        *string
	AccountInstitution *string
	Amount             *decimal.Decimal
	DayOfMonth         *int
}

// normalize mirrors _normalize (strip + casefold). Go's strings.ToLower is a
// practical stand-in for Python's casefold: both make ASCII (and the vast
// majority of real-world) text case-insensitively comparable; casefold's
// extra Unicode special-casing (e.g. German ß) is not worth a dependency for
// matching bank transaction descriptions and account names.
func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func textMatches(candidateValue *string, operator ConditionOperator, value string) bool {
	if candidateValue == nil {
		return false
	}
	nc, nv := normalize(*candidateValue), normalize(value)
	if operator == OperatorEquals {
		return nc == nv
	}
	return strings.Contains(nc, nv)
}

// ParseWithinPercent parses a "<reference>:<tolerance percent>" Value. ok is
// false for a malformed value or a zero reference, which can never match.
func ParseWithinPercent(value string) (reference, tolerance decimal.Decimal, ok bool) {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		return decimal.Zero, decimal.Zero, false
	}
	reference, err := decimal.NewFromString(strings.TrimSpace(parts[0]))
	if err != nil || reference.IsZero() {
		return decimal.Zero, decimal.Zero, false
	}
	tolerance, err = decimal.NewFromString(strings.TrimSpace(parts[1]))
	if err != nil {
		return decimal.Zero, decimal.Zero, false
	}
	return reference, tolerance, true
}

// ParseDayRange parses a "<min>:<max>" Value. ok is false for a malformed
// value; the bounds themselves are not range-checked here.
func ParseDayRange(value string) (min, max int, ok bool) {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	min, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, false
	}
	max, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, false
	}
	return min, max, true
}

// Malformed amount conditions never match.
func withinPercentMatches(candidateAmount *decimal.Decimal, value string) bool {
	if candidateAmount == nil {
		return false
	}
	reference, tolerance, ok := ParseWithinPercent(value)
	if !ok {
		return false
	}
	diff := candidateAmount.Sub(reference).Abs()
	limit := reference.Abs().Mul(tolerance).Shift(-2)
	return diff.LessThanOrEqual(limit)
}

// dayRangeMatches parses a "<min>:<max>" Value and checks candidateDay falls
// within it, inclusive on both ends. min > max wraps around the month
// boundary (e.g. "28:5" matches day 28 through day 31, then day 1 through
// day 5) so a window spanning the turn of the month doesn't need special
// casing by the caller.
func dayRangeMatches(candidateDay *int, value string) bool {
	if candidateDay == nil {
		return false
	}
	min, max, ok := ParseDayRange(value)
	if !ok {
		return false
	}
	day := *candidateDay
	if min <= max {
		return day >= min && day <= max
	}
	return day >= min || day <= max
}

func conditionMatches(candidate MatchCandidate, condition Condition) bool {
	switch condition.Field {
	case FieldDescription:
		return textMatches(candidate.Description, condition.Operator, condition.Value)
	case FieldCard:
		return textMatches(candidate.CardNumber, condition.Operator, condition.Value)
	case FieldAmount:
		return withinPercentMatches(candidate.Amount, condition.Value)
	case FieldDayOfMonth:
		return dayRangeMatches(candidate.DayOfMonth, condition.Value)
	default: // FieldAccount
		return textMatches(candidate.AccountName, condition.Operator, condition.Value) ||
			textMatches(candidate.AccountInstitution, condition.Operator, condition.Value)
	}
}

// Matches mirrors matches: an empty conditions slice never occurs in
// practice (the schema and the write-side validation both require at least
// one), but "and" over zero conditions is vacuously true and "or" is false,
// matching Python's all()/any() on an empty generator.
func Matches(candidate MatchCandidate, conditions []Condition, logic LogicOperator) bool {
	if logic == LogicAnd {
		for _, c := range conditions {
			if !conditionMatches(candidate, c) {
				return false
			}
		}
		return true
	}
	for _, c := range conditions {
		if conditionMatches(candidate, c) {
			return true
		}
	}
	return false
}
