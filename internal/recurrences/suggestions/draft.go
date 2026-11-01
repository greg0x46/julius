// Package suggestions builds read-only reconciliation drafts from historical evidence.
package suggestions

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/automation"
	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/rules"
	"github.com/greg0x46/julius/internal/transactions"
)

// Observation associates a discovered transaction with its intended occurrence.
// Keeping both dates exposes payments made outside the occurrence's month.
type Observation struct {
	Transaction    transactions.Item
	OccurrenceDate time.Time
}

type Draft struct {
	Commitment recurrences.RecurringCommitment
	Rule       automation.Write
	Evidence   []Observation
	Rationale  []string
	Warnings   []string
}

// FromSuggestion resolves discovery evidence from history. Discovery schedules
// start in the future; the owner supplies the eventual target ID and any missing category.
func FromSuggestion(candidate recurrences.Suggestion, targetID, categoryID string, history []transactions.Item) (Draft, error) {
	commitment := candidate.Draft
	commitment.ID = targetID
	if categoryID != "" {
		commitment.CategoryID = categoryID
	}
	byID := make(map[string]transactions.Item, len(history))
	for _, item := range history {
		byID[item.ID] = item
	}
	evidence := make([]Observation, 0, len(candidate.TransactionIDs))
	for _, id := range candidate.TransactionIDs {
		item, ok := byID[id]
		if !ok || item.OccurredAt == nil {
			return Draft{}, fmt.Errorf("discovery evidence transaction is missing from history")
		}
		date := *item.OccurredAt
		anchor := min(commitment.DayOfMonth, dates.DaysInMonth(date.Year(), date.Month()))
		occurrence := time.Date(date.Year(), date.Month(), anchor, 0, 0, 0, 0, time.UTC)
		evidence = append(evidence, Observation{Transaction: item, OccurrenceDate: occurrence})
	}
	return Generate(commitment, evidence)
}

// Generate never saves the rule or its target. Commitment.ID is the eventual
// scenario ID; saving later must still validate target/category existence.
func Generate(commitment recurrences.RecurringCommitment, evidence []Observation) (Draft, error) {
	if err := commitment.Validate(); err != nil {
		return Draft{}, err
	}
	if strings.TrimSpace(commitment.ID) == "" || len(evidence) < 2 {
		return Draft{}, fmt.Errorf("a target scenario ID and at least two observations are required")
	}
	d := Draft{Commitment: commitment, Evidence: append([]Observation(nil), evidence...)}
	d.Rule = automation.Write{
		Name: commitment.Name, LogicOperator: rules.LogicAnd,
		Actions: []automation.ActionWrite{{Type: automation.ActionReconcile, ScenarioID: &d.Commitment.ID}},
	}
	var amounts []decimal.Decimal
	var descriptions []string
	var days []int
	seen := map[string]bool{}
	for _, observation := range evidence {
		item := observation.Transaction
		if item.ID == "" || seen[item.ID] || !cashCandidate(item) {
			return Draft{}, fmt.Errorf("evidence must contain distinct dated cash movements with valid effective money")
		}
		seen[item.ID] = true
		if !recurrences.EligibilityForItem(commitment.Kind, item, false).Eligible {
			return Draft{}, fmt.Errorf("evidence must have eligible BRL money in the commitment's direction")
		}
		if commitment.AccountID != nil && item.Account.ID != *commitment.AccountID {
			return Draft{}, fmt.Errorf("evidence must belong to the target account")
		}
		day := dates.Day(observation.OccurrenceDate)
		historical := commitment
		historical.StartDate, historical.EndDate, historical.IsActive = day, nil, true
		if len(historical.Occurrences(day, day)) != 1 {
			return Draft{}, fmt.Errorf("evidence date must follow the proposed calendar")
		}
		if !recurrences.MonthKey(day).Equal(recurrences.MonthKey(*item.OccurredAt)) {
			d.Warnings = appendUnique(d.Warnings, "cross_month_unsupported")
		}
		amount, _ := decimal.NewFromString(item.EffectiveMoney.Value)
		if amount.IsZero() {
			return Draft{}, fmt.Errorf("evidence amounts must be non-zero")
		}
		amounts = append(amounts, amount.Abs())
		description := ""
		if item.Description != nil {
			description = strings.ToLower(strings.TrimSpace(*item.Description))
		}
		descriptions = append(descriptions, description)
		days = append(days, item.OccurredAt.Day())
	}
	text, operator := sharedDescription(descriptions)
	if text != "" {
		d.Rule.Conditions = append(d.Rule.Conditions, rules.Condition{Field: rules.FieldDescription, Operator: operator, Value: text})
		d.Rationale = append(d.Rationale, "description: shared historical text")
	}
	if text == "" || (operator == rules.OperatorContains && len(strings.Fields(text)) == 1) {
		d.Warnings = appendUnique(d.Warnings, "broad_description")
	}
	minAmount, maxAmount := amounts[0], amounts[0]
	for _, amount := range amounts[1:] {
		minAmount = decimal.Min(minAmount, amount)
		maxAmount = decimal.Max(maxAmount, amount)
	}
	reference := minAmount.Add(maxAmount).Mul(decimal.New(5, -1))
	difference := maxAmount.Sub(reference)
	// Round the percentage outward so division cannot exclude an observed endpoint.
	precision := int32(decimal.DivisionPrecision)
	tolerance := difference.Mul(decimal.NewFromInt(100)).DivRound(reference, precision)
	if reference.Mul(tolerance).Shift(-2).LessThan(difference) {
		tolerance = tolerance.Add(decimal.New(1, -precision))
	}
	d.Rule.Conditions = append(d.Rule.Conditions, rules.Condition{
		Field: rules.FieldAmount, Operator: rules.OperatorWithinPercent, Value: reference.String() + ":" + tolerance.String(),
	})
	d.Rationale = append(d.Rationale, "amount: observed range "+minAmount.String()+" to "+maxAmount.String()+"; percentage rounded outward")
	first, last := dayWindow(days)
	d.Rule.Conditions = append(d.Rule.Conditions, rules.Condition{
		Field: rules.FieldDayOfMonth, Operator: rules.OperatorDayRange, Value: fmt.Sprintf("%d:%d", first, last),
	})
	d.Rationale = append(d.Rationale, fmt.Sprintf("day: smallest observed calendar window %d:%d", first, last))
	if first > last {
		d.Warnings = appendUnique(d.Warnings, "cross_month_unsupported")
	}
	firstItem := evidence[0].Transaction
	accountID, accountName, card, category := firstItem.Account.ID, accountLabel(firstItem), "", ""
	if firstItem.Card != nil {
		card = firstItem.Card.Number
	}
	if firstItem.InternalCategory != nil && firstItem.InternalCategory.IsActive {
		category = firstItem.InternalCategory.ID
	}
	for _, observation := range evidence[1:] {
		item := observation.Transaction
		if item.Account.ID != accountID {
			accountID = ""
		}
		if accountLabel(item) != accountName {
			accountName = ""
		}
		if item.Card == nil || item.Card.Number != card {
			card = ""
		}
		if item.InternalCategory == nil || !item.InternalCategory.IsActive || item.InternalCategory.ID != category {
			category = ""
		}
	}
	if accountID != "" {
		d.Commitment.AccountID = &accountID
	}
	if accountName != "" {
		d.Rule.Conditions = append(d.Rule.Conditions, rules.Condition{Field: rules.FieldAccount, Operator: rules.OperatorEquals, Value: accountName})
		d.Rationale = append(d.Rationale, "account: identical historical account label")
	}
	if d.Commitment.AccountID == nil {
		d.Warnings = appendUnique(d.Warnings, "unrestricted_account")
	}
	if card != "" {
		d.Rule.Conditions = append(d.Rule.Conditions, rules.Condition{Field: rules.FieldCard, Operator: rules.OperatorEquals, Value: card})
		d.Rationale = append(d.Rationale, "card: identical historical card number")
	}
	if category != "" {
		d.Rule.Actions = append(d.Rule.Actions, automation.ActionWrite{Type: automation.ActionSetCategory, CategoryID: &category})
		d.Rationale = append(d.Rationale, "category: all observations share one active category; manual decisions retain precedence")
	}
	return d, d.Rule.Validate()
}

func label(value *string) string {
	if value == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(*value))
}

func accountLabel(item transactions.Item) string {
	if name := label(item.Account.Name); name != "" {
		return name
	}
	return label(item.Account.Institution)
}

func sharedDescription(values []string) (string, rules.ConditionOperator) {
	equal := true
	for _, value := range values[1:] {
		equal = equal && value == values[0]
	}
	if equal {
		return values[0], rules.OperatorEquals
	}
	words := strings.Fields(values[0])
	best := ""
	for start := range words {
		for end := start + 1; end <= len(words); end++ {
			text := strings.Join(words[start:end], " ")
			shared := true
			for _, value := range values {
				shared = shared && strings.Contains(value, text)
			}
			if shared && (len(text) > len(best) || (len(text) == len(best) && text < best)) {
				best = text
			}
		}
	}
	return best, rules.OperatorContains
}

func dayWindow(days []int) (int, int) {
	sort.Ints(days)
	first, last, gap := days[0], days[len(days)-1], days[0]+31-days[len(days)-1]
	for i := 1; i < len(days); i++ {
		if days[i]-days[i-1] > gap {
			first, last, gap = days[i], days[i-1], days[i]-days[i-1]
		}
	}
	return first, last
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func cashCandidate(item transactions.Item) bool {
	if !item.TotalsEligibility.MovesCash() || item.OccurredAt == nil || item.EffectiveMoney == nil {
		return false
	}
	_, err := decimal.NewFromString(item.EffectiveMoney.Value)
	return err == nil
}
