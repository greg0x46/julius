package investments_test

import (
	"testing"

	"github.com/greg0x46/julius/internal/investments"
)

func (f *ledgerFixture) reportedByID() map[string]string {
	f.t.Helper()
	entries, err := investments.ManualReportingEntries(f.ctx, f.conn)
	if err != nil {
		f.t.Fatalf("ManualReportingEntries: %v", err)
	}
	byID := map[string]string{}
	for _, entry := range entries {
		byID[entry.ID] = entry.Amount.String()
	}
	return byID
}

func (f *ledgerFixture) manualReporting() investments.ManualReporting {
	f.t.Helper()
	reporting, err := investments.ManualOperationReporting(f.ctx, f.conn)
	if err != nil {
		f.t.Fatalf("ManualOperationReporting: %v", err)
	}
	return reporting
}

func (f *ledgerFixture) unrealizedGain() string {
	f.t.Helper()
	summary, err := investments.BuildSummary(f.ctx, f.conn)
	if err != nil {
		f.t.Fatalf("BuildSummary: %v", err)
	}
	return summary.UnrealizedGain.String()
}

func TestUnrealizedValuationIsNotRealizedIncome(t *testing.T) {
	f := newLedgerFixture(t)
	positionID := f.addPosition(f.custodyID, "Ações")
	f.deposit("1000", day(1))
	f.buy(positionID, day(2), "600", "6")
	f.valuation(positionID, day(3), "900")

	if got := f.unrealizedGain(); got != "300" {
		t.Fatalf("unrealized gain = %s, want 300", got)
	}
	if reporting := f.manualReporting(); !reporting.Income.IsZero() {
		t.Fatalf("valuation became realized income: %+v", reporting)
	}
	if got := f.monthlyMovements(nil)["2026-09"]; !got.Income.IsZero() || got.Contributions.String() != "1000" {
		t.Fatalf("valuation leaked into monthly flows: %+v", got)
	}

	income := f.create(investments.OperationInput{
		AccountID: f.custodyID, Kind: investments.OperationIncome, OccurredOn: day(4), Amount: dec("50"),
	})
	if got := f.unrealizedGain(); got != "300" {
		t.Fatalf("realized income moved the unrealized gain to %s", got)
	}
	if got := f.reportedByID()[income.ID]; got != "50" {
		t.Fatalf("income reported %q, want 50", got)
	}
	if reporting := f.manualReporting(); reporting.Income.String() != "50" {
		t.Fatalf("realized income = %+v", reporting)
	}

	f.create(investments.OperationInput{
		AccountID: f.custodyID, Kind: investments.OperationFee, OccurredOn: day(5), Amount: dec("7"),
	})
	f.create(investments.OperationInput{
		AccountID: f.custodyID, Kind: investments.OperationTax, OccurredOn: day(6), Amount: dec("3"),
	})
	f.create(investments.OperationInput{
		AccountID: f.custodyID, PositionID: &positionID, Kind: investments.OperationSell,
		OccurredOn: day(7), Amount: dec("150"), Quantity: decRef("1"), Fees: dec("2"), Taxes: dec("1"),
	})

	reporting := f.manualReporting()
	if reporting.Income.String() != "50" || reporting.Fees.String() != "9" || reporting.Taxes.String() != "4" {
		t.Fatalf("manual reporting = %+v", reporting)
	}
	month := f.monthlyMovements(nil)["2026-09"]
	if month.Income.String() != "50" || month.Fees.String() != "9" || month.Taxes.String() != "4" ||
		month.Contributions.String() != "1000" || !month.Withdrawals.IsZero() {
		t.Fatalf("monthly flows = %+v", month)
	}
}

func TestReportingKeepsTraceabilityToBankTransactions(t *testing.T) {
	f := newLedgerFixture(t)
	depositBank := f.addBankTransaction("-1000", day(1))
	incomeBank := f.addBankTransaction("50", day(2))
	partialBank := f.addBankTransaction("200", day(4))
	fullBank := f.addBankTransaction("740", day(5))

	deposit := f.deposit("1000", day(1))
	income := f.create(investments.OperationInput{
		AccountID: f.custodyID, Kind: investments.OperationIncome, OccurredOn: day(2), Amount: dec("50"),
	})
	fee := f.create(investments.OperationInput{
		AccountID: f.custodyID, Kind: investments.OperationFee, OccurredOn: day(3), Amount: dec("10"),
	})
	partial := f.withdraw("300", day(4))
	full := f.withdraw("740", day(5))
	f.assertCash("after full withdrawal", "0")

	f.link(deposit.ID, depositBank, "1000")
	f.link(income.ID, incomeBank, "50")
	partialLink := f.link(partial.ID, partialBank, "200")
	f.link(full.ID, fullBank, "740")

	// Linked principal is reported by the bank line as a transfer; only the
	// unlinked parcel and the unlinked fee stay as manual entries.
	reported := f.reportedByID()
	if len(reported) != 2 || reported[partial.ID] != "-100" || reported[fee.ID] != "-10" {
		t.Fatalf("reported = %+v", reported)
	}

	for operationID, bankID := range map[string]string{
		deposit.ID: depositBank, income.ID: incomeBank, partial.ID: partialBank, full.ID: fullBank,
	} {
		links, err := investments.ListReconciliations(f.ctx, f.conn, investments.ReconciliationFilter{OperationID: &operationID})
		if err != nil {
			t.Fatal(err)
		}
		if len(links) != 1 || links[0].FinancialTransactionID == nil || *links[0].FinancialTransactionID != bankID {
			t.Fatalf("operation %s links = %+v", operationID, links)
		}
	}

	// Income linked to a bank credit keeps that credit as ordinary income.
	transfers, err := investments.ReconciledTransactionAmounts(f.ctx, f.conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(transfers) != 3 || transfers[depositBank].String() != "1000" ||
		transfers[partialBank].String() != "200" || transfers[fullBank].String() != "740" {
		t.Fatalf("transfers = %+v", transfers)
	}

	if err := investments.DeleteReconciliation(f.ctx, f.conn, partialLink.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.reportedByID()[partial.ID]; got != "-300" {
		t.Fatalf("unlinked partial withdrawal reported %q, want -300", got)
	}
}
