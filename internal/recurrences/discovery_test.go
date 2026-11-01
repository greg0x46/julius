package recurrences_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/transactions"
)

func discoveryHistory(t *testing.T, months []int, amounts []string) []transactions.Item {
	t.Helper()
	items := make([]transactions.Item, len(months))
	for i, m := range months {
		day := dates.AddMonths(date(t, "2024-01-31"), m)
		description, status := "  Utility * HOME  ", "POSTED"
		items[i] = transactions.Item{
			ID: fmt.Sprintf("tx-%02d", i), OccurredAt: &day, Description: &description,
			Account: transactions.AccountSummary{ID: "account"}, ProviderStatus: &status,
			Classification: money.Outflow, Inclusion: transactions.Inclusion{State: money.Considered},
			EffectiveMoney:    &transactions.EffectiveMoneyView{Value: amounts[i%len(amounts)], CurrencyCode: "BRL"},
			TotalsEligibility: transactions.TotalsEligibility{Included: true},
		}
	}
	return items
}

func TestDetectRecurringPatterns(t *testing.T) {
	tests := []struct {
		name       string
		months     []int
		amounts    []string
		count      int
		cadence    recurrences.Cadence
		amountKind string
	}{
		{"fixed monthly", []int{0, 1, 2, 3, 4, 5}, []string{"-100"}, 1, recurrences.CadenceMonthly, "fixed"},
		{"variable monthly", []int{0, 1, 2, 3}, []string{"-100", "-120", "-110", "-130"}, 1, recurrences.CadenceMonthly, "variable"},
		{"missing month", []int{0, 1, 3, 4}, []string{"-100"}, 1, recurrences.CadenceMonthly, "fixed"},
		{"too many missing", []int{0, 2, 4, 6}, []string{"-100"}, 0, "", ""},
		{"insufficient monthly", []int{0, 1, 2}, []string{"-100"}, 0, "", ""},
		{"annual", []int{0, 12, 24}, []string{"-100"}, 1, recurrences.CadenceAnnual, "fixed"},
		{"insufficient annual", []int{0, 12}, []string{"-100"}, 0, "", ""},
		{"missing annual", []int{0, 12, 36}, []string{"-100"}, 0, "", ""},
		{"frequent merchant", []int{0, 0, 1, 1, 2, 2, 3, 3}, []string{"-100", "-110"}, 0, "", ""},
		{"wild amounts", []int{0, 1, 2, 3}, []string{"-100", "-500"}, 0, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := discoveryHistory(t, tt.months, tt.amounts)
			asOf := *items[len(items)-1].OccurredAt
			got := recurrences.Detect(items, nil, asOf)
			if len(got) != tt.count {
				t.Fatalf("got %+v", got)
			}
			if len(got) == 0 {
				return
			}
			s := got[0]
			if s.Draft.Cadence != tt.cadence || s.AmountKind != tt.amountKind || s.Draft.DayOfMonth != 31 || !s.Draft.Amount.IsPositive() || len(s.TransactionIDs) != len(items) || !s.Draft.StartDate.After(asOf) {
				t.Fatalf("unexpected suggestion: %+v", s)
			}
			if tt.amountKind == "variable" && (!s.Draft.Amount.Equal(dec(t, "115")) || !s.MinimumAmount.Equal(dec(t, "100")) || !s.MaximumAmount.Equal(dec(t, "130")) || s.Confidence != "medium") {
				t.Fatalf("variable estimate: %+v", s)
			}
			if tt.cadence == recurrences.CadenceAnnual && (s.Draft.MonthOfYear == nil || *s.Draft.MonthOfYear != 1) {
				t.Fatal("annual month missing")
			}
		})
	}
}

func TestDiscoveryExclusions(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*transactions.Item)
	}{
		{"ignored", func(i *transactions.Item) { i.Inclusion.State = money.Ignored }},
		{"ineligible", func(i *transactions.Item) { i.TotalsEligibility.Included = false }},
		{"foreign currency", func(i *transactions.Item) { i.EffectiveMoney.CurrencyCode = "USD" }},
		{"missing money", func(i *transactions.Item) { i.EffectiveMoney = nil }},
		{"pending", func(i *transactions.Item) { s := "PENDING"; i.ProviderStatus = &s }},
		{"installment metadata", func(i *transactions.Item) { n := 12; i.Card = &transactions.CardInfo{TotalInstallments: &n} }},
		{"installment description", func(i *transactions.Item) { s := "Store 3/12"; i.Description = &s }},
		{"partial investment principal", func(i *transactions.Item) { i.InvestmentTransferAmount = "1" }},
		{"unclassified", func(i *transactions.Item) { i.Classification = money.Unclassified }},
	} {
		t.Run(test.name, func(t *testing.T) {
			items := discoveryHistory(t, []int{0, 1, 2, 3}, []string{"-100"})
			for n := range items {
				test.mutate(&items[n])
			}
			if got := recurrences.Detect(items, nil, date(t, "2024-05-01")); len(got) != 0 {
				t.Fatalf("unexpected %+v", got)
			}
		})
	}
	items := discoveryHistory(t, []int{0, 1, 2, 3}, []string{"-100"})
	if len(recurrences.Detect(items, nil, date(t, "2025-01-01"))) != 0 {
		t.Fatal("stale pattern suggested")
	}
	for i := range items {
		day := time.Date(2024, time.Month(i+1), 5+i*6, 0, 0, 0, 0, time.UTC)
		items[i].OccurredAt = &day
	}
	if len(recurrences.Detect(items, nil, date(t, "2024-05-01"))) != 0 {
		t.Fatal("irregular dates suggested")
	}
}

func TestDiscoveryNormalizationDuplicatesAndDeterminism(t *testing.T) {
	items := discoveryHistory(t, []int{0, 1, 2, 3, 4, 5}, []string{"-100.000000000000000001"})
	for i := range items {
		s := []string{"utility home", "UTILITY * HOME 2024-02-29", " Utility   HOME "}[i%3]
		items[i].Description = &s
	}
	asOf := date(t, "2024-07-01")
	want := recurrences.Detect(items, nil, asOf)
	if len(want) != 1 || want[0].Confidence != "high" || !want[0].Draft.Amount.Equal(dec(t, "100.000000000000000001")) {
		t.Fatalf("got %+v", want)
	}
	duplicate := items[0]
	duplicate.ID = "zz-duplicate"
	items = append(items, duplicate)
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	if got := recurrences.Detect(items, nil, asOf); !reflect.DeepEqual(got, want) {
		t.Fatalf("non-deterministic or duplicate evidence: %+v", got)
	}
	onlyDuplicates := []transactions.Item{items[0], items[0], items[0], items[0]}
	if len(recurrences.Detect(onlyDuplicates, nil, asOf)) != 0 {
		t.Fatal("duplicates fabricated history")
	}
}

func TestDiscoverySeparatesContextAndSuppressesExisting(t *testing.T) {
	items := discoveryHistory(t, []int{0, 1, 2, 3}, []string{"-100"})
	for i := range items {
		if i%2 == 0 {
			items[i].Account.ID = "other"
		}
	}
	asOf := date(t, "2024-05-01")
	if len(recurrences.Detect(items, nil, asOf)) != 0 {
		t.Fatal("accounts merged")
	}
	for i := range items {
		items[i].Account.ID = "account"
		items[i].Card = &transactions.CardInfo{Number: fmt.Sprint(i % 2)}
	}
	if len(recurrences.Detect(items, nil, asOf)) != 0 {
		t.Fatal("cards merged")
	}
	for i := range items {
		items[i].Card = nil
		if i%2 == 0 {
			items[i].Classification = money.Inflow
		}
	}
	if len(recurrences.Detect(items, nil, asOf)) != 0 {
		t.Fatal("directions merged")
	}
	for i := range items {
		items[i].Classification = money.Inflow
		items[i].InternalCategory = &transactions.InternalCategory{ID: "category", IsActive: true}
	}
	got := recurrences.Detect(items, nil, asOf)
	if len(got) != 1 || got[0].Draft.Kind != recurrences.KindIncome || got[0].Draft.CategoryID != "category" {
		t.Fatalf("income draft: %+v", got)
	}
	existing := got[0].Draft
	existing.IsActive = false
	if len(recurrences.Detect(items, []recurrences.RecurringCommitment{existing}, asOf)) != 0 {
		t.Fatal("paused recurrence suggested again")
	}
	existing.Name = "Owner renamed it"
	if len(recurrences.Detect(items, []recurrences.RecurringCommitment{existing}, asOf)) != 0 {
		t.Fatal("equivalent recurrence suggested again")
	}
}
