package recurrences_test

import (
	"testing"

	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/rules"
	"github.com/greg0x46/julius/internal/transactions"
)

func itemAt(t *testing.T, dateStr, amount string) transactions.Item {
	t.Helper()
	occurredAt := date(t, dateStr)
	return transactions.Item{
		ID:             "tx-" + dateStr,
		OccurredAt:     &occurredAt,
		EffectiveMoney: &transactions.EffectiveMoneyView{Value: amount, CurrencyCode: "BRL", Source: money.AccountCurrency},
	}
}

// baseConditions mirrors what a linked automation rule's conditions would
// be: a 1500 reference amount with 10% tolerance, and an absolute
// day-of-month window spanning day 28 through day 3 (wraps around day 31).
// Both carry their own reference directly — independent of any commitment.
func baseConditions() []rules.Condition {
	return []rules.Condition{
		{Field: rules.FieldAmount, Operator: rules.OperatorWithinPercent, Value: "1500.00:10"},
		{Field: rules.FieldDayOfMonth, Operator: rules.OperatorDayRange, Value: "28:3"},
	}
}

func TestResolveOccurrenceMatchesWithinToleranceAndDayWindow(t *testing.T) {
	candidates := []transactions.Item{itemAt(t, "2026-01-29", "1520.00")}

	matched, ok := recurrences.ResolveOccurrence(baseConditions(), rules.LogicAnd, candidates)
	if !ok {
		t.Fatal("expected a match within tolerance")
	}
	if matched.ID != "tx-2026-01-29" {
		t.Errorf("unexpected matched item: %+v", matched)
	}
}

func TestResolveOccurrenceRejectsAmountOutsideTolerance(t *testing.T) {
	candidates := []transactions.Item{itemAt(t, "2026-01-30", "2000.00")}

	if _, ok := recurrences.ResolveOccurrence(baseConditions(), rules.LogicAnd, candidates); ok {
		t.Error("expected no match: amount too far from expected")
	}
}

func TestResolveOccurrenceRejectsDayOutsideWindow(t *testing.T) {
	candidates := []transactions.Item{itemAt(t, "2026-01-10", "1500.00")}

	if _, ok := recurrences.ResolveOccurrence(baseConditions(), rules.LogicAnd, candidates); ok {
		t.Error("expected no match: day too far from expected")
	}
}

func TestResolveOccurrenceWithoutDayConditionMatchesRegardlessOfDay(t *testing.T) {
	conditions := []rules.Condition{
		{Field: rules.FieldAmount, Operator: rules.OperatorWithinPercent, Value: "1500.00:10"},
	}
	// Far from the 28:3 window — would fail baseConditions, but there's no
	// day_of_month condition here.
	candidates := []transactions.Item{itemAt(t, "2026-01-10", "1500.00")}

	if _, ok := recurrences.ResolveOccurrence(conditions, rules.LogicAnd, candidates); !ok {
		t.Error("expected a match: day is irrelevant when there's no day_of_month condition")
	}
}

func TestResolveOccurrenceNoCandidatesNeverMatches(t *testing.T) {
	if _, ok := recurrences.ResolveOccurrence(baseConditions(), rules.LogicAnd, nil); ok {
		t.Error("expected no match with zero candidates")
	}
}

func TestResolveOccurrenceUsesAbsoluteValueOfSignedAmount(t *testing.T) {
	candidates := []transactions.Item{itemAt(t, "2026-01-31", "-1500.00")}

	if _, ok := recurrences.ResolveOccurrence(baseConditions(), rules.LogicAnd, candidates); !ok {
		t.Error("expected a match: sign should not matter, only magnitude")
	}
}

func TestResolveOccurrencePicksEarliestMatchingCandidate(t *testing.T) {
	candidates := []transactions.Item{
		itemAt(t, "2026-02-01", "1500.00"),
		itemAt(t, "2026-01-30", "1500.00"),
	}

	matched, ok := recurrences.ResolveOccurrence(baseConditions(), rules.LogicAnd, candidates)
	if !ok {
		t.Fatal("expected a match")
	}
	if matched.ID != "tx-2026-01-30" {
		t.Errorf("expected the earliest matching candidate, got %s", matched.ID)
	}
}

func TestResolveOccurrenceMatchesSharedTextConditions(t *testing.T) {
	description, name, institution := "ALUGUEL TESTE", "Conta corrente", "Banco teste"
	item := itemAt(t, "2026-01-29", "-1500.00")
	item.Description = &description
	item.Account.Name, item.Account.Institution = &name, &institution
	item.Card = &transactions.CardInfo{Number: "**** 1234"}
	conditions := append(baseConditions(),
		rules.Condition{Field: rules.FieldDescription, Operator: rules.OperatorContains, Value: "aluguel"},
		rules.Condition{Field: rules.FieldAccount, Operator: rules.OperatorEquals, Value: "conta corrente"},
		rules.Condition{Field: rules.FieldAccount, Operator: rules.OperatorContains, Value: "banco"},
		rules.Condition{Field: rules.FieldCard, Operator: rules.OperatorContains, Value: "1234"},
	)
	if _, ok := recurrences.ResolveOccurrence(conditions, rules.LogicAnd, []transactions.Item{item}); !ok {
		t.Fatal("expected all money, date and text conditions to match")
	}
	for _, field := range []string{"description", "account name", "institution", "card", "missing card"} {
		t.Run(field, func(t *testing.T) {
			candidate := item
			other := "other"
			switch field {
			case "description":
				candidate.Description = &other
			case "account name":
				candidate.Account.Name = &other
			case "institution":
				candidate.Account.Institution = &other
			case "card":
				candidate.Card = &transactions.CardInfo{Number: "5678"}
			case "missing card":
				candidate.Card = nil
			}
			if _, ok := recurrences.ResolveOccurrence(conditions, rules.LogicAnd, []transactions.Item{candidate}); ok {
				t.Fatal("expected no match with an unsatisfied text condition")
			}
		})
	}
}
