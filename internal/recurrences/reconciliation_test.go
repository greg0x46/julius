package recurrences_test

import (
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/rules"
	"github.com/greg0x46/julius/internal/transactions"
)

// categorizedItem adds category, account and inclusion metadata to itemAt.
func categorizedItem(t *testing.T, dateStr, amount, categoryID, accountID string) transactions.Item {
	t.Helper()
	item := itemAt(t, dateStr, amount)
	item.InternalCategory = &transactions.InternalCategory{ID: categoryID, Name: "Aluguel"}
	item.Account = transactions.AccountSummary{ID: accountID}
	item.Classification = money.Outflow
	item.Inclusion = transactions.Inclusion{State: money.Considered}
	return item
}

func monthlyCommitment(t *testing.T, categoryID string) recurrences.RecurringCommitment {
	t.Helper()
	return recurrences.RecurringCommitment{
		ID: "commitment-1", Name: "Aluguel", Kind: recurrences.KindExpense,
		Amount: dec(t, "1500.00"), CategoryID: categoryID,
		Cadence: recurrences.CadenceMonthly, DayOfMonth: 1,
		StartDate: date(t, "2026-01-01"), IsActive: true,
	}
}

func linkedOverride(t *testing.T, occurrence, transactionID string) recurrences.Override {
	t.Helper()
	id := transactionID
	return recurrences.Override{
		ID: "override-" + occurrence, ScenarioID: "scenario-1",
		OccurrenceDate: date(t, occurrence), State: recurrences.StateLinked, TransactionID: &id,
	}
}

func detachedOverride(t *testing.T, occurrence string) recurrences.Override {
	t.Helper()
	return recurrences.Override{
		ID: "override-" + occurrence, ScenarioID: "scenario-1",
		OccurrenceDate: date(t, occurrence), State: recurrences.StateDetached,
	}
}

func TestReconcilerFallsBackToTheRuleWithoutOverrides(t *testing.T) {
	commitment := monthlyCommitment(t, "cat-1")
	candidates := []transactions.Item{categorizedItem(t, "2026-01-30", "1500.00", "cat-1", "acct-1")}

	reconciler := recurrences.NewReconciler(commitment, baseConditions(), rules.LogicAnd, true, nil, candidates)
	resolved := reconciler.Resolve(recurrences.Occurrence{Date: date(t, "2026-01-01"), ExpectedAmount: dec(t, "1500.00")})

	if !resolved.Reconciled() {
		t.Fatal("expected the rule to reconcile the occurrence")
	}
	if resolved.Origin != recurrences.OriginRule {
		t.Errorf("origin = %q, want %q", resolved.Origin, recurrences.OriginRule)
	}
	if resolved.Transaction == nil || resolved.Transaction.ID != "tx-2026-01-30" {
		t.Errorf("unexpected matched transaction: %+v", resolved.Transaction)
	}
}

// The whole reason a detached override is persisted rather than a link
// merely deleted: the rule is re-evaluated from scratch on every read, so
// anything less than a stored decision would let it re-claim the occurrence.
func TestReconcilerDetachedOverrideBeatsTheRule(t *testing.T) {
	commitment := monthlyCommitment(t, "cat-1")
	candidates := []transactions.Item{categorizedItem(t, "2026-01-30", "1500.00", "cat-1", "acct-1")}

	reconciler := recurrences.NewReconciler(commitment, baseConditions(), rules.LogicAnd, true,
		[]recurrences.Override{detachedOverride(t, "2026-01-01")}, candidates)
	resolved := reconciler.Resolve(recurrences.Occurrence{Date: date(t, "2026-01-01"), ExpectedAmount: dec(t, "1500.00")})

	if resolved.Reconciled() {
		t.Fatalf("expected the detached override to win, got %+v", resolved)
	}
	if !resolved.Detached {
		t.Error("expected Detached to be reported so the UI can offer 'voltar ao automático'")
	}
}

func TestReconcilerManualOverrideBeatsTheRule(t *testing.T) {
	commitment := monthlyCommitment(t, "cat-1")
	ruleMatch := categorizedItem(t, "2026-01-30", "1500.00", "cat-1", "acct-1")
	handPicked := categorizedItem(t, "2026-01-15", "1490.00", "cat-1", "acct-1")

	reconciler := recurrences.NewReconciler(commitment, baseConditions(), rules.LogicAnd, true,
		[]recurrences.Override{linkedOverride(t, "2026-01-01", handPicked.ID)},
		[]transactions.Item{ruleMatch, handPicked})
	resolved := reconciler.Resolve(recurrences.Occurrence{Date: date(t, "2026-01-01"), ExpectedAmount: dec(t, "1500.00")})

	if resolved.Origin != recurrences.OriginManual {
		t.Fatalf("origin = %q, want %q", resolved.Origin, recurrences.OriginManual)
	}
	if resolved.TransactionID == nil || *resolved.TransactionID != handPicked.ID {
		t.Errorf("reconciled with %v, want the hand-picked %s", resolved.TransactionID, handPicked.ID)
	}
	if resolved.Transaction == nil {
		t.Error("expected the item to be attached when the caller supplied it")
	}
}

// A manual link resolves even when the caller never loaded the transaction —
// which is exactly the Timeline's situation, since it only needs the boolean.
func TestReconcilerManualOverrideResolvesWithoutTheItemLoaded(t *testing.T) {
	commitment := monthlyCommitment(t, "cat-1")

	reconciler := recurrences.NewReconciler(commitment, nil, rules.LogicAnd, false,
		[]recurrences.Override{linkedOverride(t, "2026-01-01", "tx-elsewhere")}, nil)
	resolved := reconciler.Resolve(recurrences.Occurrence{Date: date(t, "2026-01-01"), ExpectedAmount: dec(t, "1500.00")})

	if !resolved.Reconciled() {
		t.Fatal("expected a manual link to reconcile without the item present")
	}
	if resolved.Transaction != nil {
		t.Error("expected no item attached when the caller supplied none")
	}
}

// Without this, the same money would suppress two projections: hand-linked to
// January, still pattern-matched into February.
func TestReconcilerRuleSkipsTransactionsAlreadyLinkedByHand(t *testing.T) {
	commitment := monthlyCommitment(t, "cat-1")
	shared := categorizedItem(t, "2026-02-01", "1500.00", "cat-1", "acct-1")

	reconciler := recurrences.NewReconciler(commitment, baseConditions(), rules.LogicAnd, true,
		[]recurrences.Override{linkedOverride(t, "2026-01-01", shared.ID)},
		[]transactions.Item{shared})
	resolved := reconciler.Resolve(recurrences.Occurrence{Date: date(t, "2026-02-01"), ExpectedAmount: dec(t, "1500.00")})

	if resolved.Reconciled() {
		t.Fatalf("February must not reuse the transaction January already claims, got %+v", resolved)
	}
}

func TestReconcilerWithoutRuleNeverReconciles(t *testing.T) {
	commitment := monthlyCommitment(t, "cat-1")
	candidates := []transactions.Item{categorizedItem(t, "2026-01-30", "1500.00", "cat-1", "acct-1")}

	reconciler := recurrences.NewReconciler(commitment, nil, rules.LogicAnd, false, nil, candidates)
	resolved := reconciler.Resolve(recurrences.Occurrence{Date: date(t, "2026-01-01"), ExpectedAmount: dec(t, "1500.00")})

	if resolved.Reconciled() {
		t.Error("a commitment with no rule and no override must never reconcile")
	}
}

func TestResolveRangeCoversEveryScheduledOccurrence(t *testing.T) {
	commitment := monthlyCommitment(t, "cat-1")

	reconciler := recurrences.NewReconciler(commitment, nil, rules.LogicAnd, false, nil, nil)
	resolved := reconciler.ResolveRange(date(t, "2026-01-01"), date(t, "2026-03-31"))

	if len(resolved) != 3 {
		t.Fatalf("got %d occurrences, want 3", len(resolved))
	}
	for i, want := range []string{"2026-01-01", "2026-02-01", "2026-03-01"} {
		if got := resolved[i].Occurrence.Date.Format("2006-01-02"); got != want {
			t.Errorf("occurrence %d = %s, want %s", i, got, want)
		}
	}
}

func TestCandidatesByMonthScopesToAccountAndMonth(t *testing.T) {
	commitment := monthlyCommitment(t, "cat-1")
	accountID := "acct-1"
	commitment.AccountID = &accountID

	uncategorized := categorizedItem(t, "2026-01-08", "1500.00", "cat-1", "acct-1")
	uncategorized.InternalCategory = nil
	undated := uncategorized
	undated.OccurredAt = nil
	byMonth := recurrences.CandidatesByMonth(commitment, []transactions.Item{
		categorizedItem(t, "2026-01-05", "1500.00", "cat-1", "acct-1"),
		categorizedItem(t, "2026-01-06", "1500.00", "cat-2", "acct-1"), // other category
		categorizedItem(t, "2026-01-07", "1500.00", "cat-1", "acct-9"), // other account
		categorizedItem(t, "2026-02-05", "1500.00", "cat-1", "acct-1"),
		uncategorized,
		undated,
	})

	january := byMonth[recurrences.MonthKey(date(t, "2026-01-01"))]
	if len(january) != 3 || january[0].ID != "tx-2026-01-05" || january[1].ID != "tx-2026-01-06" || january[2].ID != uncategorized.ID {
		t.Errorf("january candidates = %+v, want same-account transactions regardless of category", january)
	}
	if len(byMonth[recurrences.MonthKey(date(t, "2026-02-01"))]) != 1 {
		t.Error("expected February's own bucket")
	}
}

// The year in the key is what stops September 2027 reconciling September 2026.
func TestMonthKeyCarriesTheYear(t *testing.T) {
	if recurrences.MonthKey(date(t, "2026-09-15")).Equal(recurrences.MonthKey(date(t, "2027-09-15"))) {
		t.Error("same month in different years must not share a bucket")
	}
	want := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	if got := recurrences.MonthKey(date(t, "2026-09-15")); !got.Equal(want) {
		t.Errorf("MonthKey = %v, want %v", got, want)
	}
}

func TestReconcilerMatchesRegardlessOfCategory(t *testing.T) {
	for _, categoryID := range []string{"", "cat-2"} {
		t.Run("category="+categoryID, func(t *testing.T) {
			commitment := monthlyCommitment(t, "cat-1")
			item := itemAt(t, "2026-01-30", "1500.00")
			if categoryID != "" {
				item.InternalCategory = &transactions.InternalCategory{ID: categoryID}
			}
			reconciler := recurrences.NewReconciler(commitment, baseConditions(), rules.LogicAnd, true, nil, []transactions.Item{item})
			resolved := reconciler.ResolveRange(date(t, "2026-01-01"), date(t, "2026-02-28"))
			if len(resolved) != 2 {
				t.Fatalf("got %d occurrences, want 2", len(resolved))
			}
			if !resolved[0].Reconciled() || resolved[0].Origin != recurrences.OriginRule || *resolved[0].TransactionID != item.ID {
				t.Fatalf("January = %+v, want automatic reconciliation with %s", resolved[0], item.ID)
			}
			if resolved[1].Reconciled() {
				t.Fatalf("February must not reuse January's transaction: %+v", resolved[1])
			}
		})
	}
}
