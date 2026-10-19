package investments_test

import (
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/investments"
)

func TestContributionMonthExcludesTradesTransfersIncomeAndReconciliationDuplicates(t *testing.T) {
	eachLedger(t, func(t *testing.T, f *ledgerFixture) {
		opening := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
		f.deposit("1000", opening)
		position := f.addPosition(f.custodyID, "Stock")
		deposit := f.deposit("650", day(2))
		f.link(deposit.ID, f.addBankTransaction("-650", day(2)), "650")
		f.create(investments.OperationInput{AccountID: f.custodyID, PositionID: &position, Kind: investments.OperationBuy, OccurredOn: day(3), Amount: dec("600"), Quantity: decRef("6"), Fees: dec("10")})
		f.create(investments.OperationInput{AccountID: f.custodyID, Kind: investments.OperationIncome, OccurredOn: day(4), Amount: dec("50")})
		f.create(investments.OperationInput{AccountID: f.custodyID, PositionID: &position, Kind: investments.OperationSell, OccurredOn: day(5), Amount: dec("200"), Quantity: decRef("1")})
		account, err := investments.CreateAccount(f.ctx, f.conn, investments.AccountInput{Name: "Second custody"})
		if err != nil {
			t.Fatal(err)
		}
		destination := f.addPosition(account.ID, "Other")
		f.exec(`UPDATE investment_positions SET asset_id = (SELECT asset_id FROM investment_positions WHERE id = ?) WHERE id = ?`, position, destination)
		_, err = investments.CreateTransfer(f.ctx, f.conn, investments.TransferInput{SourcePositionID: position, DestinationPositionID: destination, Quantity: dec("1"), OccurredOn: day(6)})
		if err != nil {
			t.Fatal(err)
		}
		f.withdraw("100", day(8))
		f.deposit("100", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
		got, err := investments.ReadContributionMonth(f.ctx, f.conn, day(20))
		if err != nil || !got.Available || got.OpeningValue.String() != "1000" || got.NetPrincipal.String() != "550" {
			t.Fatalf("month = %+v, %v", got, err)
		}
	})
}

func TestContributionMonthDisclosesSyncedFundingEvenWhenReconciled(t *testing.T) {
	f := newLedgerFixture(t)
	movement := f.addInvestmentMovement()
	check := func() {
		t.Helper()
		got, err := investments.ReadContributionMonth(f.ctx, f.conn, day(20))
		if err != nil || got.Available {
			t.Fatalf("synced month = %+v, %v", got, err)
		}
	}
	check()
	if _, err := f.linkProvider(f.addBankTransaction("-1000", day(1)), movement, "1000"); err != nil {
		t.Fatal(err)
	}
	check()
}

func TestContributionMonthDisclosesUnclassifiedOpeningPrincipal(t *testing.T) {
	f := newLedgerFixture(t)
	value := dec("1000")
	_, err := investments.CreatePosition(f.ctx, f.conn, investments.PositionInput{AccountID: f.custodyID, Name: "Opening", AssetType: "Other", InitialValue: &value, OccurredOn: day(3)})
	if err != nil {
		t.Fatal(err)
	}
	got, err := investments.ReadContributionMonth(f.ctx, f.conn, day(20))
	if err != nil || got.Available {
		t.Fatalf("opening month = %+v, %v", got, err)
	}
}

func TestContributionMonthDisclosesLinkedCashAndMissingOpeningValuation(t *testing.T) {
	t.Run("linked cash", func(t *testing.T) {
		f := newLedgerFixture(t)
		f.exec(`UPDATE investment_accounts SET financial_account_id = ? WHERE id = ?`, f.bankID, f.custodyID)
		got, err := investments.ReadContributionMonth(f.ctx, f.conn, day(20))
		if err != nil || got.Available {
			t.Fatalf("linked cash = %+v, %v", got, err)
		}
	})
	t.Run("unpriced opening", func(t *testing.T) {
		f := newLedgerFixture(t)
		value := dec("1000")
		_, err := investments.CreatePosition(f.ctx, f.conn, investments.PositionInput{AccountID: f.custodyID, Name: "Unpriced", AssetType: "Other", InitialValue: &value, OccurredOn: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			t.Fatal(err)
		}
		got, err := investments.ReadContributionMonth(f.ctx, f.conn, day(20))
		if err != nil || got.Available {
			t.Fatalf("unpriced opening = %+v, %v", got, err)
		}
	})
}
