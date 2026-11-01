package suggestions

import (
	"fmt"
	"time"

	"github.com/greg0x46/julius/internal/automation"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/rules"
	"github.com/greg0x46/julius/internal/transactions"
)

type PreviewRequest struct {
	Draft Draft
	From  time.Time
	To    time.Time
	// Supply complete history for the review window, not just discovery evidence.
	History     []transactions.Item
	Rules       []automation.Rule
	Commitments []recurrences.RecurringCommitment
	// Include manual decisions outside the window that claim loaded transactions.
	Overrides []recurrences.Override
}

type TransactionPreview struct {
	TransactionID     string
	Status            string // matched, unmatched, ambiguous
	MatchesConditions bool
	FailedConditions  []rules.Condition
	Reasons           []string
	RuleIDs           []string
	ScenarioIDs       []string
}

type PreviewResult struct {
	Transactions []TransactionPreview
	Occurrences  []recurrences.Reconciliation
	Warnings     []string
	NeedsReview  bool
}

// Preview simulates enabling the draft without applying any automation action.
// It accepts values only: no database handle, category writes or link persistence.
func Preview(request PreviewRequest) (PreviewResult, error) {
	d := request.Draft
	if err := d.Commitment.Validate(); err != nil {
		return PreviewResult{}, err
	}
	if err := d.Rule.Validate(); err != nil {
		return PreviewResult{}, err
	}
	if request.From.IsZero() || request.To.IsZero() || request.To.Before(request.From) || d.Rule.LogicOperator != rules.LogicAnd {
		return PreviewResult{}, fmt.Errorf("an ordered review period and an AND draft are required")
	}
	target := false
	for _, action := range d.Rule.Actions {
		if action.Type == automation.ActionReconcile && action.ScenarioID != nil && *action.ScenarioID == d.Commitment.ID {
			target = true
		}
	}
	if !target {
		return PreviewResult{}, fmt.Errorf("draft must target its proposed recurring scenario")
	}
	result := PreviewResult{Warnings: append([]string(nil), d.Warnings...)}
	var eligible []transactions.Item
	for _, item := range request.History {
		if cashCandidate(item) {
			eligible = append(eligible, item)
		}
	}
	byScenario := map[string][]recurrences.Override{}
	for _, override := range request.Overrides {
		byScenario[override.ScenarioID] = append(byScenario[override.ScenarioID], override)
	}
	reconciler := recurrences.NewReconciler(d.Commitment, d.Rule.Conditions, d.Rule.LogicOperator, true, byScenario[d.Commitment.ID], eligible)
	result.Occurrences = reconciler.ResolveRange(request.From, request.To)
	byMonth := map[time.Time]recurrences.Reconciliation{}
	for _, resolved := range result.Occurrences {
		byMonth[recurrences.MonthKey(resolved.Occurrence.Date)] = resolved
	}
	// Existing scenarios resolve independently today; surface shared winners
	// rather than inventing a new global rule ordering for this preview.
	existingClaims := map[string][]string{}
	for _, commitment := range request.Commitments {
		if commitment.ID == d.Commitment.ID {
			continue
		}
		var conditions []rules.Condition
		logic, hasRule := rules.LogicAnd, false
		for _, rule := range request.Rules {
			if rule.IsActive && targets(rule, commitment.ID) {
				conditions, logic, hasRule = rule.Conditions, rule.LogicOperator, true
				break
			}
		}
		existing := recurrences.NewReconciler(commitment, conditions, logic, hasRule, byScenario[commitment.ID], eligible)
		for _, resolved := range existing.ResolveRange(request.From, request.To) {
			if resolved.TransactionID != nil {
				existingClaims[*resolved.TransactionID] = appendUnique(existingClaims[*resolved.TransactionID], commitment.ID)
			}
		}
	}
	for _, rule := range request.Rules {
		if targets(rule, d.Commitment.ID) {
			result.Warnings = appendUnique(result.Warnings, "target_already_has_rule:"+rule.ID)
		}
	}
	evidence := map[string]bool{}
	for _, observation := range d.Evidence {
		evidence[observation.Transaction.ID] = true
	}
	matchingByMonth := map[time.Time][]int{}
	for _, item := range request.History {
		row := TransactionPreview{TransactionID: item.ID, Status: "unmatched"}
		if item.OccurredAt == nil || item.EffectiveMoney == nil {
			row.Reasons = append(row.Reasons, "missing_date_or_money")
		} else {
			for _, condition := range d.Rule.Conditions {
				if _, ok := recurrences.ResolveOccurrence([]rules.Condition{condition}, rules.LogicAnd, []transactions.Item{item}); !ok {
					row.FailedConditions = append(row.FailedConditions, condition)
				}
			}
			row.MatchesConditions = len(row.FailedConditions) == 0
			if !row.MatchesConditions {
				row.Reasons = append(row.Reasons, "conditions_failed")
			}
		}
		if !cashCandidate(item) {
			row.Reasons = append(row.Reasons, "ineligible_cash_movement")
		}
		if d.Commitment.AccountID != nil && item.Account.ID != *d.Commitment.AccountID {
			row.Reasons = append(row.Reasons, "account_restriction")
		}
		var resolved recurrences.Reconciliation
		hasOccurrence := false
		if item.OccurredAt != nil {
			resolved, hasOccurrence = byMonth[recurrences.MonthKey(*item.OccurredAt)]
		}
		if !hasOccurrence {
			row.Reasons = append(row.Reasons, "no_occurrence_in_month")
		}
		if len(row.Reasons) == 0 {
			row.Status = "matched"
			row.Reasons = append(row.Reasons, "conditions_and_month_match")
			if resolved.Detached {
				row.Status = "unmatched"
				row.Reasons = append(row.Reasons, "manual_detach")
			} else if resolved.Origin == recurrences.OriginManual {
				row.Status = "unmatched"
				row.Reasons = append(row.Reasons, "manual_link_precedence")
			} else {
				manuallyTaken := false
				for _, override := range byScenario[d.Commitment.ID] {
					manuallyTaken = manuallyTaken || (override.State == recurrences.StateLinked && override.TransactionID != nil && *override.TransactionID == item.ID)
				}
				if manuallyTaken {
					row.Status = "unmatched"
					row.Reasons = append(row.Reasons, "manually_taken")
				} else {
					month := recurrences.MonthKey(*item.OccurredAt)
					matchingByMonth[month] = append(matchingByMonth[month], len(result.Transactions))
				}
			}
			if !evidence[item.ID] {
				if row.Status == "matched" {
					row.Status = "ambiguous"
				}
				row.Reasons = append(row.Reasons, "possible_false_positive")
				result.Warnings = appendUnique(result.Warnings, "possible_false_positives")
			}
			for _, rule := range request.Rules {
				if rule.IsActive {
					if _, ok := recurrences.ResolveOccurrence(rule.Conditions, rule.LogicOperator, []transactions.Item{item}); ok {
						row.RuleIDs = appendUnique(row.RuleIDs, rule.ID)
					}
				}
			}
			row.ScenarioIDs = append(row.ScenarioIDs, existingClaims[item.ID]...)
			for _, override := range request.Overrides {
				if override.ScenarioID != d.Commitment.ID && override.State == recurrences.StateLinked && override.TransactionID != nil && *override.TransactionID == item.ID {
					row.ScenarioIDs = appendUnique(row.ScenarioIDs, override.ScenarioID)
				}
			}
			if len(row.RuleIDs) > 0 || len(row.ScenarioIDs) > 0 {
				if row.Status == "matched" {
					row.Status = "ambiguous"
				}
				row.Reasons = append(row.Reasons, "existing_rule_or_scenario_overlap")
				result.Warnings = appendUnique(result.Warnings, "existing_overlap")
			}
		}
		result.Transactions = append(result.Transactions, row)
	}
	for _, indices := range matchingByMonth {
		if len(indices) > 1 {
			result.Warnings = appendUnique(result.Warnings, "multiple_matches_per_occurrence")
			for _, index := range indices {
				result.Transactions[index].Status = "ambiguous"
				result.Transactions[index].Reasons = append(result.Transactions[index].Reasons, "multiple_matches_per_occurrence")
			}
		}
	}
	result.NeedsReview = len(result.Warnings) > 0
	return result, nil
}

func targets(rule automation.Rule, scenarioID string) bool {
	for _, action := range rule.Actions {
		if action.Type == automation.ActionReconcile && action.ScenarioID != nil && *action.ScenarioID == scenarioID {
			return true
		}
	}
	return false
}
