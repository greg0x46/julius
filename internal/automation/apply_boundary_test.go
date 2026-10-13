package automation

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestApplyCandidatesNativeTimestamp(t *testing.T) {
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetMaxOpenConns(1)
	for _, query := range []string{
		`CREATE TABLE financial_accounts (id TEXT, name TEXT, institution TEXT, currency_code TEXT)`,
		`CREATE TABLE financial_transactions (id TEXT, account_id TEXT, description TEXT, credit_card_metadata TEXT, amount TEXT, amount_in_account_currency TEXT, currency_code TEXT, occurred_at TIMESTAMP, deleted_at TEXT)`,
		`CREATE TABLE automation_rules (id TEXT, name TEXT, is_active INTEGER, logic_operator TEXT, created_at TEXT, updated_at TEXT)`,
		`CREATE TABLE automation_rule_conditions (id TEXT, rule_id TEXT, field TEXT, operator TEXT, value TEXT, position INTEGER)`,
		`CREATE TABLE automation_rule_actions (id TEXT, rule_id TEXT, action_type TEXT, scenario_id TEXT, category_id TEXT, position INTEGER)`,
		`INSERT INTO financial_accounts VALUES ('account', 'Test', NULL, 'BRL')`,
		`INSERT INTO financial_transactions VALUES ('transaction', 'account', 'Expense', NULL, '-25.50', NULL, 'BRL', '2026-10-13 12:00:00', NULL)`,
	} {
		if _, err := conn.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	var raw any
	if err := conn.QueryRow(`SELECT occurred_at FROM financial_transactions`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw.(time.Time); !ok {
		t.Fatalf("driver timestamp = %T; want time.Time", raw)
	}
	candidate, err := candidateFor(context.Background(), conn, "transaction")
	if err != nil {
		t.Fatal(err)
	}
	if candidate.DayOfMonth == nil || *candidate.DayOfMonth != 13 {
		t.Fatalf("day = %v; want 13", candidate.DayOfMonth)
	}
	if candidate.Amount == nil || candidate.Amount.String() != "25.5" {
		t.Fatalf("amount = %v; want 25.5", candidate.Amount)
	}
	rule, err := Create(context.Background(), conn, Write{
		Name: "Different day", IsActive: true, LogicOperator: LogicAnd,
		Conditions: []Condition{{Field: FieldDayOfMonth, Operator: OperatorDayRange, Value: "14:14"}},
		Actions:    []ActionWrite{{Type: ActionIgnore}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := ApplyRetroactively(context.Background(), conn, rule.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != (RetroactiveResult{}) {
		t.Fatalf("retroactive result = %+v; want no matches", result)
	}
}
