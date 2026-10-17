package transactions_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/reporting"
	"github.com/greg0x46/julius/internal/timeline"
	"github.com/greg0x46/julius/internal/transactions"
)

func (f *fixture) rate(currency, day, value string) {
	f.exec(`INSERT INTO exchange_rates (base_currency, quote_currency, as_of, effective_on, rate, source, fetched_at) VALUES (?, 'BRL', ?, ?, ?, 'test', ?)`, currency, day, day, value, db.FormatTime(time.Now()))
}

func TestMixedCurrencyReportingAgreesAcrossConsumers(t *testing.T) {
	f := newFixture(t)
	day := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	f.rate("USD", "2026-03-15", "5")
	f.rate("EUR", "2026-03-15", "6")
	add := func(currency, amount, movement, status, category string) string {
		accountID := f.addAccount(account{CurrencyCode: strp(currency), Balance: strp("0")})
		id := f.addTransaction(txn{AccountID: accountID, Amount: strp(amount), CurrencyCode: strp(currency), OccurredAt: &day, ProviderStatus: strp(status), MovementType: strp(movement)})
		f.setCategory(id, category)
		return id
	}
	usdID := add("USD", "10", "DEBIT", "POSTED", categorySupermercado)
	add("USD", "2", "DEBIT", "PENDING", categorySupermercado)
	add("EUR", "20", "CREDIT", "POSTED", categorySalario)
	add("BRL", "20", "DEBIT", "POSTED", categorySalario)
	add("USD", "500", "DEBIT", "POSTED", categoryTransferencia)
	add("EUR", "500", "CREDIT", "PROCESSING", categorySalario)
	from, to := money.Date{Year: 2026, Month: 3, Day: 1}, money.Date{Year: 2026, Month: 3, Day: 31}
	filters := transactions.Filters{DateFrom: &from, DateTo: &to}
	result, err := transactions.Query(context.Background(), f.conn, transactions.QueryRequest{Timezone: "UTC", GroupBy: money.GroupMonth, PageSize: 100, Filters: filters})
	if err != nil {
		t.Fatal(err)
	}
	totals := result.Reporting.Totals
	if totals == nil || totals.Inflow != "120" || totals.Outflow != "80" || totals.Balance != "40" {
		t.Fatalf("reporting = %+v", result.Reporting)
	}
	if len(result.Totals) != 3 || result.Reporting.Eligible != 4 || result.Reporting.Converted != 4 {
		t.Fatalf("coverage/raw = %+v / %+v", result.Reporting, result.Totals)
	}
	if result.Groups[0].Reporting.Totals.Balance != "40" {
		t.Fatal("group differs")
	}
	for _, item := range result.Items {
		if item.ID == usdID && (*item.Amount != "10" || *item.CurrencyCode != "USD" || item.EffectiveMoney.CurrencyCode != "USD") {
			t.Fatal("original money changed")
		}
	}
	categories, err := transactions.CategoryReporting(context.Background(), f.conn, filters, "UTC", money.Outflow)
	if err != nil {
		t.Fatal(err)
	}
	if categories.Reporting.Totals.Outflow != "80" || len(categories.Items) != 2 {
		t.Fatalf("categories = %+v", categories)
	}
	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{From: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC), ReferenceDate: day})
	if err != nil {
		t.Fatal(err)
	}
	period := timeline.TotalsForPeriod(series)
	if !period.Income.Equal(decimal.RequireFromString("120")) || !period.Expense.Equal(decimal.RequireFromString("80")) || series.Reporting.Totals.Balance != "40" {
		t.Fatalf("timeline = %+v", period)
	}
	if len(series.Totals) != 3 {
		t.Fatalf("timeline raw totals = %+v", series.Totals)
	}
	filters.CategoryIDs = []string{categorySupermercado}
	filtered, err := transactions.CategoryReporting(context.Background(), f.conn, filters, "UTC", money.Outflow)
	if err != nil || filtered.Reporting.Totals.Outflow != "60" || len(filtered.Items) != 1 {
		t.Fatalf("filtered = %+v, %v", filtered, err)
	}
}

func TestMissingFXCannotProduceCompleteReporting(t *testing.T) {
	f := newFixture(t)
	day := time.Date(2099, 3, 15, 12, 0, 0, 0, time.UTC)
	accountID := f.addAccount(account{CurrencyCode: strp("USD")})
	f.addTransaction(txn{AccountID: accountID, Amount: strp("10"), CurrencyCode: strp("USD"), OccurredAt: &day, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT")})
	query := transactions.QueryRequest{Timezone: "UTC", GroupBy: money.GroupMonth}
	result, err := transactions.Query(context.Background(), f.conn, query)
	if err != nil {
		t.Fatal(err)
	}
	if result.Reporting.Status != "unavailable" || result.Reporting.Missing != 1 || result.Reporting.Totals != nil || result.Totals[0].Outflow != "10" {
		t.Fatalf("missing report = %+v", result)
	}
	category, err := transactions.CategoryReporting(context.Background(), f.conn, transactions.Filters{}, "UTC", money.Outflow)
	if err != nil || category.Reporting.Totals != nil || len(category.Items) != 0 {
		t.Fatalf("missing category = %+v, %v", category, err)
	}
	_, err = timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{From: day, To: day, ReferenceDate: day})
	var incomplete *reporting.IncompleteError
	if !errors.As(err, &incomplete) || incomplete.Coverage.Missing != 1 {
		t.Fatalf("timeline missing = %v", err)
	}
	brl := f.addAccount(account{CurrencyCode: strp("BRL")})
	f.addTransaction(txn{AccountID: brl, Amount: strp("20"), CurrencyCode: strp("BRL"), OccurredAt: &day, ProviderStatus: strp("POSTED"), MovementType: strp("CREDIT")})
	result, err = transactions.Query(context.Background(), f.conn, query)
	if err != nil || result.Reporting.Status != "incomplete" || result.Reporting.Totals != nil || result.Reporting.Converted != 1 {
		t.Fatalf("partial report = %+v, %v", result.Reporting, err)
	}
}

func TestReportingRateUsesSelectedPeriodBasis(t *testing.T) {
	f := newFixture(t)
	f.setPreference("transactions.period_basis", "paid_at")
	accountID := f.addAccount(account{CurrencyCode: strp("USD"), AccountType: strp("CREDIT")})
	occurred := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	due := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	f.addBill(bill{AccountID: accountID, ExternalID: "bill-fx", DueDate: due})
	f.addTransaction(txn{AccountID: accountID, Amount: strp("10"), CurrencyCode: strp("USD"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT"), CreditCardMetadata: strp(`{"billId":"bill-fx"}`)})
	f.rate("USD", "2026-02-15", "4")
	f.rate("USD", "2026-03-20", "5")
	from, to := money.Date{Year: 2026, Month: 3, Day: 1}, money.Date{Year: 2026, Month: 3, Day: 31}
	query := transactions.QueryRequest{Timezone: "UTC", GroupBy: money.GroupNone, Filters: transactions.Filters{DateFrom: &from, DateTo: &to}}
	result, err := transactions.Query(context.Background(), f.conn, query)
	if err != nil || result.Reporting.Totals.Outflow != "50" {
		t.Fatalf("paid_at = %+v, %v", result.Reporting, err)
	}
	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{From: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC), ReferenceDate: due})
	if err != nil || !timeline.TotalsForPeriod(series).Expense.Equal(decimal.NewFromInt(50)) {
		t.Fatalf("paid_at timeline = %+v, %v", series.Reporting, err)
	}
	f.exec(`UPDATE settings SET value = 'occurred_at' WHERE key = 'transactions.period_basis'`)
	result, err = transactions.Query(context.Background(), f.conn, query)
	if err != nil || result.Reporting.Eligible != 0 {
		t.Fatalf("occurred_at march = %+v, %v", result.Reporting, err)
	}
	query.Filters = transactions.Filters{}
	result, err = transactions.Query(context.Background(), f.conn, query)
	if err != nil || result.Reporting.Totals.Outflow != "40" {
		t.Fatalf("occurred_at = %+v, %v", result.Reporting, err)
	}
}

func TestReportingRateUsesLocalReportingDay(t *testing.T) {
	f := newFixture(t)
	day := time.Date(2026, 3, 16, 1, 0, 0, 0, time.UTC)
	accountID := f.addAccount(account{CurrencyCode: strp("USD")})
	f.addTransaction(txn{AccountID: accountID, Amount: strp("10"), CurrencyCode: strp("USD"), OccurredAt: &day, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT")})
	f.rate("USD", "2026-03-15", "5")
	f.rate("USD", "2026-03-16", "6")
	for zone, want := range map[string]string{"UTC": "60", "America/Sao_Paulo": "50"} {
		result, err := transactions.Query(context.Background(), f.conn, transactions.QueryRequest{Timezone: zone, GroupBy: money.GroupNone})
		if err != nil || result.Reporting.Totals.Outflow != want {
			t.Fatalf("%s: %+v, %v", zone, result.Reporting, err)
		}
	}
}

func TestReportingUsesEffectiveAccountCurrency(t *testing.T) {
	f := newFixture(t)
	accountID := f.addAccount(account{CurrencyCode: strp("BRL")})
	day := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	f.addTransaction(txn{AccountID: accountID, Amount: strp("10"), AmountInAccountCurrency: strp("51"), CurrencyCode: strp("USD"), OccurredAt: &day, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT")})
	result, err := transactions.Query(context.Background(), f.conn, transactions.QueryRequest{Timezone: "UTC", GroupBy: money.GroupNone})
	if err != nil {
		t.Fatal(err)
	}
	if result.Reporting.Status != "complete" || result.Reporting.Totals.Outflow != "51" || result.Totals[0].CurrencyCode != "BRL" {
		t.Fatalf("effective report = %+v", result.Reporting)
	}
	item := result.Items[0]
	if *item.Amount != "10" || *item.CurrencyCode != "USD" || *item.AmountInAccountCurrency != "51" || item.EffectiveMoney.Source != money.AccountCurrency {
		t.Fatalf("transaction money = %+v", item)
	}
	classification := money.Inflow
	categories, err := transactions.CategoryReporting(context.Background(), f.conn, transactions.Filters{Classification: &classification}, "UTC", money.Outflow)
	if err != nil || categories.Reporting.Eligible != 0 || len(categories.Items) != 0 {
		t.Fatalf("conflicting filter = %+v, %v", categories, err)
	}
}

func TestLocalReportingDayAgreesWithTimelineAtMidnight(t *testing.T) {
	f := newFixture(t)
	occurred := time.Date(2026, 3, 16, 1, 0, 0, 0, time.UTC)
	f.rate("USD", "2026-03-15", "5")
	f.rate("USD", "2026-03-16", "6")
	accountID := f.addAccount(account{CurrencyCode: strp("USD"), Balance: strp("0")})
	id := f.addTransaction(txn{AccountID: accountID, Amount: strp("10"), CurrencyCode: strp("USD"), OccurredAt: &occurred, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT")})
	f.setCategory(id, categorySupermercado)
	for _, window := range []struct{ from, to int }{{1, 31}, {15, 15}} {
		from, to := money.Date{Year: 2026, Month: 3, Day: window.from}, money.Date{Year: 2026, Month: 3, Day: window.to}
		filters := transactions.Filters{DateFrom: &from, DateTo: &to, CategoryIDs: []string{categorySupermercado}}
		query, err := transactions.Query(context.Background(), f.conn, transactions.QueryRequest{Timezone: "America/Sao_Paulo", GroupBy: money.GroupNone, Filters: filters})
		if err != nil {
			t.Fatal(err)
		}
		categories, err := transactions.CategoryReporting(context.Background(), f.conn, filters, "America/Sao_Paulo", money.Outflow)
		if err != nil {
			t.Fatal(err)
		}
		series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{From: time.Date(2026, 3, window.from, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 3, window.to, 0, 0, 0, 0, time.UTC), ReferenceDate: occurred, Timezone: "America/Sao_Paulo", CategoryIDs: []string{categorySupermercado}})
		if err != nil {
			t.Fatal(err)
		}
		if query.Reporting.Totals.Outflow != "50" || categories.Reporting.Totals.Outflow != "50" || series.Reporting.Totals.Outflow != "50" || len(series.Entries) != 1 || series.Entries[0].Date.Day() != 15 {
			t.Fatalf("local reporting differs: query=%+v, category=%+v, timeline=%+v", query.Reporting, categories.Reporting, series)
		}
	}
}
