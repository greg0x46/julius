package timeline_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/reporting"
	"github.com/greg0x46/julius/internal/timeline"
	"github.com/greg0x46/julius/internal/transactions"
)

func TestBuildSeriesMixedCurrencyReportingAgreesWithLedger(t *testing.T) {
	f := newFixture(t)
	day := date(t, "2026-03-15")
	for _, tc := range []struct{ currency, amount, rate, category string }{
		{"BRL", "100", "1", categorySalario},
		{"USD", "-10", "5", categorySupermercado},
		{"EUR", "-20", "6", categorySupermercado},
		{"USD", "-30", "5", categoryTransferencia},
	} {
		account := f.addAccount("0")
		f.exec(`UPDATE financial_accounts SET currency_code = ? WHERE id = ?`, tc.currency, account)
		f.addTransaction(txn{AccountID: account, Amount: tc.amount, OccurredAt: day, CategoryID: &tc.category})
		if tc.currency != "BRL" {
			f.exec(`INSERT INTO exchange_rates(base_currency,quote_currency,as_of,effective_on,rate,source,fetched_at) VALUES(?,'BRL','2026-03-15','2026-03-15',?,'test',?) ON CONFLICT DO NOTHING`, tc.currency, tc.rate, db.FormatTime(time.Now()))
		}
	}
	ctx := context.Background()
	for _, category := range []string{"", categorySupermercado} {
		t.Run(category, func(t *testing.T) {
			params := timeline.BuildParams{From: day, To: day, ReferenceDate: day}
			filters := transactions.Filters{DateFrom: &money.Date{Year: 2026, Month: time.March, Day: 15}, DateTo: &money.Date{Year: 2026, Month: time.March, Day: 15}}
			if category != "" {
				params.CategoryIDs = []string{category}
				filters.CategoryIDs = []string{category}
			}
			series, err := timeline.BuildSeries(ctx, f.conn, params)
			if err != nil {
				t.Fatal(err)
			}
			ledger, err := transactions.Query(ctx, f.conn, transactions.QueryRequest{GroupBy: money.GroupNone, Filters: filters, Timezone: "UTC", Page: 1, PageSize: 100})
			if err != nil {
				t.Fatal(err)
			}
			totals := timeline.TotalsForPeriod(series)
			if totals.Income.String() != ledger.Reporting.Totals.Inflow || totals.Expense.String() != ledger.Reporting.Totals.Outflow || totals.Expense.String() != "170" {
				t.Fatalf("timeline %+v disagrees with ledger %+v", totals, ledger.Reporting)
			}
			if len(series.Totals) < 2 {
				t.Fatalf("native totals missing: %+v", series.Totals)
			}
		})
	}
}

func TestBuildSeriesMissingForeignFlowCannotReturnCompleteSeries(t *testing.T) {
	f := newFixture(t)
	account := f.addAccount("0")
	f.exec(`UPDATE financial_accounts SET currency_code = 'EUR' WHERE id = ?`, account)
	day := date(t, "2099-03-15")
	category := categorySupermercado
	f.addTransaction(txn{AccountID: account, Amount: "-10", OccurredAt: day, CategoryID: &category})
	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{From: day, To: day, ReferenceDate: day})
	var incomplete *reporting.IncompleteError
	if !errors.As(err, &incomplete) || incomplete.Coverage.Missing != 1 || len(series.Points) != 0 {
		t.Fatalf("missing foreign flow = %+v, %v", series, err)
	}
	if len(incomplete.Totals) != 1 || incomplete.Totals[0].CurrencyCode != "EUR" || incomplete.Totals[0].Outflow != "10" {
		t.Fatalf("native coverage missing: %+v", incomplete)
	}
}
