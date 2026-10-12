// Package automation ports app/automation_rules/matching.py and
// app/automation_rules/service.py: the catalog of user-defined rules, and
// matching+applying them (on a newly-synced transaction, and retroactively
// across every existing one) to act on transactions automatically.
//
// Matching primitives alias internal/rules. All transaction fields are available
// during ingestion and retroactive application; reconciliation uses the same
// amount/day conditions at read time.
package automation

import "github.com/greg0x46/julius/internal/rules"

type ConditionField = rules.ConditionField

const (
	FieldDescription = rules.FieldDescription
	FieldCard        = rules.FieldCard
	FieldAccount     = rules.FieldAccount
	FieldAmount      = rules.FieldAmount
	FieldDayOfMonth  = rules.FieldDayOfMonth
)

type ConditionOperator = rules.ConditionOperator

const (
	OperatorContains      = rules.OperatorContains
	OperatorEquals        = rules.OperatorEquals
	OperatorWithinPercent = rules.OperatorWithinPercent
	OperatorDayRange      = rules.OperatorDayRange
)

type LogicOperator = rules.LogicOperator

const (
	LogicAnd = rules.LogicAnd
	LogicOr  = rules.LogicOr
)

// Condition is one field/operator/value test within a Rule.
type Condition = rules.Condition

// MatchCandidate mirrors MatchCandidate: the transaction/account fields
// rules can test against.
type MatchCandidate = rules.MatchCandidate

// Matches mirrors matches, delegating to the shared engine.
func Matches(candidate MatchCandidate, conditions []Condition, logic LogicOperator) bool {
	return rules.Matches(candidate, conditions, logic)
}
