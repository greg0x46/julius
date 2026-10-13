package transactions_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/categories"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/transactions"
)

func TestQueryEligibilityAndTotals(t *testing.T) {
	f := newFixture(t)
	acc := f.addAccount(account{Name: strp("Conta Corrente"), CurrencyCode: strp("BRL")})

	occurred := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	outflowID := f.addTransaction(txn{
		AccountID: acc, Description: strp("Mercado XYZ"), Amount: strp("-50.00"),
		AmountInAccountCurrency: strp("-50.00"), CurrencyCode: strp("BRL"),
		OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT"),
	})
	f.setCategory(outflowID, categorySupermercado)

	inflowID := f.addTransaction(txn{
		AccountID: acc, Description: strp("Salario"), Amount: strp("1000.00"),
		AmountInAccountCurrency: strp("1000.00"), CurrencyCode: strp("BRL"),
		OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("CREDIT"),
	})
	f.setCategory(inflowID, categorySalario)

	// Uncategorized: having no category never gates totals eligibility, so
	// this still counts toward the outflow total. The transfer kind is the
	// one exception where a category does gate — see
	// TestQueryExcludesTransferCategoryFromTotals.
	uncategorizedID := f.addTransaction(txn{
		AccountID: acc, Description: strp("Sem categoria"), Amount: strp("-5.00"),
		AmountInAccountCurrency: strp("-5.00"), CurrencyCode: strp("BRL"),
		OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT"),
	})

	result, err := transactions.Query(context.Background(), f.conn, transactions.QueryRequest{
		Timezone: "UTC", GroupBy: money.GroupNone, Page: 1, PageSize: 50,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if result.StoredTotal != 3 {
		t.Errorf("StoredTotal = %d, want 3", result.StoredTotal)
	}
	if len(result.Items) != 3 {
		t.Fatalf("len(Items) = %d, want 3", len(result.Items))
	}
	if len(result.Totals) != 1 {
		t.Fatalf("len(Totals) = %d, want 1", len(result.Totals))
	}
	totals := result.Totals[0]
	if totals.CurrencyCode != "BRL" || totals.Inflow != "1000.00" || totals.Outflow != "55.00" || totals.Balance != "945.00" {
		t.Errorf("Totals = %+v", totals)
	}

	for _, item := range result.Items {
		switch item.ID {
		case outflowID, uncategorizedID:
			if !item.TotalsEligibility.Included || item.Classification != money.Outflow {
				t.Errorf("outflow item %s: included=%v classification=%v", item.ID, item.TotalsEligibility.Included, item.Classification)
			}
		case inflowID:
			if !item.TotalsEligibility.Included || item.Classification != money.Inflow {
				t.Errorf("inflow item: included=%v classification=%v", item.TotalsEligibility.Included, item.Classification)
			}
		}
	}
}

func TestQueryFilterByAccount(t *testing.T) {
	f := newFixture(t)
	accA := f.addAccount(account{CurrencyCode: strp("BRL")})
	accB := f.addAccount(account{CurrencyCode: strp("BRL")})
	occurred := time.Now().UTC()

	idA := f.addTransaction(txn{AccountID: accA, Amount: strp("-10.00"), AmountInAccountCurrency: strp("-10.00"), CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT")})
	f.addTransaction(txn{AccountID: accB, Amount: strp("-20.00"), AmountInAccountCurrency: strp("-20.00"), CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT")})

	result, err := transactions.Query(context.Background(), f.conn, transactions.QueryRequest{
		Timezone: "UTC", GroupBy: money.GroupNone, Page: 1, PageSize: 50,
		Filters: transactions.Filters{AccountIDs: []string{accA}},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != idA {
		t.Errorf("expected only transaction %s, got %+v", idA, result.Items)
	}

	// Several accounts are an OR: a transaction from any of them stays.
	result, err = transactions.Query(context.Background(), f.conn, transactions.QueryRequest{
		Timezone: "UTC", GroupBy: money.GroupNone, Page: 1, PageSize: 50,
		Filters: transactions.Filters{AccountIDs: []string{accA, accB}},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result.Items) != 2 {
		t.Errorf("expected both accounts' transactions, got %+v", result.Items)
	}
}

func TestQueryExposesAndFiltersSourceProvider(t *testing.T) {
	f := newFixture(t)
	f.exec(`UPDATE data_sources SET provider = 'file' WHERE id = ?`, f.sourceID)
	acc := f.addAccount(account{CurrencyCode: strp("BRL")})
	id := f.addTransaction(txn{AccountID: acc, Amount: strp("-10.00"), CurrencyCode: strp("BRL")})
	file := "file"
	result, err := transactions.Query(context.Background(), f.conn, transactions.QueryRequest{
		Timezone: "UTC", GroupBy: money.GroupNone, Page: 1, PageSize: 50,
		Filters: transactions.Filters{SourceProvider: &file},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != id || result.Items[0].SourceProvider == nil || *result.Items[0].SourceProvider != "file" {
		t.Fatalf("file items = %+v", result.Items)
	}
	pluggy := "pluggy"
	result, err = transactions.Query(context.Background(), f.conn, transactions.QueryRequest{
		Timezone: "UTC", GroupBy: money.GroupNone, Page: 1, PageSize: 50,
		Filters: transactions.Filters{SourceProvider: &pluggy},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Page.TotalItems != 0 {
		t.Fatalf("pluggy filter returned %d file items", result.Page.TotalItems)
	}
}

func TestQueryFilterByCategoriesAndStatuses(t *testing.T) {
	f := newFixture(t)
	acc := f.addAccount(account{CurrencyCode: strp("BRL")})
	occurred := time.Now().UTC()

	market := f.addTransaction(txn{AccountID: acc, Amount: strp("-30.00"), AmountInAccountCurrency: strp("-30.00"), CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT")})
	f.setCategory(market, categorySupermercado)
	salary := f.addTransaction(txn{AccountID: acc, Amount: strp("1000.00"), AmountInAccountCurrency: strp("1000.00"), CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("PENDING"), MovementType: strp("CREDIT")})
	f.setCategory(salary, categorySalario)
	// Uncategorized, so it never matches a category set.
	f.addTransaction(txn{AccountID: acc, Amount: strp("-15.00"), AmountInAccountCurrency: strp("-15.00"), CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT")})

	query := func(filters transactions.Filters) []string {
		t.Helper()
		result, err := transactions.Query(context.Background(), f.conn, transactions.QueryRequest{
			Timezone: "UTC", GroupBy: money.GroupNone, Page: 1, PageSize: 50, Filters: filters,
		})
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		ids := make([]string, 0, len(result.Items))
		for _, item := range result.Items {
			ids = append(ids, item.ID)
		}
		return ids
	}

	if got := query(transactions.Filters{CategoryIDs: []string{categorySupermercado, categorySalario}}); len(got) != 2 {
		t.Errorf("two categories: got %v, want both categorized transactions", got)
	}
	if got := query(transactions.Filters{ProviderStatuses: []string{"POSTED", "PENDING"}}); len(got) != 3 {
		t.Errorf("both statuses: got %v, want all three", got)
	}
	// Across fields the filter is still an AND.
	got := query(transactions.Filters{CategoryIDs: []string{categorySupermercado, categorySalario}, ProviderStatuses: []string{"PENDING"}})
	if len(got) != 1 || got[0] != salary {
		t.Errorf("categories AND status: got %v, want only %s", got, salary)
	}
}

func TestSpendingByCategory(t *testing.T) {
	f := newFixture(t)
	acc := f.addAccount(account{CurrencyCode: strp("BRL")})
	occurred := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	outsideRange := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)

	market1 := f.addTransaction(txn{AccountID: acc, Amount: strp("-30.00"), AmountInAccountCurrency: strp("-30.00"), CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT")})
	f.setCategory(market1, categorySupermercado)
	market2 := f.addTransaction(txn{AccountID: acc, Amount: strp("-20.00"), AmountInAccountCurrency: strp("-20.00"), CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT")})
	f.setCategory(market2, categorySupermercado)

	f.addTransaction(txn{AccountID: acc, Amount: strp("-15.00"), AmountInAccountCurrency: strp("-15.00"), CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT")})

	// Inflow must not count toward spending, even if categorized.
	salary := f.addTransaction(txn{AccountID: acc, Amount: strp("1000.00"), AmountInAccountCurrency: strp("1000.00"), CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("CREDIT")})
	f.setCategory(salary, categorySalario)

	// Out of the requested date range: must not count.
	beforeRange := f.addTransaction(txn{AccountID: acc, Amount: strp("-999.00"), AmountInAccountCurrency: strp("-999.00"), CurrencyCode: strp("BRL"), OccurredAt: &outsideRange, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT")})
	f.setCategory(beforeRange, categorySupermercado)

	from := money.Date{Year: 2026, Month: 3, Day: 1}
	to := money.Date{Year: 2026, Month: 3, Day: 31}
	items, err := transactions.SpendingByCategory(context.Background(), f.conn, transactions.Filters{
		DateFrom: &from, DateTo: &to,
	}, "UTC")
	if err != nil {
		t.Fatalf("SpendingByCategory: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2: %+v", len(items), items)
	}
	if items[0].CategoryID == nil || *items[0].CategoryID != categorySupermercado || items[0].Amount != "50.00" {
		t.Errorf("items[0] = %+v, want category %s amount 50.00", items[0], categorySupermercado)
	}
	if items[1].CategoryID != nil || items[1].CategoryName != "Sem categoria" || items[1].Amount != "15.00" {
		t.Errorf("items[1] = %+v, want uncategorized amount 15.00", items[1])
	}
}

func TestQueryFilterUncategorized(t *testing.T) {
	f := newFixture(t)
	acc := f.addAccount(account{CurrencyCode: strp("BRL")})
	occurred := time.Now().UTC()

	categorized := f.addTransaction(txn{AccountID: acc, Amount: strp("-1.00"), AmountInAccountCurrency: strp("-1.00"), CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT")})
	f.setCategory(categorized, categorySupermercado)
	uncategorized := f.addTransaction(txn{AccountID: acc, Amount: strp("-2.00"), AmountInAccountCurrency: strp("-2.00"), CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT")})

	uncategorizedFilter := true
	result, err := transactions.Query(context.Background(), f.conn, transactions.QueryRequest{
		Timezone: "UTC", GroupBy: money.GroupNone, Page: 1, PageSize: 50,
		Filters: transactions.Filters{Uncategorized: uncategorizedFilter},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != uncategorized {
		t.Errorf("expected only uncategorized transaction %s, got %+v", uncategorized, result.Items)
	}
}

func TestQueryExposesCardInfo(t *testing.T) {
	f := newFixture(t)
	acc := f.addAccount(account{CurrencyCode: strp("BRL")})
	occurred := time.Now().UTC()

	plain := f.addTransaction(txn{
		AccountID: acc, Amount: strp("-10.00"), AmountInAccountCurrency: strp("-10.00"),
		CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"),
		MovementType: strp("DEBIT"), CreditCardMetadata: strp(`{"cardNumber":"**** 4321"}`),
	})
	installment := f.addTransaction(txn{
		AccountID: acc, Amount: strp("-20.00"), AmountInAccountCurrency: strp("-20.00"),
		CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"),
		MovementType:       strp("DEBIT"),
		CreditCardMetadata: strp(`{"cardNumber":"**** 1234","installmentNumber":3,"totalInstallments":12}`),
	})
	noCard := f.addTransaction(txn{
		AccountID: acc, Amount: strp("-30.00"), AmountInAccountCurrency: strp("-30.00"),
		CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"),
		MovementType: strp("DEBIT"),
	})

	result, err := transactions.Query(context.Background(), f.conn, transactions.QueryRequest{
		Timezone: "UTC", GroupBy: money.GroupNone, Page: 1, PageSize: 50,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	byID := make(map[string]transactions.Item, len(result.Items))
	for _, item := range result.Items {
		byID[item.ID] = item
	}

	plainItem := byID[plain]
	if plainItem.Card == nil || plainItem.Card.Number != "**** 4321" {
		t.Errorf("plain item Card = %+v, want Number **** 4321", plainItem.Card)
	}
	if plainItem.Card != nil && (plainItem.Card.InstallmentNumber != nil || plainItem.Card.TotalInstallments != nil) {
		t.Errorf("plain item Card = %+v, want no installment info", plainItem.Card)
	}

	installmentItem := byID[installment]
	if installmentItem.Card == nil || installmentItem.Card.Number != "**** 1234" {
		t.Fatalf("installment item Card = %+v, want Number **** 1234", installmentItem.Card)
	}
	if installmentItem.Card.InstallmentNumber == nil || *installmentItem.Card.InstallmentNumber != 3 {
		t.Errorf("installment item Card.InstallmentNumber = %v, want 3", installmentItem.Card.InstallmentNumber)
	}
	if installmentItem.Card.TotalInstallments == nil || *installmentItem.Card.TotalInstallments != 12 {
		t.Errorf("installment item Card.TotalInstallments = %v, want 12", installmentItem.Card.TotalInstallments)
	}

	if byID[noCard].Card != nil {
		t.Errorf("no-card item Card = %+v, want nil", byID[noCard].Card)
	}
}

func TestQueryAmountRangeFilter(t *testing.T) {
	f := newFixture(t)
	acc := f.addAccount(account{CurrencyCode: strp("BRL")})
	occurred := time.Now().UTC()

	small := f.addTransaction(txn{AccountID: acc, Amount: strp("-5.00"), AmountInAccountCurrency: strp("-5.00"), CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT")})
	f.addTransaction(txn{AccountID: acc, Amount: strp("-500.00"), AmountInAccountCurrency: strp("-500.00"), CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT")})

	max := mustDecimal(t, "100.00")
	result, err := transactions.Query(context.Background(), f.conn, transactions.QueryRequest{
		Timezone: "UTC", GroupBy: money.GroupNone, Page: 1, PageSize: 50,
		Filters: transactions.Filters{AmountMax: max},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != small {
		t.Errorf("expected only the %s transaction, got %+v", small, result.Items)
	}
}

func TestQueryPaginationOrdersNewestFirst(t *testing.T) {
	f := newFixture(t)
	acc := f.addAccount(account{CurrencyCode: strp("BRL")})

	var ids []string
	for i := 0; i < 5; i++ {
		occurred := time.Date(2026, 1, i+1, 0, 0, 0, 0, time.UTC)
		id := f.addTransaction(txn{
			AccountID: acc, Amount: strp("-1.00"), AmountInAccountCurrency: strp("-1.00"),
			CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT"),
		})
		ids = append(ids, id)
	}

	result, err := transactions.Query(context.Background(), f.conn, transactions.QueryRequest{
		Timezone: "UTC", GroupBy: money.GroupNone, Page: 1, PageSize: 2,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if result.Page.TotalItems != 5 || result.Page.TotalPages != 3 {
		t.Errorf("Page = %+v, want TotalItems=5 TotalPages=3", result.Page)
	}
	if len(result.Items) != 2 {
		t.Fatalf("len(Items) = %d, want 2", len(result.Items))
	}
	// Newest occurred_at (ids[4]) must come first.
	if result.Items[0].ID != ids[4] || result.Items[1].ID != ids[3] {
		t.Errorf("Items = [%s, %s], want [%s, %s]", result.Items[0].ID, result.Items[1].ID, ids[4], ids[3])
	}
}

func mustDecimal(t *testing.T, s string) *decimal.Decimal {
	t.Helper()
	d, err := decimal.NewFromString(s)
	if err != nil {
		t.Fatal(err)
	}
	return &d
}

// TestQueryExcludesTransferCategoryFromTotals covers the double-counting this
// rule exists to stop: the same money leaving one tracked account and landing
// on another would otherwise show up as both an expense and an income of the
// full amount. Both legs must drop out of the totals while staying in the
// list — the money really did move, and hiding the rows would misrepresent
// the ledger.
func TestQueryExcludesTransferCategoryFromTotals(t *testing.T) {
	f := newFixture(t)
	origin := f.addAccount(account{CurrencyCode: strp("BRL")})
	destination := f.addAccount(account{CurrencyCode: strp("BRL")})
	occurred := time.Now().UTC()

	groceries := f.addTransaction(txn{
		AccountID: origin, Description: strp("Mercado"), Amount: strp("-30.00"),
		AmountInAccountCurrency: strp("-30.00"), CurrencyCode: strp("BRL"),
		OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT"),
	})
	f.setCategory(groceries, categorySupermercado)

	sent := f.addTransaction(txn{
		AccountID: origin, Description: strp("Transferencia enviada"), Amount: strp("-500.00"),
		AmountInAccountCurrency: strp("-500.00"), CurrencyCode: strp("BRL"),
		OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT"),
	})
	f.setCategory(sent, categoryTransferencia)

	received := f.addTransaction(txn{
		AccountID: destination, Description: strp("Transferencia recebida"), Amount: strp("500.00"),
		AmountInAccountCurrency: strp("500.00"), CurrencyCode: strp("BRL"),
		OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("CREDIT"),
	})
	f.setCategory(received, categoryTransferencia)

	result, err := transactions.Query(context.Background(), f.conn, transactions.QueryRequest{
		Timezone: "UTC", GroupBy: money.GroupNone, Page: 1, PageSize: 50,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	if len(result.Items) != 3 {
		t.Fatalf("len(Items) = %d, want 3 — transfers stay in the ledger", len(result.Items))
	}
	if len(result.Totals) != 1 {
		t.Fatalf("len(Totals) = %d, want 1", len(result.Totals))
	}
	totals := result.Totals[0]
	if totals.Inflow != "0" || totals.Outflow != "30.00" || totals.Balance != "-30.00" {
		t.Errorf("Totals = %+v, want only the groceries outflow", totals)
	}

	for _, item := range result.Items {
		switch item.ID {
		case sent, received:
			if item.TotalsEligibility.Included {
				t.Errorf("transfer %s should be excluded from totals", item.ID)
			}
			if item.TotalsEligibility.Reason == nil || *item.TotalsEligibility.Reason != money.ReasonTransferCategory {
				t.Errorf("transfer %s: reason = %v, want %v", item.ID, item.TotalsEligibility.Reason, money.ReasonTransferCategory)
			}
			// The exclusion is a categorization outcome, not an inclusion
			// decision — the inclusion state must stay untouched, or the
			// user loses the distinction between "ignored" and "transfer".
			if item.Inclusion.State != money.Considered {
				t.Errorf("transfer %s: inclusion state = %v, want considered", item.ID, item.Inclusion.State)
			}
		case groceries:
			if !item.TotalsEligibility.Included {
				t.Errorf("groceries should still count toward totals")
			}
		}
	}
}

// TestQueryExcludesAutomaticSamePersonTransferFromTotals runs the provider's
// own label through the automatic mapping and out the other end of Query:
// money the user moved between two of their own banks must stop counting as
// income (both legs, so the pair nets to zero) without disappearing from the
// ledger — MovesCash is what keeps it in the balance reconstruction that
// timeline and net worth build on (see
// timeline.TestBuildSeriesKeepsTransferCategorizedCashMovement).
func TestQueryExcludesAutomaticSamePersonTransferFromTotals(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	origin := f.addAccount(account{CurrencyCode: strp("BRL")})
	destination := f.addAccount(account{CurrencyCode: strp("BRL")})
	occurred := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	label := categories.SamePersonTransferLabel

	salary := f.addTransaction(txn{
		AccountID: destination, Description: strp("Salario"), Amount: strp("100.00"),
		AmountInAccountCurrency: strp("100.00"), CurrencyCode: strp("BRL"),
		OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("CREDIT"),
	})
	f.setCategory(salary, categorySalario)

	sent := f.addTransaction(txn{
		AccountID: origin, Description: strp("TED Itau -> Nubank"), Amount: strp("-1500.00"),
		AmountInAccountCurrency: strp("-1500.00"), CurrencyCode: strp("BRL"),
		OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT"),
		SourceCategory: &label,
	})
	received := f.addTransaction(txn{
		AccountID: destination, Description: strp("TED recebida"), Amount: strp("1500.00"),
		AmountInAccountCurrency: strp("1500.00"), CurrencyCode: strp("BRL"),
		OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("CREDIT"),
		SourceCategory: &label,
	})
	for _, id := range []string{sent, received} {
		if err := categories.ApplyAutomatic(ctx, f.conn, id, &label); err != nil {
			t.Fatalf("ApplyAutomatic(%s): %v", id, err)
		}
	}

	result, err := transactions.Query(ctx, f.conn, transactions.QueryRequest{
		Timezone: "UTC", GroupBy: money.GroupNone, Page: 1, PageSize: 50,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	if len(result.Items) != 3 {
		t.Fatalf("len(Items) = %d, want 3 — the transfer legs stay in the ledger", len(result.Items))
	}
	totals := result.Totals[0]
	if totals.Inflow != "100.00" || totals.Outflow != "0" {
		t.Errorf("Totals = %+v, want only the salary — the transfer is the user's own money", totals)
	}

	for _, item := range result.Items {
		switch item.ID {
		case sent, received:
			if item.TotalsEligibility.Included {
				t.Errorf("transfer leg %s should be excluded from totals", item.ID)
			}
			if item.TotalsEligibility.Reason == nil || *item.TotalsEligibility.Reason != money.ReasonTransferCategory {
				t.Errorf("transfer leg %s: reason = %v, want %v", item.ID, item.TotalsEligibility.Reason, money.ReasonTransferCategory)
			}
			if !item.TotalsEligibility.MovesCash() {
				t.Errorf("transfer leg %s: MovesCash() = false — the money really did change accounts", item.ID)
			}
		case salary:
			if !item.TotalsEligibility.Included {
				t.Errorf("salary should still count toward totals")
			}
		}
	}
}

// TestSpendingByCategoryExcludesTransfers keeps transfers out of the
// financial report's spending breakdown, which reads the same eligibility.
func TestSpendingByCategoryExcludesTransfers(t *testing.T) {
	f := newFixture(t)
	acc := f.addAccount(account{CurrencyCode: strp("BRL")})
	occurred := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

	groceries := f.addTransaction(txn{AccountID: acc, Amount: strp("-30.00"), AmountInAccountCurrency: strp("-30.00"), CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT")})
	f.setCategory(groceries, categorySupermercado)
	transfer := f.addTransaction(txn{AccountID: acc, Amount: strp("-500.00"), AmountInAccountCurrency: strp("-500.00"), CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT")})
	f.setCategory(transfer, categoryTransferencia)

	from := money.Date{Year: 2026, Month: time.March, Day: 1}
	to := money.Date{Year: 2026, Month: time.March, Day: 31}
	items, err := transactions.SpendingByCategory(context.Background(), f.conn, transactions.Filters{DateFrom: &from, DateTo: &to}, "UTC")
	if err != nil {
		t.Fatalf("SpendingByCategory: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1 — the transfer must not appear as spending: %+v", len(items), items)
	}
	if items[0].Amount != "30.00" {
		t.Errorf("Amount = %s, want 30.00", items[0].Amount)
	}
}

func TestCategoryBreakdownInflowEligibility(t *testing.T) {
	f := newFixture(t)
	acc := f.addAccount(account{CurrencyCode: strp("BRL")})
	occurred := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	add := func(amount, category string) string {
		id := f.addTransaction(txn{AccountID: acc, Amount: strp(amount), AmountInAccountCurrency: strp(amount), CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("CREDIT")})
		f.setCategory(id, category)
		return id
	}
	add("100.00", categorySalario)
	add("25.00", categorySalario)
	add("500.00", categoryTransferencia)
	ignored := add("900.00", categorySalario)
	if _, err := transactions.SetInclusion(context.Background(), f.conn, ignored, money.Ignored, transactions.InclusionOriginManual, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	from, to := money.Date{Year: 2026, Month: 3, Day: 1}, money.Date{Year: 2026, Month: 3, Day: 31}
	items, err := transactions.CategoryBreakdown(context.Background(), f.conn, transactions.Filters{DateFrom: &from, DateTo: &to}, "UTC", money.Inflow)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].CategoryID == nil || *items[0].CategoryID != categorySalario || items[0].Amount != "125.00" {
		t.Fatalf("unexpected inflow categories: %+v", items)
	}
}

func TestQueryCreditCardFilter(t *testing.T) {
	f := newFixture(t)
	occurred := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	var cardAccount string
	for _, kind := range []string{"CREDIT", "BANK", ""} {
		acc := f.addAccount(account{CurrencyCode: strp("BRL"), AccountType: strp(kind)})
		if kind == "CREDIT" {
			cardAccount = acc
		}
		f.addTransaction(txn{AccountID: acc, Amount: strp("-25.00"), AmountInAccountCurrency: strp("-25.00"), CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT")})
	}
	for _, enabled := range []bool{true, false} {
		result, err := transactions.Query(context.Background(), f.conn, transactions.QueryRequest{Timezone: "UTC", GroupBy: money.GroupNone, Page: 1, PageSize: 50, Filters: transactions.Filters{CreditCard: enabled}})
		if err != nil {
			t.Fatal(err)
		}
		want := 3
		if enabled {
			want = 1
		}
		if len(result.Items) != want {
			t.Fatalf("credit_card=%v: got %d items, want %d", enabled, len(result.Items), want)
		}
		if enabled && (len(result.Totals) != 1 || result.Totals[0].Outflow != "25.00") {
			t.Fatalf("unexpected filtered totals: %+v", result.Totals)
		}
	}
	result, err := transactions.Query(context.Background(), f.conn, transactions.QueryRequest{Timezone: "UTC", GroupBy: money.GroupNone, Page: 1, PageSize: 50, Filters: transactions.Filters{CreditCard: true, AccountIDs: []string{cardAccount}}})
	if err != nil || len(result.Items) != 1 {
		t.Fatalf("combined account filter: %+v, %v", result, err)
	}
}

func TestQueryTypedValuesBothDialects(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		f := newFixtureWithConn(t, conn)
		accountID := f.addAccount(account{CurrencyCode: strp("BRL")})
		occurred := time.Date(2026, 3, 15, 23, 30, 0, 123456000, time.FixedZone("sample", -3*60*60))
		amount := "9007199254740993.01"
		id := f.addTransaction(txn{AccountID: accountID, Amount: &amount, AmountInAccountCurrency: &amount,
			CurrencyCode: strp("BRL"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("CREDIT")})
		f.setCategory(id, categorySalario)
		f.exec(`INSERT INTO transaction_inclusion_decisions
   (transaction_id, state, revision, changed_at, origin) VALUES (?, 'considered', 1, ?, 'manual')`,
			id, occurred.UTC().Format("2006-01-02T15:04:05.000000000Z"))
		nullID := f.addTransaction(txn{AccountID: accountID})
		result, err := transactions.Query(context.Background(), conn, transactions.QueryRequest{
			Timezone: "UTC", GroupBy: money.GroupNone, Page: 1, PageSize: 50,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Items) != 2 {
			t.Fatalf("items = %d, want 2", len(result.Items))
		}
		for _, item := range result.Items {
			switch item.ID {
			case id:
				if item.Amount == nil || *item.Amount != amount || item.AmountInAccountCurrency == nil || *item.AmountInAccountCurrency != amount {
					t.Fatalf("exact amounts lost: %+v", item)
				}
				if item.OccurredAt == nil || !item.OccurredAt.Equal(occurred) || item.OccurredAt.Location() != time.UTC {
					t.Fatalf("occurred_at = %v, want UTC instant %v", item.OccurredAt, occurred.UTC())
				}
				if item.Inclusion.ChangedAt == nil || !item.Inclusion.ChangedAt.Equal(occurred) {
					t.Fatalf("inclusion changed_at = %v", item.Inclusion.ChangedAt)
				}
				if item.InternalCategory == nil || item.InternalCategory.ChangedAt.IsZero() || !item.InternalCategory.IsActive {
					t.Fatalf("category decision = %+v", item.InternalCategory)
				}
			case nullID:
				if item.Amount != nil || item.AmountInAccountCurrency != nil || item.OccurredAt != nil || item.Inclusion.ChangedAt != nil || item.InternalCategory != nil {
					t.Fatalf("SQL NULL values lost: %+v", item)
				}
			default:
				t.Fatalf("unexpected transaction %s", item.ID)
			}
		}
		if len(result.Totals) != 1 || result.Totals[0].Inflow != amount || result.Totals[0].Balance != amount {
			t.Fatalf("exact totals lost: %+v", result.Totals)
		}
	})
}
