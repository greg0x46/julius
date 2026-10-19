package investments_test

import (
	"errors"
	"testing"

	"github.com/greg0x46/julius/internal/investments"
)

func (f *ledgerFixture) link(operationID, transactionID, amount string) investments.Reconciliation {
	f.t.Helper()
	in := investments.ReconciliationInput{OperationID: operationID, Amount: dec(amount)}
	if transactionID != "" {
		in.FinancialTransactionID = &transactionID
	}
	link, err := investments.CreateReconciliation(f.ctx, f.conn, in)
	if err != nil {
		f.t.Fatalf("CreateReconciliation: %v", err)
	}
	return link
}

func TestBankLineSplitsBetweenOperations(t *testing.T) {
	f := newLedgerFixture(t)
	transactionID := f.addBankTransaction("-1000", day(1))
	first := f.deposit("600", day(1))
	second := f.deposit("400", day(1))
	third := f.deposit("100", day(1))

	f.link(first.ID, transactionID, "600")
	f.link(second.ID, transactionID, "400")

	links, err := investments.ListReconciliations(f.ctx, f.conn, investments.ReconciliationFilter{FinancialTransactionID: &transactionID})
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 || links[0].Amount.String() != "600" || links[1].Amount.String() != "400" {
		t.Fatalf("links = %+v", links)
	}
	byOperation, err := investments.ListReconciliations(f.ctx, f.conn, investments.ReconciliationFilter{OperationID: &first.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(byOperation) != 1 || byOperation[0].FinancialTransactionID == nil || *byOperation[0].FinancialTransactionID != transactionID {
		t.Fatalf("operation links = %+v", byOperation)
	}

	// The bank line is fully allocated; a third parcel would explain money
	// that movement never carried.
	if _, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{
		OperationID: third.ID, FinancialTransactionID: &transactionID, Amount: dec("100"),
	}); !errors.Is(err, investments.ErrReconciliationConflict) {
		t.Fatalf("over-allocated bank line = %v", err)
	}

	// Linking the same parcel again is a duplicate, not a second split.
	if _, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{
		OperationID: first.ID, FinancialTransactionID: &transactionID, Amount: dec("1"),
	}); !errors.Is(err, investments.ErrReconciliationDuplicate) {
		t.Fatalf("duplicate pair = %v", err)
	}
}

func TestReconciliationCannotExceedItsOperation(t *testing.T) {
	f := newLedgerFixture(t)
	transactionID := f.addBankTransaction("-1000", day(1))
	deposit := f.deposit("500", day(1))
	if _, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{
		OperationID: deposit.ID, FinancialTransactionID: &transactionID, Amount: dec("600"),
	}); !errors.Is(err, investments.ErrReconciliationConflict) {
		t.Fatalf("over-allocated operation = %v", err)
	}
	f.link(deposit.ID, transactionID, "300")
	other := f.addBankTransaction("-1000", day(2))
	if _, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{
		OperationID: deposit.ID, FinancialTransactionID: &other, Amount: dec("300"),
	}); !errors.Is(err, investments.ErrReconciliationConflict) {
		t.Fatalf("second parcel above operation = %v", err)
	}
}

func TestReconciliationRefusesWrongDirectionAndKind(t *testing.T) {
	f := newLedgerFixture(t)
	incoming := f.addBankTransaction("1000", day(1))
	outgoing := f.addBankTransaction("-1000", day(1))

	deposit := f.deposit("1000", day(1))
	if _, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{
		OperationID: deposit.ID, FinancialTransactionID: &incoming, Amount: dec("1000"),
	}); !errors.Is(err, investments.ErrInvalidReconciliationLink) {
		t.Fatalf("deposit against arriving money = %v", err)
	}

	withdrawal := f.create(investments.OperationInput{
		AccountID: f.custodyID, Kind: investments.OperationWithdrawal, OccurredOn: day(2), Amount: dec("400"),
	})
	if _, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{
		OperationID: withdrawal.ID, FinancialTransactionID: &outgoing, Amount: dec("400"),
	}); !errors.Is(err, investments.ErrInvalidReconciliationLink) {
		t.Fatalf("withdrawal against leaving money = %v", err)
	}
	f.link(withdrawal.ID, incoming, "400")

	positionID := f.addPosition(f.custodyID, "Ações")
	buy := f.create(investments.OperationInput{
		AccountID: f.custodyID, PositionID: &positionID, Kind: investments.OperationBuy,
		OccurredOn: day(3), Amount: dec("300"), Quantity: decRef("3"), UnitPrice: decRef("100"),
	})
	if _, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{
		OperationID: buy.ID, FinancialTransactionID: &outgoing, Amount: dec("300"),
	}); !errors.Is(err, investments.ErrInvalidReconciliationLink) {
		t.Fatalf("buy link = %v", err)
	}

	fee := f.create(investments.OperationInput{
		AccountID: f.custodyID, Kind: investments.OperationFee, OccurredOn: day(4), Amount: dec("20"),
	})
	f.link(fee.ID, outgoing, "20")

	income := f.create(investments.OperationInput{
		AccountID: f.custodyID, Kind: investments.OperationIncome, OccurredOn: day(5), Amount: dec("50"),
	})
	f.link(income.ID, incoming, "50")
}

func TestReconciliationInputValidation(t *testing.T) {
	f := newLedgerFixture(t)
	transactionID := f.addBankTransaction("-1000", day(1))
	deposit := f.deposit("1000", day(1))

	if _, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{
		OperationID: deposit.ID, FinancialTransactionID: &transactionID, Amount: dec("0"),
	}); !errors.Is(err, investments.ErrInvalidInput) {
		t.Fatalf("zero amount = %v", err)
	}
	if _, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{
		OperationID: deposit.ID, Amount: dec("100"),
	}); !errors.Is(err, investments.ErrInvalidReconciliationLink) {
		t.Fatalf("link without a source = %v", err)
	}
	missing := "missing-transaction"
	if _, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{
		OperationID: deposit.ID, FinancialTransactionID: &missing, Amount: dec("100"),
	}); !errors.Is(err, investments.ErrInvalidReconciliationLink) {
		t.Fatalf("unknown bank line = %v", err)
	}
	if _, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{
		OperationID: deposit.ID, FinancialInvestmentTransactionID: &missing, Amount: dec("100"),
	}); !errors.Is(err, investments.ErrInvalidReconciliationLink) {
		t.Fatalf("unknown investment movement = %v", err)
	}
	if _, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{
		OperationID: "missing", FinancialTransactionID: &transactionID, Amount: dec("100"),
	}); !errors.Is(err, investments.ErrOperationNotFound) {
		t.Fatalf("unknown operation = %v", err)
	}

	movementID := f.addInvestmentMovement()
	link, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{
		OperationID: deposit.ID, FinancialInvestmentTransactionID: &movementID, Amount: dec("1000"),
	})
	if err != nil {
		t.Fatalf("imported movement link: %v", err)
	}
	if link.FinancialTransactionID != nil || link.FinancialInvestmentTransactionID == nil {
		t.Fatalf("link = %+v", link)
	}
	if _, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{
		OperationID: deposit.ID, FinancialInvestmentTransactionID: &movementID, Amount: dec("1"),
	}); !errors.Is(err, investments.ErrReconciliationDuplicate) {
		t.Fatalf("duplicate imported movement = %v", err)
	}
	if err := investments.DeleteReconciliation(f.ctx, f.conn, "missing"); !errors.Is(err, investments.ErrReconciliationNotFound) {
		t.Fatalf("delete of unknown link = %v", err)
	}
}

func TestUnlinkingRestoresPlainReporting(t *testing.T) {
	f := newLedgerFixture(t)
	transactionID := f.addBankTransaction("-1000", day(1))
	deposit := f.deposit("1000", day(1))

	reportedAmount := func() string {
		f.t.Helper()
		entries, err := investments.ManualReportingEntries(f.ctx, f.conn)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.ID == deposit.ID {
				return entry.Amount.String()
			}
		}
		return ""
	}

	if got := reportedAmount(); got != "1000" {
		t.Fatalf("unlinked deposit reported %q, want 1000", got)
	}
	link := f.link(deposit.ID, transactionID, "1000")
	if got := reportedAmount(); got != "" {
		t.Fatalf("fully linked deposit still reported %q", got)
	}
	if err := investments.DeleteReconciliation(f.ctx, f.conn, link.ID); err != nil {
		t.Fatal(err)
	}
	if got := reportedAmount(); got != "1000" {
		t.Fatalf("unlinked deposit reported %q, want 1000", got)
	}
	f.assertCash("reconciliation never moves cash", "1000")
}

func TestLinkedOperationResistsIncompatibleChanges(t *testing.T) {
	f := newLedgerFixture(t)
	transactionID := f.addBankTransaction("-1000", day(1))
	deposit := f.deposit("1000", day(1))
	link := f.link(deposit.ID, transactionID, "800")

	if err := investments.DeleteOperation(f.ctx, f.conn, deposit.ID); !errors.Is(err, investments.ErrOperationHasReconciliations) {
		t.Fatalf("delete of linked operation = %v", err)
	}
	if _, err := investments.UpdateOperation(f.ctx, f.conn, deposit.ID, investments.OperationInput{
		AccountID: f.custodyID, Kind: investments.OperationDeposit, OccurredOn: day(1), Amount: dec("500"),
	}); !errors.Is(err, investments.ErrReconciliationConflict) {
		t.Fatalf("shrinking below the linked parcel = %v", err)
	}
	// Reversing the direction would leave the bank line explaining the
	// opposite movement.
	if _, err := investments.UpdateOperation(f.ctx, f.conn, deposit.ID, investments.OperationInput{
		AccountID: f.custodyID, Kind: investments.OperationWithdrawal, OccurredOn: day(1), Amount: dec("1000"),
	}); !errors.Is(err, investments.ErrInvalidReconciliationLink) {
		t.Fatalf("direction flip = %v", err)
	}
	if _, err := investments.UpdateOperation(f.ctx, f.conn, deposit.ID, investments.OperationInput{
		AccountID: f.custodyID, Kind: investments.OperationDeposit, OccurredOn: day(2), Amount: dec("900"),
	}); err != nil {
		t.Fatalf("compatible correction: %v", err)
	}

	if err := investments.DeleteReconciliation(f.ctx, f.conn, link.ID); err != nil {
		t.Fatal(err)
	}
	if err := investments.DeleteOperation(f.ctx, f.conn, deposit.ID); err != nil {
		t.Fatalf("delete after unlinking: %v", err)
	}
	f.assertCash("after delete", "0")
}

func TestReconciliationCurrencyAndImportedCapacity(t *testing.T) {
	f := newLedgerFixture(t)
	bank := f.addBankTransaction("-1000", day(1))
	op := f.deposit("1000", day(1))
	f.exec(`UPDATE financial_accounts SET currency_code='USD' WHERE id=?`, f.bankID)
	if _, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{OperationID: op.ID, FinancialTransactionID: &bank, Amount: dec("1000")}); !errors.Is(err, investments.ErrInvalidReconciliationLink) {
		t.Fatalf("foreign currency = %v", err)
	}
	movement := f.addInvestmentMovement()
	if _, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{OperationID: op.ID, FinancialInvestmentTransactionID: &movement, Amount: dec("1000")}); err != nil {
		t.Fatal(err)
	}
	other := f.deposit("100", day(2))
	if _, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{OperationID: other.ID, FinancialInvestmentTransactionID: &movement, Amount: dec("1")}); !errors.Is(err, investments.ErrReconciliationConflict) {
		t.Fatalf("provider parcel duplicated = %v", err)
	}
	f.exec(`UPDATE financial_investment_transactions SET direction='outflow' WHERE id=?`, movement)
	if _, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{OperationID: other.ID, FinancialInvestmentTransactionID: &movement, Amount: dec("1")}); !errors.Is(err, investments.ErrInvalidReconciliationLink) {
		t.Fatalf("wrong provider direction = %v", err)
	}
}

func (f *ledgerFixture) linkProvider(transactionID, movementID, amount string) (investments.Reconciliation, error) {
	f.t.Helper()
	in := investments.ReconciliationInput{FinancialInvestmentTransactionID: &movementID, Amount: dec(amount)}
	if transactionID != "" {
		in.FinancialTransactionID = &transactionID
	}
	return investments.CreateReconciliation(f.ctx, f.conn, in)
}

func TestProviderMovementBecomesTheReconciliationPivot(t *testing.T) {
	f := newLedgerFixture(t)
	bank := f.addBankTransaction("-1000", day(13))
	movement := f.addInvestmentMovement()
	f.exec(`UPDATE financial_investment_transactions SET trade_date = ? WHERE id = ?`, "2026-09-13", movement)

	link, err := f.linkProvider(bank, movement, "1000")
	if err != nil {
		t.Fatalf("link without operation: %v", err)
	}
	if link.FinancialTransactionID == nil || link.FinancialInvestmentTransactionID == nil {
		t.Fatalf("link = %+v", link)
	}
	pivot, err := investments.GetOperation(f.ctx, f.conn, link.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if pivot.Source != "synced" || pivot.IsEditable || pivot.Kind != investments.OperationDeposit ||
		pivot.AccountID != "integrated:"+f.sourceID || pivot.Amount.String() != "1000" || pivot.PositionID != nil ||
		pivot.OccurredOn.Format("2006-01-02") != "2026-09-13" {
		t.Fatalf("derived pivot = %+v", pivot)
	}
	if pivot.Notes == nil || *pivot.Notes != "APPLICATION" {
		t.Fatalf("pivot notes = %v", pivot.Notes)
	}

	transfers, err := investments.ReconciledTransactionAmounts(f.ctx, f.conn)
	if err != nil {
		t.Fatal(err)
	}
	if transfers[bank].String() != "1000" {
		t.Fatalf("bank line not reported as transfer: %+v", transfers)
	}
	entries, err := investments.ManualReportingEntries(f.ctx, f.conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("derived pivot leaked into manual reporting: %+v", entries)
	}

	// The pivot is provider evidence, not a user entry.
	if _, err := investments.UpdateOperation(f.ctx, f.conn, pivot.ID, investments.OperationInput{
		AccountID: pivot.AccountID, Kind: pivot.Kind, OccurredOn: pivot.OccurredOn, Amount: dec("1"),
	}); !errors.Is(err, investments.ErrNotManual) {
		t.Fatalf("edit derived pivot = %v", err)
	}
	if err := investments.DeleteOperation(f.ctx, f.conn, pivot.ID); !errors.Is(err, investments.ErrNotManual) {
		t.Fatalf("delete derived pivot = %v", err)
	}

	if err := investments.DeleteReconciliation(f.ctx, f.conn, link.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := investments.GetOperation(f.ctx, f.conn, pivot.ID); !errors.Is(err, investments.ErrOperationNotFound) {
		t.Fatalf("orphan pivot survived unlink: %v", err)
	}
}

func TestProviderPivotIsSharedBetweenBankParcels(t *testing.T) {
	f := newLedgerFixture(t)
	movement := f.addInvestmentMovement()
	first := f.addBankTransaction("-600", day(1))
	second := f.addBankTransaction("-400", day(1))
	third := f.addBankTransaction("-1", day(1))

	a, err := f.linkProvider(first, movement, "600")
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.linkProvider(second, movement, "400")
	if err != nil {
		t.Fatalf("second parcel: %v", err)
	}
	if a.OperationID != b.OperationID {
		t.Fatalf("pivots differ: %s vs %s", a.OperationID, b.OperationID)
	}
	if b.FinancialInvestmentTransactionID != nil {
		t.Fatalf("second parcel repeated the movement link: %+v", b)
	}
	if _, err := f.linkProvider(third, movement, "1"); !errors.Is(err, investments.ErrReconciliationConflict) {
		t.Fatalf("over-allocated movement = %v", err)
	}
	if _, err := f.linkProvider("", movement, "1"); !errors.Is(err, investments.ErrReconciliationDuplicate) {
		t.Fatalf("movement-only repeat = %v", err)
	}

	if err := investments.DeleteReconciliation(f.ctx, f.conn, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := investments.GetOperation(f.ctx, f.conn, a.OperationID); err != nil {
		t.Fatalf("pivot with a remaining link was removed: %v", err)
	}
	if err := investments.DeleteReconciliation(f.ctx, f.conn, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := investments.GetOperation(f.ctx, f.conn, a.OperationID); !errors.Is(err, investments.ErrOperationNotFound) {
		t.Fatalf("orphan pivot survived: %v", err)
	}
}

// A provider investment without a currency is in reais, as everywhere else.
func TestProviderMovementOfAnInvestmentWithNoCurrencyIsInReais(t *testing.T) {
	f := newLedgerFixture(t)
	bank := f.addBankTransaction("-1000", day(13))
	movement := f.addInvestmentMovement()
	f.exec(`UPDATE financial_investments SET currency_code = NULL`)
	if _, err := f.linkProvider(bank, movement, "1000"); err != nil {
		t.Fatalf("pivot from a movement with no currency: %v", err)
	}

	other := f.addInvestmentMovement()
	f.exec(`UPDATE financial_investments SET currency_code = NULL`)
	deposit := f.deposit("500", day(14))
	if _, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{
		OperationID: deposit.ID, FinancialInvestmentTransactionID: &other, Amount: dec("500"),
	}); err != nil {
		t.Fatalf("link to a movement with no currency: %v", err)
	}

	// A foreign currency is still refused.
	foreign := f.addInvestmentMovement()
	f.exec(`UPDATE financial_investments SET currency_code = 'USD' WHERE id = (SELECT investment_id FROM financial_investment_transactions WHERE id = ?)`, foreign)
	if _, err := f.linkProvider("", foreign, "1000"); !errors.Is(err, investments.ErrInvalidReconciliationLink) {
		t.Fatalf("USD movement = %v, want ErrInvalidReconciliationLink", err)
	}
}

func TestProviderPivotFollowsNormalizedDirection(t *testing.T) {
	f := newLedgerFixture(t)
	movement := f.addInvestmentMovement()
	outflowBank := f.addBankTransaction("-1000", day(1))
	inflowBank := f.addBankTransaction("1000", day(1))

	f.exec(`UPDATE financial_investment_transactions SET direction = 'outflow', movement_type = 'SELL' WHERE id = ?`, movement)
	if _, err := f.linkProvider(outflowBank, movement, "1000"); !errors.Is(err, investments.ErrInvalidReconciliationLink) {
		t.Fatalf("resgate against bank outflow = %v", err)
	}
	link, err := f.linkProvider(inflowBank, movement, "1000")
	if err != nil {
		t.Fatal(err)
	}
	pivot, err := investments.GetOperation(f.ctx, f.conn, link.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if pivot.Kind != investments.OperationWithdrawal {
		t.Fatalf("SELL/outflow kind = %s", pivot.Kind)
	}
	if err := investments.DeleteReconciliation(f.ctx, f.conn, link.ID); err != nil {
		t.Fatal(err)
	}

	f.exec(`UPDATE financial_investment_transactions SET movement_type = 'INTEREST' WHERE id = ?`, movement)
	link, err = f.linkProvider(inflowBank, movement, "1000")
	if err != nil {
		t.Fatal(err)
	}
	if pivot, err = investments.GetOperation(f.ctx, f.conn, link.OperationID); err != nil || pivot.Kind != investments.OperationIncome {
		t.Fatalf("INTEREST kind = %s (%v)", pivot.Kind, err)
	}
	if err := investments.DeleteReconciliation(f.ctx, f.conn, link.ID); err != nil {
		t.Fatal(err)
	}

	f.exec(`UPDATE financial_investment_transactions SET direction = NULL WHERE id = ?`, movement)
	if _, err := f.linkProvider(inflowBank, movement, "1000"); !errors.Is(err, investments.ErrInvalidReconciliationLink) {
		t.Fatalf("unknown direction = %v", err)
	}
	if _, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{
		FinancialTransactionID: &inflowBank, Amount: dec("1"),
	}); !errors.Is(err, investments.ErrInvalidReconciliationLink) {
		t.Fatalf("no operation and no movement = %v", err)
	}
}

func TestProviderMovementWithoutDateCannotPivot(t *testing.T) {
	f := newLedgerFixture(t)
	movement := f.addInvestmentMovement()
	bank := f.addBankTransaction("-1000", day(1))
	f.exec(`UPDATE financial_investment_transactions SET occurred_at = NULL, trade_date = NULL WHERE id = ?`, movement)
	if _, err := f.linkProvider(bank, movement, "1000"); !errors.Is(err, investments.ErrInvalidReconciliationLink) {
		t.Fatalf("undated movement = %v", err)
	}
}

func (f *ledgerFixture) syncedOperationCount() int {
	f.t.Helper()
	var count int
	if err := f.conn.QueryRow(`SELECT COUNT(*) FROM investment_operations WHERE source = 'synced'`).Scan(&count); err != nil {
		f.t.Fatal(err)
	}
	return count
}

func TestDeletingTheParcelWithTheMovementKeepsThePivotReusable(t *testing.T) {
	f := newLedgerFixture(t)
	movement := f.addInvestmentMovement()
	first := f.addBankTransaction("-600", day(1))
	second := f.addBankTransaction("-400", day(1))
	third := f.addBankTransaction("-1000", day(2))

	a, err := f.linkProvider(first, movement, "600")
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.linkProvider(second, movement, "400")
	if err != nil {
		t.Fatal(err)
	}

	// Only A recorded the movement; deleting it must hand the movement to B
	// so the pivot stays findable instead of surviving as a ghost.
	if err := investments.DeleteReconciliation(f.ctx, f.conn, a.ID); err != nil {
		t.Fatal(err)
	}
	remaining, err := investments.ListReconciliations(f.ctx, f.conn, investments.ReconciliationFilter{OperationID: &a.OperationID})
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].ID != b.ID || remaining[0].FinancialInvestmentTransactionID == nil ||
		*remaining[0].FinancialInvestmentTransactionID != movement {
		t.Fatalf("movement did not move to the sibling parcel: %+v", remaining)
	}

	// The next parcel reuses the same pivot, whose capacity is what B left
	// over — not a second pivot with the movement's full amount again.
	if _, err := f.linkProvider(third, movement, "1000"); !errors.Is(err, investments.ErrReconciliationConflict) {
		t.Fatalf("relinking beyond the remaining capacity = %v", err)
	}
	c, err := f.linkProvider(third, movement, "600")
	if err != nil {
		t.Fatalf("relink after deleting the first parcel: %v", err)
	}
	if c.OperationID != b.OperationID {
		t.Fatalf("second pivot derived: %s vs %s", c.OperationID, b.OperationID)
	}
	if got := f.syncedOperationCount(); got != 1 {
		t.Fatalf("synced pivots = %d, want 1", got)
	}
	transfers, err := investments.ReconciledTransactionAmounts(f.ctx, f.conn)
	if err != nil {
		t.Fatal(err)
	}
	if total := transfers[second].Add(transfers[third]); total.String() != "1000" || transfers[first].String() != "0" {
		t.Fatalf("reported transfers %+v exceed the movement", transfers)
	}

	for _, id := range []string{b.ID, c.ID} {
		if err := investments.DeleteReconciliation(f.ctx, f.conn, id); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.syncedOperationCount(); got != 0 {
		t.Fatalf("orphan pivot survived: %d", got)
	}
}

func TestManualOperationKeepsTheMovementOnItsOwnParcel(t *testing.T) {
	f := newLedgerFixture(t)
	movement := f.addInvestmentMovement()
	first := f.addBankTransaction("-600", day(1))
	second := f.addBankTransaction("-400", day(1))
	deposit := f.deposit("1000", day(1))

	a, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{
		OperationID: deposit.ID, FinancialTransactionID: &first, FinancialInvestmentTransactionID: &movement, Amount: dec("600"),
	})
	if err != nil {
		t.Fatal(err)
	}
	b := f.link(deposit.ID, second, "400")

	// On a manual operation the movement is the user's choice for that
	// parcel alone; undoing it does not reassign the movement to another.
	if err := investments.DeleteReconciliation(f.ctx, f.conn, a.ID); err != nil {
		t.Fatal(err)
	}
	remaining, err := investments.ListReconciliations(f.ctx, f.conn, investments.ReconciliationFilter{OperationID: &deposit.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].ID != b.ID || remaining[0].FinancialInvestmentTransactionID != nil {
		t.Fatalf("manual parcel gained a movement it never had: %+v", remaining)
	}
}

func (f *ledgerFixture) ignore(transactionID string) {
	f.t.Helper()
	f.exec(`INSERT INTO transaction_inclusion_decisions (transaction_id, state, revision, changed_at, origin)
		VALUES (?, 'ignored', 1, ?, 'manual')`, transactionID, "2026-09-01T00:00:00.000000000Z")
}

func TestIgnoredBankLineCannotBeReconciled(t *testing.T) {
	f := newLedgerFixture(t)
	bank := f.addBankTransaction("-1000", day(1))
	deposit := f.deposit("1000", day(1))
	f.ignore(bank)
	if _, err := investments.CreateReconciliation(f.ctx, f.conn, investments.ReconciliationInput{
		OperationID: deposit.ID, FinancialTransactionID: &bank, Amount: dec("1000"),
	}); !errors.Is(err, investments.ErrInvalidReconciliationLink) {
		t.Fatalf("ignored bank line = %v", err)
	}
	movement := f.addInvestmentMovement()
	if _, err := f.linkProvider(bank, movement, "1000"); !errors.Is(err, investments.ErrInvalidReconciliationLink) {
		t.Fatalf("ignored bank line against a provider movement = %v", err)
	}
	if got := f.syncedOperationCount(); got != 0 {
		t.Fatalf("refused provider link left a pivot behind: %d", got)
	}
}

func TestUnlinkTransactionDropsEveryParcelAndTheOrphanPivot(t *testing.T) {
	f := newLedgerFixture(t)
	bank := f.addBankTransaction("-1000", day(1))
	deposit := f.deposit("1000", day(1))
	movement := f.addInvestmentMovement()

	if err := investments.UnlinkTransactionIfPresent(f.ctx, f.conn, bank); err != nil {
		t.Fatalf("unlink without links: %v", err)
	}
	f.link(deposit.ID, bank, "600")
	if _, err := f.linkProvider(bank, movement, "400"); err != nil {
		t.Fatal(err)
	}

	if err := investments.UnlinkTransactionIfPresent(f.ctx, f.conn, bank); err != nil {
		t.Fatal(err)
	}
	links, err := investments.ListReconciliations(f.ctx, f.conn, investments.ReconciliationFilter{FinancialTransactionID: &bank})
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Fatalf("links survived: %+v", links)
	}
	if got := f.syncedOperationCount(); got != 0 {
		t.Fatalf("orphan pivot survived: %d", got)
	}
	// The manual deposit is whole again, reported by itself.
	entries, err := investments.ManualReportingEntries(f.ctx, f.conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != deposit.ID || entries[0].Amount.String() != "1000" {
		t.Fatalf("manual reporting after unlink = %+v", entries)
	}
	transfer, err := investments.ReconciledTransactionAmount(f.ctx, f.conn, bank)
	if err != nil || !transfer.IsZero() {
		t.Fatalf("transfer after unlink = %s (%v)", transfer, err)
	}
}

func TestReconciledTransactionAmountReadsOneLine(t *testing.T) {
	f := newLedgerFixture(t)
	bank := f.addBankTransaction("-1000", day(1))
	other := f.addBankTransaction("-300", day(1))
	deposit := f.deposit("600", day(1))
	withdrawal := f.create(investments.OperationInput{
		AccountID: f.custodyID, Kind: investments.OperationWithdrawal, OccurredOn: day(2), Amount: dec("500"),
	})
	fee := f.create(investments.OperationInput{
		AccountID: f.custodyID, Kind: investments.OperationFee, OccurredOn: day(3), Amount: dec("50"),
	})
	incoming := f.addBankTransaction("500", day(2))

	f.link(deposit.ID, bank, "600")
	f.link(fee.ID, bank, "50")
	f.link(withdrawal.ID, incoming, "500")

	// Deposit parcels count, the fee parcel stays in ordinary reporting,
	// and other lines never leak in.
	transfer, err := investments.ReconciledTransactionAmount(f.ctx, f.conn, bank)
	if err != nil || transfer.String() != "600" {
		t.Fatalf("transfer for bank line = %s (%v)", transfer, err)
	}
	transfer, err = investments.ReconciledTransactionAmount(f.ctx, f.conn, incoming)
	if err != nil || transfer.String() != "500" {
		t.Fatalf("transfer for resgate line = %s (%v)", transfer, err)
	}
	transfer, err = investments.ReconciledTransactionAmount(f.ctx, f.conn, other)
	if err != nil || !transfer.IsZero() {
		t.Fatalf("transfer for unlinked line = %s (%v)", transfer, err)
	}
	all, err := investments.ReconciledTransactionAmounts(f.ctx, f.conn)
	if err != nil || all[bank].String() != "600" || all[incoming].String() != "500" {
		t.Fatalf("bulk view disagrees: %+v (%v)", all, err)
	}
}
