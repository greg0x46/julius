package suggestions_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/automation"
	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/projections"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/recurrences/suggestions"
	"github.com/greg0x46/julius/internal/rules"
	"github.com/greg0x46/julius/internal/transactions"
	"github.com/shopspring/decimal"
)

func TestSuggestionApprovalBothDialects(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
		category, err := categories.Create(ctx, conn, "Synthetic", money.Expense, "home", "#112233")
		if err != nil {
			t.Fatal(err)
		}
		account, err := transactions.WriteManualAccount(ctx, conn, "", transactions.ManualAccountInput{Name: "Synthetic", CurrencyCode: "BRL"}, true)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"Internet", "Insurance", "Music"} {
			for month := 0; month < 6; month++ {
				if _, err := transactions.CreateManual(ctx, conn, transactions.ManualInput{AccountID: account, Description: name, Amount: decimal.RequireFromString("-100.1234"), OccurredAt: dates.AddMonths(time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), month)}); err != nil {
					t.Fatal(err)
				}
			}
		}
		candidates, err := suggestions.List(ctx, conn, now)
		if err != nil || len(candidates) != 3 {
			t.Fatalf("list: %v %v", candidates, err)
		}
		writeFor := func(c recurrences.Suggestion) recurrences.Write {
			d := c.Draft
			return recurrences.Write{Name: d.Name, Kind: d.Kind, Amount: d.Amount, CategoryID: category.ID, AccountID: d.AccountID, Cadence: d.Cadence, DayOfMonth: d.DayOfMonth, MonthOfYear: d.MonthOfYear, StartDate: d.StartDate, IsActive: true}
		}
		review := func(c recurrences.Suggestion) suggestions.Review {
			r, err := suggestions.ReviewCandidate(ctx, conn, now, suggestions.Key(c), writeFor(c), nil)
			if err != nil {
				t.Fatal(err)
			}
			return r
		}
		before, err := transactions.History(ctx, conn)
		if err != nil {
			t.Fatal(err)
		}
		c := candidates[0]
		r := review(c)
		again := review(c)
		if r.Token != again.Token {
			t.Fatal("unstable token")
		}
		bad := writeFor(c)
		bad.CategoryID = ""
		if _, err := suggestions.ReviewCandidate(ctx, conn, now, suggestions.Key(c), bad, nil); !errors.Is(err, suggestions.ErrInvalid) {
			t.Fatalf("missing category: %v", err)
		}
		inactive := false
		if _, err := categories.Update(ctx, conn, category.ID, nil, &inactive, nil, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := suggestions.Accept(ctx, conn, now, suggestions.Key(c), writeFor(c), r.Draft.Rule.Conditions, r.Token, true); !errors.Is(err, suggestions.ErrInvalid) {
			t.Fatalf("inactive: %v", err)
		}
		active := true
		if _, err := categories.Update(ctx, conn, category.ID, nil, &active, nil, nil); err != nil {
			t.Fatal(err)
		}
		// Failure after scenario creation must roll back the scenario and schedule.
		trigger := `CREATE TRIGGER fail_suggested_rule BEFORE INSERT ON automation_rules BEGIN SELECT RAISE(ABORT, 'synthetic failure'); END`
		drop := `DROP TRIGGER fail_suggested_rule`
		if dbtest.IsPostgres(conn) {
			if _, err := conn.Exec(`CREATE FUNCTION fail_suggestion() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic failure'; END $$`); err != nil {
				t.Fatal(err)
			}
			trigger = `CREATE TRIGGER fail_suggested_rule BEFORE INSERT ON automation_rules FOR EACH ROW EXECUTE FUNCTION fail_suggestion()`
			drop = `DROP TRIGGER fail_suggested_rule ON automation_rules`
		}
		if _, err := conn.Exec(trigger); err != nil {
			t.Fatal(err)
		}
		if _, err := suggestions.Accept(ctx, conn, now, suggestions.Key(c), writeFor(c), r.Draft.Rule.Conditions, r.Token, true); err == nil {
			t.Fatal("expected rollback")
		}
		for _, table := range []string{"scenarios", "scenario_recurring_schedules", "automation_rules", "recurrence_suggestion_decisions"} {
			var count int
			if err := conn.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
				t.Fatalf("orphan %s: %d %v", table, count, err)
			}
		}
		if _, err := conn.Exec(drop); err != nil {
			t.Fatal(err)
		}

		type outcome struct {
			id  string
			err error
		}
		outcomes := make(chan outcome, 2)
		for range 2 {
			go func() {
				id, err := suggestions.Accept(ctx, conn, now, suggestions.Key(c), writeFor(c), r.Draft.Rule.Conditions, r.Token, true)
				outcomes <- outcome{id, err}
			}()
		}
		id := ""
		for range 2 {
			result := <-outcomes
			if result.err == nil {
				if id != "" && id != result.id {
					t.Fatal("concurrent duplicate")
				}
				id = result.id
			}
		}
		if id == "" {
			t.Fatal("no concurrent approval succeeded")
		}

		retry, err := suggestions.Accept(ctx, conn, now, suggestions.Key(c), writeFor(c), r.Draft.Rule.Conditions, r.Token, true)
		if err != nil || retry != id {
			t.Fatalf("retry: %s %v", retry, err)
		}
		targets, err := automation.ListActiveReconcileTargets(ctx, conn)
		if err != nil || len(targets) != 1 {
			t.Fatalf("targets: %v %v", targets, err)
		}
		rule, err := automation.Get(ctx, conn, targets[id].ID)
		if err != nil || len(rule.Actions) != 1 || rule.Actions[0].Type != automation.ActionReconcile {
			t.Fatalf("unexpected category action: %+v %v", rule, err)
		}
		committed, err := recurrences.Get(ctx, conn, id)
		if err != nil || len(committed.Occurrences(now, dates.AddMonths(now, 1))) != 1 {
			t.Fatalf("schedule: %+v %v", committed, err)
		}
		duplicate := writeFor(candidates[1])
		duplicate.Name = c.Draft.Name
		duplicate.AccountID = nil
		if _, err := suggestions.ReviewCandidate(ctx, conn, now, suggestions.Key(candidates[1]), duplicate, nil); !errors.Is(err, suggestions.ErrDuplicate) {
			t.Fatalf("edited duplicate: %v", err)
		}
		events, err := projections.List(ctx, conn, projections.ProjectionQuery{From: now, To: dates.AddMonths(now, 1), Selection: projections.SelectionActive})
		if err != nil || len(events) != 1 || events[0].ScenarioID != id || events[0].Realized {
			t.Fatalf("projection: %+v %v", events, err)
		}
		// Unrelated approvals do not invalidate each other's previews (bulk semantics).
		second := candidates[1]
		r2 := review(second)
		third := candidates[2]
		r3 := review(third)
		if _, err := suggestions.Accept(ctx, conn, now, suggestions.Key(second), writeFor(second), r2.Draft.Rule.Conditions, r2.Token, true); err != nil {
			t.Fatal(err)
		}
		if err := suggestions.Dismiss(ctx, conn, now, suggestions.Key(third)); err != nil {
			t.Fatal(err)
		}
		if _, err := suggestions.Accept(ctx, conn, now, suggestions.Key(third), writeFor(third), r3.Draft.Rule.Conditions, r3.Token, true); !errors.Is(err, suggestions.ErrStale) {
			t.Fatalf("dismissed: %v", err)
		}
		after, err := transactions.History(ctx, conn)
		if err != nil {
			t.Fatal(err)
		}
		index := func(items []transactions.Item) map[string]transactions.Item {
			m := map[string]transactions.Item{}
			for _, i := range items {
				m[i.ID] = i
			}
			return m
		}
		if !reflect.DeepEqual(index(before), index(after)) {
			t.Fatal("ledger decisions changed")
		}
		// New real movements reconcile through the ordinary projection service.
		occurrence := committed.StartDate
		if _, err := transactions.CreateManual(ctx, conn, transactions.ManualInput{AccountID: account, Description: committed.Name, Amount: committed.Amount.Neg(), OccurredAt: occurrence}); err != nil {
			t.Fatal(err)
		}
		query := projections.ProjectionQuery{From: occurrence, To: occurrence, Selection: projections.SelectionExplicit, IDs: []string{id}}
		events, err = projections.List(ctx, conn, query)
		if err != nil || len(events) != 1 || !events[0].Realized {
			t.Fatalf("accepted rule reconciliation: %+v %v", events, err)
		}
		if _, err := recurrences.PutOverride(ctx, conn, id, occurrence, recurrences.StateDetached, nil); err != nil {
			t.Fatal(err)
		}
		events, err = projections.List(ctx, conn, query)
		if err != nil || len(events) != 1 || events[0].Realized || !events[0].Detached {
			t.Fatalf("manual precedence: %+v %v", events, err)
		}
		if _, err := transactions.CreateManual(ctx, conn, transactions.ManualInput{AccountID: account, Description: third.Draft.Name, Amount: third.Draft.Amount.Neg(), OccurredAt: occurrence}); err != nil {
			t.Fatal(err)
		}
		candidates, err = suggestions.List(ctx, conn, dates.AddMonths(now, 1))
		if err != nil || len(candidates) != 0 {
			t.Fatalf("remaining: %v %v", candidates, err)
		}
	})
}

func TestSuggestionStaleAndAmbiguousBothDialects(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
		category, err := categories.Create(ctx, conn, "Synthetic", money.Expense, "home", "#112233")
		if err != nil {
			t.Fatal(err)
		}
		account, err := transactions.WriteManualAccount(ctx, conn, "", transactions.ManualAccountInput{Name: "Synthetic", CurrencyCode: "BRL"}, true)
		if err != nil {
			t.Fatal(err)
		}
		for month := 0; month < 6; month++ {
			if _, err := transactions.CreateManual(ctx, conn, transactions.ManualInput{AccountID: account, Description: "Internet", Amount: decimal.NewFromInt(-100), OccurredAt: dates.AddMonths(time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), month)}); err != nil {
				t.Fatal(err)
			}
		}
		candidates, err := suggestions.List(ctx, conn, now)
		if err != nil || len(candidates) != 1 {
			t.Fatal(err, candidates)
		}
		c := candidates[0]
		d := c.Draft
		write := recurrences.Write{Name: d.Name, Kind: d.Kind, Amount: d.Amount, CategoryID: category.ID, AccountID: d.AccountID, Cadence: d.Cadence, DayOfMonth: d.DayOfMonth, StartDate: d.StartDate, IsActive: true}
		r, err := suggestions.ReviewCandidate(ctx, conn, now, suggestions.Key(c), write, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := transactions.CreateManual(ctx, conn, transactions.ManualInput{AccountID: account, Description: "Other subscription", Amount: decimal.NewFromInt(-100), OccurredAt: time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)}); err != nil {
			t.Fatal(err)
		}
		if _, err := suggestions.Accept(ctx, conn, now, suggestions.Key(c), write, r.Draft.Rule.Conditions, r.Token, true); !errors.Is(err, suggestions.ErrStale) {
			t.Fatalf("stale history: %v", err)
		}
		fresh, err := suggestions.ReviewCandidate(ctx, conn, now, suggestions.Key(c), write, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := automation.Create(ctx, conn, automation.Write{Name: "Competing rule", IsActive: true, LogicOperator: rules.LogicAnd, Conditions: []rules.Condition{{Field: rules.FieldDescription, Operator: rules.OperatorEquals, Value: "Internet"}}, Actions: []automation.ActionWrite{{Type: automation.ActionIgnore}}}); err != nil {
			t.Fatal(err)
		}
		if _, err := suggestions.Accept(ctx, conn, now, suggestions.Key(c), write, fresh.Draft.Rule.Conditions, fresh.Token, true); !errors.Is(err, suggestions.ErrStale) {
			t.Fatalf("stale rule preview: %v", err)
		}
		broad := []rules.Condition{{Field: rules.FieldAmount, Operator: rules.OperatorWithinPercent, Value: "100:0"}}
		r, err = suggestions.ReviewCandidate(ctx, conn, now, suggestions.Key(c), write, broad)
		if err != nil || !r.Preview.NeedsReview {
			t.Fatalf("ambiguous: %+v %v", r, err)
		}
		if _, err := suggestions.Accept(ctx, conn, now, suggestions.Key(c), write, broad, r.Token, false); !errors.Is(err, suggestions.ErrReview) {
			t.Fatalf("unacknowledged: %v", err)
		}
		wrap := []rules.Condition{{Field: rules.FieldDayOfMonth, Operator: rules.OperatorDayRange, Value: "28:5"}}
		r, err = suggestions.ReviewCandidate(ctx, conn, now, suggestions.Key(c), write, wrap)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := suggestions.Accept(ctx, conn, now, suggestions.Key(c), write, wrap, r.Token, true); !errors.Is(err, suggestions.ErrReview) {
			t.Fatalf("unsupported: %v", err)
		}
	})
}
