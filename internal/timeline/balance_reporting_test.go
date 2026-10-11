package timeline_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/reporting"
	"github.com/greg0x46/julius/internal/timeline"
)

func TestReportingBalanceConvertsAtReferenceAndFiltersAccounts(t *testing.T) {
	f := newFixture(t)
	usd := f.addAccount("100")
	f.exec(`UPDATE financial_accounts SET currency_code = 'USD' WHERE id = ?`, usd)
	f.addAccount("30")
	f.addCreditCardAccount("999")
	for day, rate := range map[string]string{"2026-03-15": "5", "2026-03-16": "6"} {
		f.exec(`INSERT INTO exchange_rates (base_currency, quote_currency, as_of, effective_on, rate, source, fetched_at) VALUES ('USD', 'BRL', ?, ?, ?, 'test', ?)`, day, day, rate, db.FormatTime(time.Now()))
	}
	tx := f.addTransaction(txn{AccountID: usd, Amount: "10", OccurredAt: date(t, "2026-03-15"), CategoryID: &[]string{categorySalario}[0]})
	f.exec(`UPDATE financial_transactions SET currency_code = 'USD' WHERE id = ?`, tx)
	params := timeline.BuildParams{From: date(t, "2026-03-15"), To: date(t, "2026-03-17"), ReferenceDate: date(t, "2026-03-16")}
	s, err := timeline.BuildSeries(context.Background(), f.conn, params)
	if err != nil {
		t.Fatal(err)
	}
	if s.StartingBalance.String() != "630" || s.Points[1].Balance.String() != "630" || s.LowestBalance.Balance.String() != "630" {
		t.Fatalf("balance = %+v", s)
	}
	if s.Reporting.Totals.Inflow != "50" || s.Reporting.Eligible != 1 || s.BalanceReporting.Eligible != 2 {
		t.Fatalf("coverage = %+v / %+v", s.Reporting, s.BalanceReporting)
	}
	params.AccountIDs = []string{usd}
	s, err = timeline.BuildSeries(context.Background(), f.conn, params)
	if err != nil || s.StartingBalance.String() != "600" {
		t.Fatalf("filtered balance = %v, %v", s.StartingBalance, err)
	}
}

func TestReportingBalanceMissingFXCannotReturnSeries(t *testing.T) {
	f := newFixture(t)
	usd := f.addAccount("100")
	f.exec(`UPDATE financial_accounts SET currency_code = 'USD' WHERE id = ?`, usd)
	ref := date(t, "2099-03-15")
	s, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{From: ref, To: ref, ReferenceDate: ref})
	var incomplete *reporting.IncompleteError
	if !errors.As(err, &incomplete) || incomplete.Coverage.Missing != 1 || len(s.Points) != 0 {
		t.Fatalf("missing = %+v, %v", s, err)
	}
	if incomplete.Error() != "Faltam cotações históricas para converter o saldo das contas." {
		t.Fatal(incomplete.Error())
	}
}

func TestReportingBalanceBRLNeedsNoRates(t *testing.T) {
	f := newFixture(t)
	f.addAccount("100")
	f.addAccount("-20")
	ref := date(t, "2099-03-15")
	s, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{From: ref, To: ref, ReferenceDate: ref})
	if err != nil || s.StartingBalance.String() != "80" || s.BalanceReporting.Status != "complete" || s.Reporting.Eligible != 0 {
		t.Fatalf("BRL = %+v, %v", s, err)
	}
}

func TestReportingBalanceDisclosesStaleFXSeparatelyFromPeriodCoverage(t *testing.T) {
	f := newFixture(t)
	usd := f.addAccount("100")
	f.exec(`UPDATE financial_accounts SET currency_code = 'USD' WHERE id = ?`, usd)
	f.exec(`INSERT INTO exchange_rates (base_currency, quote_currency, as_of, effective_on, rate, source, fetched_at) VALUES ('USD', 'BRL', '2026-03-15', '2026-03-01', '5', 'test', ?)`, db.FormatTime(time.Now()))
	ref := date(t, "2026-03-15")
	s, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{From: ref, To: ref, ReferenceDate: ref})
	if err != nil || s.StartingBalance.String() != "500" || s.BalanceReporting.Status != "stale" || s.BalanceReporting.Stale != 1 || s.Reporting.Status != "complete" || s.Reporting.Stale != 0 {
		t.Fatalf("stale = %+v, %v", s, err)
	}
}

func TestReportingBalanceUnknownCurrencyIsExplicitlyUnavailable(t *testing.T) {
	f := newFixture(t)
	account := f.addAccount("100")
	f.exec(`UPDATE financial_accounts SET currency_code = NULL WHERE id = ?`, account)
	ref := date(t, "2026-03-15")
	_, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{From: ref, To: ref, ReferenceDate: ref})
	var incomplete *reporting.IncompleteError
	if !errors.As(err, &incomplete) || incomplete.Coverage.Status != "unavailable" || incomplete.Totals == nil {
		t.Fatalf("unknown currency = %v", err)
	}
}
