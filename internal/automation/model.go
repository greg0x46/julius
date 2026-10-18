package automation

import (
	"fmt"
	"strings"

	"github.com/greg0x46/julius/internal/rules"
)

// Validate checks a Write for structural correctness: at least one action —
// a rule may carry several (e.g. set_category + reconcile together), any
// combination is allowed — each action's target reference matching its
// type, and every condition's field/operator pairing. amount/day_of_month
// conditions are allowed regardless of action type: each carries its own
// reference value and tolerance/range directly in Condition.Value,
// independent of any linked commitment's own amount/day_of_month.
func (w Write) Validate() error {
	if strings.TrimSpace(w.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if w.LogicOperator != LogicAnd && w.LogicOperator != LogicOr {
		return fmt.Errorf("unknown logic operator %q", w.LogicOperator)
	}
	if len(w.Conditions) == 0 {
		return fmt.Errorf("at least one condition is required")
	}
	if len(w.Actions) == 0 {
		return fmt.Errorf("at least one action is required")
	}

	for _, action := range w.Actions {
		switch action.Type {
		case ActionIgnore:
			if action.ScenarioID != nil || action.CategoryID != nil {
				return fmt.Errorf("ignore action must not reference a target")
			}
		case ActionReconcile:
			if action.ScenarioID == nil || strings.TrimSpace(*action.ScenarioID) == "" {
				return fmt.Errorf("reconcile action requires a scenario_id")
			}
			if action.CategoryID != nil {
				return fmt.Errorf("reconcile action must not reference a category")
			}
		case ActionSetCategory:
			if action.CategoryID == nil || strings.TrimSpace(*action.CategoryID) == "" {
				return fmt.Errorf("set_category action requires a category_id")
			}
			if action.ScenarioID != nil {
				return fmt.Errorf("set_category action must not reference a recurring scenario")
			}
		default:
			return fmt.Errorf("unknown action type %q", action.Type)
		}
	}

	for _, c := range w.Conditions {
		if err := validateCondition(c); err != nil {
			return err
		}
	}
	return nil
}

func validateCondition(c Condition) error {
	switch c.Field {
	case FieldDescription, FieldCard, FieldAccount:
		if c.Operator != OperatorContains && c.Operator != OperatorEquals {
			return fmt.Errorf("field %q only accepts contains/equals, got %q", c.Field, c.Operator)
		}
	case FieldAmount:
		if c.Operator != OperatorWithinPercent {
			return fmt.Errorf("field %q only accepts within_percent, got %q", c.Field, c.Operator)
		}
	case FieldDayOfMonth:
		if c.Operator != OperatorDayRange {
			return fmt.Errorf("field %q only accepts day_range, got %q", c.Field, c.Operator)
		}
	default:
		return fmt.Errorf("unknown condition field %q", c.Field)
	}
	if strings.TrimSpace(c.Value) == "" {
		return fmt.Errorf("condition value is required")
	}
	switch c.Field {
	case FieldAmount:
		_, tolerance, ok := rules.ParseWithinPercent(c.Value)
		if !ok || tolerance.IsNegative() {
			return fmt.Errorf("amount condition value must be <non-zero amount>:<tolerance percent >= 0>, got %q", c.Value)
		}
	case FieldDayOfMonth:
		// min > max is valid: the range wraps across the turn of the month.
		first, last, ok := rules.ParseDayRange(c.Value)
		if !ok || first < 1 || first > 31 || last < 1 || last > 31 {
			return fmt.Errorf("day_of_month condition value must be <day 1-31>:<day 1-31>, got %q", c.Value)
		}
	}
	return nil
}
