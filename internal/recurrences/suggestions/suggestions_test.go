package suggestions_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/automation"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/recurrences/suggestions"
	"github.com/greg0x46/julius/internal/rules"
	"github.com/greg0x46/julius/internal/transactions"
)

func day(value string) time.Time {
	d, err := time.Parse("2006-01-02", value)
	if err != nil {
		panic(err)
	}
	return d
}

func ptr[T any](value T) *T { return &value }

func item(id, date, amount, description string) transactions.Item {
	return transactions.Item{
		ID: id, OccurredAt: ptr(day(date)), Description: ptr(description),
		Account:          transactions.AccountSummary{ID: "account", Name: ptr("Bank")},
		Card:             &transactions.CardInfo{Number: "1234"},
		InternalCategory: &transactions.InternalCategory{ID: "category", IsActive: true},
		EffectiveMoney:   &transactions.EffectiveMoneyView{Value: amount, CurrencyCode: "BRL", Source: money.AccountCurrency},
		Classification:   money.Outflow, Inclusion: transactions.Inclusion{State: money.Considered},
		TotalsEligibility: transactions.TotalsEligibility{Included: true},
	}
}

func commitment() recurrences.RecurringCommitment {
	return recurrences.RecurringCommitment{
		ID: "proposed", Name: "Internet", Kind: recurrences.KindExpense,
		Amount: decimal.RequireFromString("100"), CategoryID: "category",
		Cadence: recurrences.CadenceMonthly, DayOfMonth: 10,
		StartDate: day("2026-01-01"), IsActive: true,
	}
}

func fixture(t *testing.T) (suggestions.Draft, []transactions.Item) {
	t.Helper()
	history := []transactions.Item{
		item("jan", "2026-01-10", "-99.999999999999999999", "PIX Fiber Internet JAN"),
		item("feb", "2026-02-12", "-103.000000000000000001", "PIX Fiber Internet FEB"),
		item("mar", "2026-03-11", "-101", "PIX Fiber Internet MAR"),
	}
	evidence := []suggestions.Observation{
		{Transaction: history[0], OccurrenceDate: day("2026-01-10")},
		{Transaction: history[1], OccurrenceDate: day("2026-02-10")},
		{Transaction: history[2], OccurrenceDate: day("2026-03-10")},
	}
	draft, err := suggestions.Generate(commitment(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	return draft, history
}

func condition(t *testing.T, draft suggestions.Draft, field rules.ConditionField) rules.Condition {
	t.Helper()
	for _, c := range draft.Rule.Conditions {
		if c.Field == field {
			return c
		}
	}
	t.Fatalf("missing condition %s", field)
	return rules.Condition{}
}

func request(draft suggestions.Draft, history []transactions.Item) suggestions.PreviewRequest {
	return suggestions.PreviewRequest{Draft: draft, History: history, From: day("2026-01-01"), To: day("2026-03-31")}
}

func preview(t *testing.T, req suggestions.PreviewRequest) suggestions.PreviewResult {
	t.Helper()
	result, err := suggestions.Preview(req)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestGenerateValidSpecificDraftWithObservedTolerances(t *testing.T) {
	draft, history := fixture(t)
	if err := draft.Rule.Validate(); err != nil {
		t.Fatal(err)
	}
	if draft.Rule.IsActive || draft.Rule.LogicOperator != rules.LogicAnd || len(draft.Rule.Conditions) != 5 {
		t.Fatalf("unexpected draft: %+v", draft)
	}
	if got := condition(t, draft, rules.FieldDescription); got.Operator != rules.OperatorContains || got.Value != "pix fiber internet" {
		t.Fatalf("description: %+v", got)
	}
	if got := condition(t, draft, rules.FieldDayOfMonth).Value; got != "10:12" {
		t.Fatalf("days = %s", got)
	}
	if draft.Commitment.AccountID == nil || *draft.Commitment.AccountID != "account" {
		t.Fatal("missing account restriction")
	}
	if len(draft.Rule.Actions) != 2 || *draft.Rule.Actions[0].ScenarioID != draft.Commitment.ID || *draft.Rule.Actions[1].CategoryID != "category" {
		t.Fatalf("actions: %+v", draft.Rule.Actions)
	}
	result := preview(t, request(draft, history))
	if result.NeedsReview || len(result.Occurrences) != 3 || len(draft.Rationale) != 6 {
		t.Fatalf("unexpected review: %+v", result)
	}
	for i, row := range result.Transactions {
		if row.Status != "matched" || !row.MatchesConditions || *result.Occurrences[i].TransactionID != history[i].ID {
			t.Fatalf("evidence endpoint excluded: %+v", row)
		}
	}
	outside := item("outside", "2026-03-11", "-103.000001", "PIX Fiber Internet XXX")
	result = preview(t, request(draft, append(history, outside)))
	row := result.Transactions[3]
	if row.Status != "unmatched" || len(row.FailedConditions) != 1 || row.FailedConditions[0].Field != rules.FieldAmount {
		t.Fatalf("amount outside observed range matched: %+v", row)
	}
}

func TestGenerateExactTextAndZeroVariation(t *testing.T) {
	history := []transactions.Item{
		item("jan", "2026-01-10", "-0.00000000000000000001", "  Fiber Internet "),
		item("feb", "2026-02-10", "-0.00000000000000000001", "fiber INTERNET"),
	}
	draft, err := suggestions.Generate(commitment(), []suggestions.Observation{
		{Transaction: history[0], OccurrenceDate: day("2026-01-10")},
		{Transaction: history[1], OccurrenceDate: day("2026-02-10")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if c := condition(t, draft, rules.FieldDescription); c.Operator != rules.OperatorEquals || c.Value != "fiber internet" {
		t.Fatalf("exact text: %+v", c)
	}
	if got := condition(t, draft, rules.FieldAmount).Value; got != "0.00000000000000000001:0" {
		t.Fatalf("amount precision lost: %s", got)
	}
	if got := condition(t, draft, rules.FieldDayOfMonth).Value; got != "10:10" {
		t.Fatal(got)
	}
}

func TestGenerateBroadEvidenceAndInconsistentCategories(t *testing.T) {
	draft, _ := fixture(t)
	evidence := append([]suggestions.Observation(nil), draft.Evidence...)
	evidence[0].Transaction.Description = ptr("PIX A")
	evidence[1].Transaction.Description = ptr("PIX B")
	evidence[2].Transaction.Description = ptr("PIX C")
	evidence[1].Transaction.Account.ID = "other"
	evidence[1].Transaction.Account.Name = ptr("Other")
	evidence[1].Transaction.Card = nil
	evidence[1].Transaction.InternalCategory = &transactions.InternalCategory{ID: "other", IsActive: true}
	got, err := suggestions.Generate(commitment(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got.Warnings, "broad_description") || !slices.Contains(got.Warnings, "unrestricted_account") || len(got.Rule.Actions) != 1 || len(got.Rule.Conditions) != 3 {
		t.Fatalf("unsupported specificity: %+v", got)
	}
	evidence[2].Transaction.Description = nil
	got, err = suggestions.Generate(commitment(), evidence)
	if err != nil || len(got.Rule.Conditions) != 2 {
		t.Fatalf("missing description: %+v, %v", got, err)
	}
}

func TestPreviewReportsFalsePositivesMultipleMatchesAndFailures(t *testing.T) {
	draft, history := fixture(t)
	duplicate := item("duplicate", "2026-01-11", "-101", "PIX Fiber Internet OTHER")
	unrelated := item("unrelated", "2026-02-11", "-101", "Groceries")
	wrongAccount := item("other-account", "2026-02-11", "-101", "PIX Fiber Internet OTHER")
	wrongAccount.Account.ID = "other"
	wrongDay := item("wrong-day", "2026-02-20", "-101", "PIX Fiber Internet OTHER")
	wrongCard := item("wrong-card", "2026-02-11", "-101", "PIX Fiber Internet OTHER")
	wrongCard.Card.Number = "5678"
	history = append(history, duplicate, unrelated, wrongAccount, wrongDay, wrongCard)
	result := preview(t, request(draft, history))
	if !result.NeedsReview || result.Transactions[0].Status != "ambiguous" || result.Transactions[3].Status != "ambiguous" {
		t.Fatalf("multiple matches not exposed: %+v", result)
	}
	if *result.Occurrences[0].TransactionID != "jan" {
		t.Fatal("earliest match precedence changed")
	}
	if !slices.Contains(result.Transactions[3].Reasons, "possible_false_positive") || !slices.Contains(result.Warnings, "multiple_matches_per_occurrence") {
		t.Fatal(result)
	}
	for _, index := range []int{4, 5, 6, 7} {
		if result.Transactions[index].Status != "unmatched" {
			t.Fatalf("unrelated transaction matched: %+v", result.Transactions[index])
		}
	}
	if result.Transactions[4].FailedConditions[0].Field != rules.FieldDescription || result.Transactions[6].FailedConditions[0].Field != rules.FieldDayOfMonth || result.Transactions[7].FailedConditions[0].Field != rules.FieldCard {
		t.Fatalf("missing explanations: %+v", result.Transactions)
	}
}

func TestPreviewExistingRulesAndScenarioCompetition(t *testing.T) {
	draft, history := fixture(t)
	existing := commitment()
	existing.ID = "existing"
	req := request(draft, history)
	req.Commitments = []recurrences.RecurringCommitment{existing}
	req.Rules = []automation.Rule{
		{ID: "ignore", IsActive: true, Conditions: draft.Rule.Conditions, LogicOperator: rules.LogicAnd, Actions: []automation.Action{{Type: automation.ActionIgnore}}},
		{ID: "reconcile", IsActive: true, Conditions: draft.Rule.Conditions, LogicOperator: rules.LogicAnd, Actions: []automation.Action{{Type: automation.ActionReconcile, ScenarioID: &existing.ID}}},
		{ID: "inactive", Conditions: draft.Rule.Conditions, LogicOperator: rules.LogicAnd},
		{ID: "reserved-target", Actions: []automation.Action{{Type: automation.ActionReconcile, ScenarioID: &draft.Commitment.ID}}},
	}
	result := preview(t, req)
	if !slices.Contains(result.Warnings, "target_already_has_rule:reserved-target") {
		t.Fatal(result.Warnings)
	}
	for _, row := range result.Transactions {
		if row.Status != "ambiguous" || !reflect.DeepEqual(row.RuleIDs, []string{"ignore", "reconcile"}) || !reflect.DeepEqual(row.ScenarioIDs, []string{"existing"}) {
			t.Fatalf("overlap silently accepted: %+v", row)
		}
	}
	// Same text/amount is not a scenario conflict when its account excludes it.
	req.Commitments[0].AccountID = ptr("different-account")
	result = preview(t, req)
	if len(result.Transactions[0].ScenarioIDs) != 0 {
		t.Fatal("existing scenario's account restriction ignored")
	}
}

func TestPreviewPreservesManualOverridesAndIsReadOnly(t *testing.T) {
	draft, history := fixture(t)
	req := request(draft, history)
	req.Overrides = []recurrences.Override{
		{ScenarioID: draft.Commitment.ID, OccurrenceDate: day("2026-01-10"), State: recurrences.StateDetached},
		{ScenarioID: draft.Commitment.ID, OccurrenceDate: day("2026-02-10"), State: recurrences.StateLinked, TransactionID: ptr("mar")},
	}
	before, _ := json.Marshal(req)
	first := preview(t, req)
	second := preview(t, req)
	after, _ := json.Marshal(req)
	if string(before) != string(after) || !reflect.DeepEqual(first, second) {
		t.Fatal("preview mutated inputs or changed across repeated reads")
	}
	if !first.Occurrences[0].Detached || first.Occurrences[0].Reconciled() || first.Occurrences[1].Origin != recurrences.OriginManual || *first.Occurrences[1].TransactionID != "mar" || first.Occurrences[2].Reconciled() {
		t.Fatalf("manual precedence changed: %+v", first.Occurrences)
	}
	for i, reason := range []string{"manual_detach", "manual_link_precedence", "manually_taken"} {
		if first.Transactions[i].Status != "unmatched" || !slices.Contains(first.Transactions[i].Reasons, reason) {
			t.Fatalf("manual decision hidden: %+v", first.Transactions[i])
		}
	}
	// An out-of-window manual claim by another scenario is still a conflict.
	req.Overrides = []recurrences.Override{{ScenarioID: "other", OccurrenceDate: day("2025-12-31"), State: recurrences.StateLinked, TransactionID: ptr("jan")}}
	result := preview(t, req)
	if result.Transactions[0].Status != "ambiguous" || !slices.Contains(result.Transactions[0].ScenarioIDs, "other") {
		t.Fatal("out-of-window claim missed")
	}
}

func TestPreviewCashEligibilityMatchesExistingReconciler(t *testing.T) {
	draft, history := fixture(t)
	ignored, transfer, invalid := history[0], history[1], history[2]
	ignored.Inclusion.State = money.Ignored
	ignored.TotalsEligibility = transactions.TotalsEligibility{Reason: ptr(money.ReasonIgnored)}
	transfer.TotalsEligibility = transactions.TotalsEligibility{Reason: ptr(money.ReasonTransferCategory)}
	invalid.EffectiveMoney = &transactions.EffectiveMoneyView{Value: "invalid", CurrencyCode: "BRL"}
	result := preview(t, request(draft, []transactions.Item{ignored, transfer, invalid}))
	if result.Transactions[0].Status != "unmatched" || result.Transactions[1].Status != "matched" || result.Transactions[2].Status != "unmatched" {
		t.Fatalf("cash eligibility drift: %+v", result.Transactions)
	}
	if result.Occurrences[0].Reconciled() || !result.Occurrences[1].Reconciled() || result.Occurrences[2].Reconciled() {
		t.Fatal(result.Occurrences)
	}
}

func TestGenerateFlagsCalendarBoundaryPatterns(t *testing.T) {
	for _, tc := range []struct {
		name, first, second, occurrence1, occurrence2 string
		anchor                                        int
	}{
		{"shifted_payment", "2026-01-31", "2026-03-01", "2026-02-01", "2026-03-01", 1},
		{"wrapped_days", "2026-01-31", "2026-02-01", "2026-01-01", "2026-02-01", 1},
		{"leap_month_end", "2028-01-31", "2028-02-29", "2028-01-31", "2028-02-29", 31},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := commitment()
			c.DayOfMonth = tc.anchor
			history := []transactions.Item{item("a", tc.first, "-100", "Fiber"), item("b", tc.second, "-100", "Fiber")}
			draft, err := suggestions.Generate(c, []suggestions.Observation{
				{Transaction: history[0], OccurrenceDate: day(tc.occurrence1)},
				{Transaction: history[1], OccurrenceDate: day(tc.occurrence2)},
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := slices.Contains(draft.Warnings, "cross_month_unsupported"); got != (tc.name != "leap_month_end") {
				t.Fatalf("calendar warning: %+v", draft)
			}
			req := request(draft, history)
			req.From, req.To = day(tc.occurrence1), day(tc.occurrence2)
			result := preview(t, req)
			if tc.name == "shifted_payment" && result.Occurrences[0].Reconciled() {
				t.Fatal("previous month automatically reconciled February")
			}
			if tc.name == "leap_month_end" && (!result.Occurrences[0].Reconciled() || !result.Occurrences[1].Reconciled() || result.NeedsReview) {
				t.Fatalf("month end clamping failed: %+v", result)
			}
		})
	}
}

func TestGenerateAndPreviewRejectInvalidInputs(t *testing.T) {
	draft, history := fixture(t)
	for _, mutate := range []func(*recurrences.RecurringCommitment, *[]suggestions.Observation){
		func(c *recurrences.RecurringCommitment, _ *[]suggestions.Observation) { c.ID = "" },
		func(c *recurrences.RecurringCommitment, _ *[]suggestions.Observation) { c.Amount = decimal.Zero },
		func(_ *recurrences.RecurringCommitment, e *[]suggestions.Observation) { *e = (*e)[:1] },
		func(_ *recurrences.RecurringCommitment, e *[]suggestions.Observation) { (*e)[1] = (*e)[0] },
		func(c *recurrences.RecurringCommitment, _ *[]suggestions.Observation) { c.AccountID = ptr("wrong") },
		func(_ *recurrences.RecurringCommitment, e *[]suggestions.Observation) {
			(*e)[0].Transaction.OccurredAt = nil
		},
		func(_ *recurrences.RecurringCommitment, e *[]suggestions.Observation) {
			(*e)[0].OccurrenceDate = day("2026-01-11")
		},
	} {
		c, evidence := commitment(), append([]suggestions.Observation(nil), draft.Evidence...)
		mutate(&c, &evidence)
		if _, err := suggestions.Generate(c, evidence); err == nil {
			t.Fatal("accepted invalid evidence")
		}
	}
	req := request(draft, history)
	req.To = req.From.AddDate(0, 0, -1)
	if _, err := suggestions.Preview(req); err == nil {
		t.Fatal("accepted reversed period")
	}
	req = request(draft, history)
	req.Draft.Rule.Actions = []automation.ActionWrite{{Type: automation.ActionReconcile, ScenarioID: ptr("wrong")}}
	if _, err := suggestions.Preview(req); err == nil {
		t.Fatal("accepted wrong target")
	}
}
