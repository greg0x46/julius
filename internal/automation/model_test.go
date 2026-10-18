package automation_test

import (
	"testing"

	"github.com/greg0x46/julius/internal/automation"
)

func baseWrite() automation.Write {
	return automation.Write{
		Name:          "Ignorar assinaturas",
		IsActive:      true,
		LogicOperator: automation.LogicAnd,
		Conditions: []automation.Condition{
			{Field: automation.FieldDescription, Operator: automation.OperatorContains, Value: "netflix"},
		},
		Actions: []automation.ActionWrite{{Type: automation.ActionIgnore}},
	}
}

func TestValidateRequiresAtLeastOneCondition(t *testing.T) {
	w := baseWrite()
	w.Conditions = nil
	if err := w.Validate(); err == nil {
		t.Error("expected an error when no condition is set")
	}
}

func TestValidateRejectsMismatchedFieldOperatorPairing(t *testing.T) {
	w := baseWrite()
	w.Conditions = []automation.Condition{
		{Field: automation.FieldDescription, Operator: automation.OperatorWithinPercent, Value: "1500.00:10"},
	}
	if err := w.Validate(); err == nil {
		t.Error("expected an error: description only supports contains/equals")
	}
}

func TestValidateRequiresAtLeastOneAction(t *testing.T) {
	w := baseWrite()
	w.Actions = nil
	if err := w.Validate(); err == nil {
		t.Error("expected an error: at least one action is required")
	}
}

func TestValidateAllowsMultipleActions(t *testing.T) {
	scenarioID := "scenario-1"
	categoryID := "category-1"
	w := baseWrite()
	w.Actions = []automation.ActionWrite{
		{Type: automation.ActionReconcile, ScenarioID: &scenarioID},
		{Type: automation.ActionSetCategory, CategoryID: &categoryID},
	}
	if err := w.Validate(); err != nil {
		t.Errorf("expected a rule to allow combining reconcile and set_category actions, got %v", err)
	}
}

func TestValidateSetCategoryActionRequiresCategoryID(t *testing.T) {
	w := baseWrite()
	w.Actions = []automation.ActionWrite{{Type: automation.ActionSetCategory}}
	if err := w.Validate(); err == nil {
		t.Error("expected an error: set_category action requires a category_id")
	}
}

func TestValidateIgnoreActionRejectsCategoryID(t *testing.T) {
	categoryID := "category-1"
	w := baseWrite()
	w.Actions = []automation.ActionWrite{{Type: automation.ActionIgnore, CategoryID: &categoryID}}
	if err := w.Validate(); err == nil {
		t.Error("expected an error: ignore action must not reference a category")
	}
}

func TestValidateReconcileActionRequiresScenarioID(t *testing.T) {
	w := baseWrite()
	w.Actions = []automation.ActionWrite{{Type: automation.ActionReconcile}}
	if err := w.Validate(); err == nil {
		t.Error("expected an error: reconcile action requires a scenario_id")
	}
}

func TestValidateIgnoreActionRejectsScenarioID(t *testing.T) {
	scenarioID := "scenario-1"
	w := baseWrite()
	w.Actions = []automation.ActionWrite{{Type: automation.ActionIgnore, ScenarioID: &scenarioID}}
	if err := w.Validate(); err == nil {
		t.Error("expected an error: ignore action must not reference a recurring scenario")
	}
}

func TestValidateRejectsUnknownActionType(t *testing.T) {
	w := baseWrite()
	w.Actions = []automation.ActionWrite{{Type: "archive"}}
	if err := w.Validate(); err == nil {
		t.Error("expected an error: unknown action type")
	}
}

func TestValidateAmountConditionAllowedWithoutReconcileAction(t *testing.T) {
	w := baseWrite()
	w.Conditions = []automation.Condition{
		{Field: automation.FieldAmount, Operator: automation.OperatorWithinPercent, Value: "1500.00:10"},
	}
	if err := w.Validate(); err != nil {
		t.Errorf("expected amount condition to be valid without a reconcile action, got %v", err)
	}
}

func TestValidateAmountConditionAllowedWithReconcileAction(t *testing.T) {
	scenarioID := "scenario-1"
	w := baseWrite()
	w.Actions = []automation.ActionWrite{{Type: automation.ActionReconcile, ScenarioID: &scenarioID}}
	w.Conditions = []automation.Condition{
		{Field: automation.FieldAmount, Operator: automation.OperatorWithinPercent, Value: "1500.00:10"},
		{Field: automation.FieldDayOfMonth, Operator: automation.OperatorDayRange, Value: "1:5"},
	}
	if err := w.Validate(); err != nil {
		t.Errorf("expected amount/day_of_month conditions to be valid with a reconcile action, got %v", err)
	}
}

func TestValidateRejectsMalformedAmountAndDayRangeValues(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cond  automation.Condition
		valid bool
	}{
		{"amount ok", automation.Condition{Field: automation.FieldAmount, Operator: automation.OperatorWithinPercent, Value: "1500.00:10"}, true},
		{"amount negative reference ok", automation.Condition{Field: automation.FieldAmount, Operator: automation.OperatorWithinPercent, Value: "-80:0"}, true},
		{"amount without tolerance", automation.Condition{Field: automation.FieldAmount, Operator: automation.OperatorWithinPercent, Value: "10"}, false},
		{"amount not a number", automation.Condition{Field: automation.FieldAmount, Operator: automation.OperatorWithinPercent, Value: "abc:10"}, false},
		{"amount NaN reference", automation.Condition{Field: automation.FieldAmount, Operator: automation.OperatorWithinPercent, Value: "NaN:10"}, false},
		{"amount zero reference", automation.Condition{Field: automation.FieldAmount, Operator: automation.OperatorWithinPercent, Value: "0:10"}, false},
		{"amount negative tolerance", automation.Condition{Field: automation.FieldAmount, Operator: automation.OperatorWithinPercent, Value: "100:-1"}, false},
		{"amount bad tolerance", automation.Condition{Field: automation.FieldAmount, Operator: automation.OperatorWithinPercent, Value: "100:x"}, false},
		{"day ok", automation.Condition{Field: automation.FieldDayOfMonth, Operator: automation.OperatorDayRange, Value: "1:31"}, true},
		{"day wrapping ok", automation.Condition{Field: automation.FieldDayOfMonth, Operator: automation.OperatorDayRange, Value: "28:5"}, true},
		{"day single value", automation.Condition{Field: automation.FieldDayOfMonth, Operator: automation.OperatorDayRange, Value: "5"}, false},
		{"day not a number", automation.Condition{Field: automation.FieldDayOfMonth, Operator: automation.OperatorDayRange, Value: "a:b"}, false},
		{"day zero", automation.Condition{Field: automation.FieldDayOfMonth, Operator: automation.OperatorDayRange, Value: "0:10"}, false},
		{"day above 31", automation.Condition{Field: automation.FieldDayOfMonth, Operator: automation.OperatorDayRange, Value: "1:32"}, false},
		{"day negative", automation.Condition{Field: automation.FieldDayOfMonth, Operator: automation.OperatorDayRange, Value: "-1:10"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := baseWrite()
			w.Conditions = []automation.Condition{tc.cond}
			err := w.Validate()
			if tc.valid && err != nil {
				t.Errorf("Validate() = %v, want valid", err)
			}
			if !tc.valid && err == nil {
				t.Errorf("Validate() = nil, want an error for %q", tc.cond.Value)
			}
		})
	}
}
