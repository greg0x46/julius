package httpapi

import "github.com/greg0x46/julius/internal/rules"

// conditionDTO is the wire shape of a rules.Condition, shared by
// automation_handlers.go and recurrences_handlers.go — both now compose
// reconciliation/matching rules from the same internal/rules primitives
// (see docs/development/architecture.md (Domain engines)).
type conditionDTO struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

func toConditionDTOs(conditions []rules.Condition) []conditionDTO {
	dtos := make([]conditionDTO, len(conditions))
	for i, c := range conditions {
		dtos[i] = conditionDTO{Field: string(c.Field), Operator: string(c.Operator), Value: c.Value}
	}
	return dtos
}

// conditionsFromDTOs converts and validates dtos, rejecting an empty Value
// or any field/operator pairing allowed doesn't recognize.
func conditionsFromDTOs(dtos []conditionDTO, allowed func(field, operator string) bool) ([]rules.Condition, bool) {
	conditions := make([]rules.Condition, len(dtos))
	for i, d := range dtos {
		if d.Value == "" || !allowed(d.Field, d.Operator) {
			return nil, false
		}
		conditions[i] = rules.Condition{Field: rules.ConditionField(d.Field), Operator: rules.ConditionOperator(d.Operator), Value: d.Value}
	}
	return conditions, true
}
