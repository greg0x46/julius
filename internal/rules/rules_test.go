package rules_test

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/rules"
)

func strp(s string) *string { return &s }
func intp(i int) *int       { return &i }

func TestMatchesDescriptionContainsIsCaseInsensitive(t *testing.T) {
	candidate := rules.MatchCandidate{Description: strp("PIX Enviado Mercado Livre")}
	conditions := []rules.Condition{{Field: rules.FieldDescription, Operator: rules.OperatorContains, Value: "mercado livre"}}
	if !rules.Matches(candidate, conditions, rules.LogicAnd) {
		t.Error("expected a case-insensitive substring match")
	}
}

func TestMatchesDescriptionEqualsRequiresExactNormalizedMatch(t *testing.T) {
	candidate := rules.MatchCandidate{Description: strp("  Uber Trip  ")}
	conditions := []rules.Condition{{Field: rules.FieldDescription, Operator: rules.OperatorEquals, Value: "uber trip"}}
	if !rules.Matches(candidate, conditions, rules.LogicAnd) {
		t.Error("expected equals to match after trim+casefold normalization")
	}

	conditions = []rules.Condition{{Field: rules.FieldDescription, Operator: rules.OperatorEquals, Value: "uber"}}
	if rules.Matches(candidate, conditions, rules.LogicAnd) {
		t.Error("equals should not match a mere substring")
	}
}

func TestMatchesNilCandidateFieldNeverMatches(t *testing.T) {
	candidate := rules.MatchCandidate{}
	conditions := []rules.Condition{{Field: rules.FieldDescription, Operator: rules.OperatorContains, Value: "anything"}}
	if rules.Matches(candidate, conditions, rules.LogicAnd) {
		t.Error("a nil candidate field should never match")
	}
}

func TestMatchesCardField(t *testing.T) {
	candidate := rules.MatchCandidate{CardNumber: strp("**** 1234")}
	conditions := []rules.Condition{{Field: rules.FieldCard, Operator: rules.OperatorContains, Value: "1234"}}
	if !rules.Matches(candidate, conditions, rules.LogicAnd) {
		t.Error("expected card field match")
	}
}

func TestMatchesAccountFieldChecksNameOrInstitution(t *testing.T) {
	byName := rules.MatchCandidate{AccountName: strp("Conta Corrente"), AccountInstitution: strp("Banco X")}
	byInstitution := rules.MatchCandidate{AccountName: strp("Poupança"), AccountInstitution: strp("Nubank")}
	conditions := []rules.Condition{{Field: rules.FieldAccount, Operator: rules.OperatorContains, Value: "nubank"}}

	if rules.Matches(byName, conditions, rules.LogicAnd) {
		t.Error("should not match when neither name nor institution contains the value")
	}
	if !rules.Matches(byInstitution, conditions, rules.LogicAnd) {
		t.Error("should match via institution when name doesn't match")
	}
}

func TestMatchesLogicAndRequiresAllConditions(t *testing.T) {
	candidate := rules.MatchCandidate{Description: strp("Uber Trip"), CardNumber: strp("1234")}
	conditions := []rules.Condition{
		{Field: rules.FieldDescription, Operator: rules.OperatorContains, Value: "uber"},
		{Field: rules.FieldCard, Operator: rules.OperatorEquals, Value: "9999"},
	}
	if rules.Matches(candidate, conditions, rules.LogicAnd) {
		t.Error("AND should fail when one condition doesn't match")
	}
	if !rules.Matches(candidate, conditions, rules.LogicOr) {
		t.Error("OR should succeed when at least one condition matches")
	}
}

func amountp(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

func TestMatchesWithinPercentAcceptsAmountInsideTolerance(t *testing.T) {
	candidate := rules.MatchCandidate{Amount: amountp("1540.00")}
	conditions := []rules.Condition{{Field: rules.FieldAmount, Operator: rules.OperatorWithinPercent, Value: "1500.00:10"}}
	if !rules.Matches(candidate, conditions, rules.LogicAnd) {
		t.Error("1540 should be within 10% of 1500 (max diff 150)")
	}
}

func TestMatchesWithinPercentRejectsAmountOutsideTolerance(t *testing.T) {
	candidate := rules.MatchCandidate{Amount: amountp("1800.00")}
	conditions := []rules.Condition{{Field: rules.FieldAmount, Operator: rules.OperatorWithinPercent, Value: "1500.00:10"}}
	if rules.Matches(candidate, conditions, rules.LogicAnd) {
		t.Error("1800 should be outside 10% of 1500 (max diff 150)")
	}
}

func TestMatchesWithinPercentNilAmountNeverMatches(t *testing.T) {
	candidate := rules.MatchCandidate{}
	conditions := []rules.Condition{{Field: rules.FieldAmount, Operator: rules.OperatorWithinPercent, Value: "1500.00:10"}}
	if rules.Matches(candidate, conditions, rules.LogicAnd) {
		t.Error("a nil candidate amount should never match")
	}
}

func TestMatchesWithinPercentMalformedValueNeverMatches(t *testing.T) {
	candidate := rules.MatchCandidate{Amount: amountp("1500.00")}
	conditions := []rules.Condition{{Field: rules.FieldAmount, Operator: rules.OperatorWithinPercent, Value: "not-a-number"}}
	if rules.Matches(candidate, conditions, rules.LogicAnd) {
		t.Error("a malformed condition value should never match")
	}
}

func TestMatchesDayRangeAcceptsDayInsideRange(t *testing.T) {
	candidate := rules.MatchCandidate{DayOfMonth: intp(17)}
	conditions := []rules.Condition{{Field: rules.FieldDayOfMonth, Operator: rules.OperatorDayRange, Value: "15:20"}}
	if !rules.Matches(candidate, conditions, rules.LogicAnd) {
		t.Error("day 17 should be within the 15-20 range")
	}
}

func TestMatchesDayRangeRejectsDayOutsideRange(t *testing.T) {
	candidate := rules.MatchCandidate{DayOfMonth: intp(25)}
	conditions := []rules.Condition{{Field: rules.FieldDayOfMonth, Operator: rules.OperatorDayRange, Value: "15:20"}}
	if rules.Matches(candidate, conditions, rules.LogicAnd) {
		t.Error("day 25 should be outside the 15-20 range")
	}
}

func TestMatchesDayRangeWrapsAroundMonthBoundary(t *testing.T) {
	candidate := rules.MatchCandidate{DayOfMonth: intp(1)}
	conditions := []rules.Condition{{Field: rules.FieldDayOfMonth, Operator: rules.OperatorDayRange, Value: "28:2"}}
	if !rules.Matches(candidate, conditions, rules.LogicAnd) {
		t.Error("day 1 should be inside the 28-2 range across the month boundary")
	}
}

func TestMatchesDayRangeRejectsDayOutsideWrappedRange(t *testing.T) {
	candidate := rules.MatchCandidate{DayOfMonth: intp(15)}
	conditions := []rules.Condition{{Field: rules.FieldDayOfMonth, Operator: rules.OperatorDayRange, Value: "28:2"}}
	if rules.Matches(candidate, conditions, rules.LogicAnd) {
		t.Error("day 15 should be outside the 28-2 wrapped range")
	}
}

func TestMatchesDayRangeNilDayNeverMatches(t *testing.T) {
	candidate := rules.MatchCandidate{}
	conditions := []rules.Condition{{Field: rules.FieldDayOfMonth, Operator: rules.OperatorDayRange, Value: "15:20"}}
	if rules.Matches(candidate, conditions, rules.LogicAnd) {
		t.Error("a nil candidate day should never match")
	}
}

func TestWithinPercentExactBoundary(t *testing.T) {
	for _, tc := range []struct {
		amount, condition string
		want              bool
	}{
		{"9007199254740993.01", "9007199254740993.01:0", true},
		{"9007199254740993.02", "9007199254740993.01:0", false},
		{"1.01000000000000000101", "1.000000000000000001:1", true},
		{"1.01000000000000000102", "1.000000000000000001:1", false},
		{"-1.01", "-1:1", true},
		{"1", "NaN:1", false},
	} {
		amount := decimal.RequireFromString(tc.amount)
		condition := rules.Condition{Field: rules.FieldAmount, Operator: rules.OperatorWithinPercent, Value: tc.condition}
		if got := rules.Matches(rules.MatchCandidate{Amount: &amount}, []rules.Condition{condition}, rules.LogicAnd); got != tc.want {
			t.Errorf("%s within %s = %v, want %v", tc.amount, tc.condition, got, tc.want)
		}
	}
}
