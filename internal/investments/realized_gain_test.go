package investments_test

import (
	"context"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/investments"
)

func (f *ledgerFixture) buyWithCosts(accountID, positionID string, on time.Time, amount, quantity, fees, taxes string) {
	f.t.Helper()
	f.create(investments.OperationInput{
		AccountID: accountID, PositionID: &positionID, Kind: investments.OperationBuy, OccurredOn: on,
		Amount: dec(amount), Quantity: decRef(quantity), Fees: dec(fees), Taxes: dec(taxes),
	})
}

func (f *ledgerFixture) sellWithCosts(accountID, positionID string, on time.Time, amount, quantity, fees, taxes string) {
	f.t.Helper()
	f.create(investments.OperationInput{
		AccountID: accountID, PositionID: &positionID, Kind: investments.OperationSell, OccurredOn: on,
		Amount: dec(amount), Quantity: decRef(quantity), Fees: dec(fees), Taxes: dec(taxes),
	})
}

// Realized gain is the net proceeds of a sale (principal minus its fees and
// taxes, as the ledger credits the cash) minus the all-in cost basis the sale
// wrote off, so purchase costs are inside it too and nothing is deducted twice.
func TestMonthlyRealizedGainIsNetOfPurchaseAndSaleCosts(t *testing.T) {
	eachLedger(t, func(t *testing.T, f *ledgerFixture) {
		positionID := f.addPosition(f.custodyID, "Ações")
		october := func(n int) time.Time { return time.Date(2026, 10, n, 0, 0, 0, 0, time.UTC) }
		f.deposit("2000", day(1))
		// 6 units for 600 plus 12 fees and 6 taxes: a basis of 618 (103 a unit).
		f.buyWithCosts(f.custodyID, positionID, day(2), "600", "6", "12", "6")

		// A partial sale writes off 2 × 103 = 206 and nets 260 − 3 − 1 = 256.
		f.sellWithCosts(f.custodyID, positionID, day(7), "260", "2", "3", "1")
		sep := f.monthlyMovements(nil)["2026-09"]
		if sep.RealizedGain.String() != "50" || sep.Fees.String() != "15" || sep.Taxes.String() != "7" || !sep.Income.IsZero() {
			t.Fatalf("after the partial sale: %+v", sep)
		}
		f.assertCash("after the partial sale", "1638")

		// The rest (4 × 103 = 412) nets 380 − 5 = 375: a loss.
		f.sellWithCosts(f.custodyID, positionID, october(10), "380", "4", "5", "0")
		months := f.monthlyMovements(nil)
		if len(months) != 2 {
			t.Fatalf("months = %+v", months)
		}
		oct := months["2026-10"]
		if oct.RealizedGain.String() != "-37" || oct.Fees.String() != "5" || !oct.Taxes.IsZero() || !oct.Income.IsZero() {
			t.Fatalf("after the full sale: %+v", oct)
		}
		if months["2026-09"].RealizedGain.String() != "50" {
			t.Fatalf("a later sale changed september: %+v", months["2026-09"])
		}

		// Nothing is deducted twice: with the holding sold out, the realized gains
		// are exactly what the cash made, 256 + 375 received for 618 paid. Fees
		// and taxes keep itemizing the costs inside it.
		if got := sep.RealizedGain.Add(oct.RealizedGain); got.String() != "13" {
			t.Fatalf("realized gain over the holding's life = %s, want 13", got)
		}
		f.assertCash("after the full sale", "2013")

		// The gain is informative: it reaches neither the manual reporting entries
		// nor the income they feed.
		if reporting := f.manualReporting(); !reporting.Income.IsZero() || reporting.Fees.String() != "20" || reporting.Taxes.String() != "7" {
			t.Fatalf("manual reporting = %+v", reporting)
		}
		// The deposit plus the fee and tax of the purchase and of the sales.
		if reported := f.reportedByID(); len(reported) != 6 {
			t.Fatalf("reported entries = %+v, want only the deposit and five costs", reported)
		}
	})
}

// A transferred holding carries its frozen all-in cost, which is what a later
// sale at the destination writes off; the transfer itself realizes nothing.
func TestMonthlyRealizedGainOfATransferredHoldingUsesTheFrozenBasis(t *testing.T) {
	eachLedger(t, func(t *testing.T, f *ledgerFixture) {
		destination, err := investments.CreateAccount(f.ctx, f.conn, investments.AccountInput{Name: "Carteira"})
		if err != nil {
			t.Fatal(err)
		}
		ticker := "ABCD3"
		source, err := investments.CreatePosition(f.ctx, f.conn, investments.PositionInput{AccountID: f.custodyID, Name: "Ações", Ticker: &ticker, AssetType: "Ação"})
		if err != nil {
			t.Fatal(err)
		}
		target, err := investments.CreatePosition(f.ctx, f.conn, investments.PositionInput{AccountID: destination.ID, AssetID: source.AssetID, Name: "Ações", AssetType: "Ação"})
		if err != nil {
			t.Fatal(err)
		}
		f.deposit("2000", day(1))
		f.buyWithCosts(f.custodyID, source.ID, day(1), "1000", "10", "20", "0") // basis 1020
		if _, err := investments.CreateTransfer(f.ctx, f.conn, investments.TransferInput{SourcePositionID: source.ID, DestinationPositionID: target.ID, Quantity: dec("10"), OccurredOn: day(5)}); err != nil {
			t.Fatalf("CreateTransfer: %v", err)
		}
		if got := f.monthlyMovements(nil)["2026-09"]; !got.RealizedGain.IsZero() || !got.Income.IsZero() {
			t.Fatalf("the transfer realized something: %+v", got)
		}

		// 4 of 10 units write off 408 of the carried 1020 and net 500 − 2 = 498.
		f.sellWithCosts(destination.ID, target.ID, day(10), "500", "4", "2", "0")
		if got := f.monthlyMovements(nil)["2026-09"]; got.RealizedGain.String() != "90" || got.Fees.String() != "22" {
			t.Fatalf("all accounts: %+v", got)
		}
		if got := f.monthlyMovements(&destination.ID)["2026-09"]; got.RealizedGain.String() != "90" || got.Fees.String() != "2" {
			t.Fatalf("destination: %+v", got)
		}
		if got := f.monthlyMovements(&f.custodyID)["2026-09"]; !got.RealizedGain.IsZero() || got.Fees.String() != "20" {
			t.Fatalf("source: %+v", got)
		}
	})
}

// Appreciation moves net worth and the unrealized gain, never the realized
// gain or income, whatever the position is valued at when it is sold.
func TestUnrealizedGainStaysOutOfRealizedGainAndIncome(t *testing.T) {
	eachLedger(t, func(t *testing.T, f *ledgerFixture) {
		positionID := f.addPosition(f.custodyID, "Ações")
		f.deposit("1000", day(1))
		f.buy(positionID, day(2), "600", "6")
		f.valuation(positionID, day(3), "900")

		if got := f.unrealizedGain(); got != "300" {
			t.Fatalf("unrealized gain = %s, want 300", got)
		}
		if got := f.monthlyMovements(nil)["2026-09"]; !got.RealizedGain.IsZero() || !got.Income.IsZero() {
			t.Fatalf("valuation leaked into the monthly flows: %+v", got)
		}

		// Sold for 150 against a basis of 100, although valued at 150 a unit.
		f.sellWithCosts(f.custodyID, positionID, day(7), "150", "1", "0", "0")
		f.valuation(positionID, day(8), "5000")
		got := f.monthlyMovements(nil)["2026-09"]
		if got.RealizedGain.String() != "50" || !got.Income.IsZero() {
			t.Fatalf("after the sale: %+v", got)
		}
		if got := f.unrealizedGain(); got != "4500" {
			t.Fatalf("unrealized gain after the sale = %s, want 4500 (5000 − 500)", got)
		}
	})
}

// The basis written off is read from the ledger itself, so selling a holding
// unit by unit realizes exactly what it earned, with no drift from an
// inexact unit cost.
func TestMonthlyRealizedGainOfSalesOneUnitAtATimeAddsUpExactly(t *testing.T) {
	eachLedger(t, func(t *testing.T, f *ledgerFixture) {
		positionID := f.addPosition(f.custodyID, "Fundo")
		f.deposit("100", day(1))
		f.buy(positionID, day(2), "100", "3")
		for n := 5; n <= 7; n++ {
			f.sellWithCosts(f.custodyID, positionID, day(n), "40", "1", "0", "0")
		}
		if got := f.monthlyMovements(nil)["2026-09"].RealizedGain; got.String() != "20" {
			t.Fatalf("realized gain = %s, want exactly 20 (120 received for 100 paid)", got)
		}
	})
}

// A manual holding inside an integrated account realizes its own gain; the
// provider's cash and positions play no part.
func TestMonthlyRealizedGainOfAManualHoldingInAnIntegratedAccount(t *testing.T) {
	f := newBookFixture(t)
	f.addFinancialInvestment("2000", "4", "500")
	integrated := f.integratedAccountID()
	manual := f.openBitcoin(integrated, "0.5", "100000")

	if _, err := investments.CreateOperation(context.Background(), f.conn, investments.OperationInput{
		AccountID: integrated, PositionID: &manual.ID, Kind: investments.OperationSell,
		OccurredOn: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), Amount: dec("24000"), Quantity: decRef("0.2"), Fees: dec("100"),
	}); err != nil {
		t.Fatalf("CreateOperation: %v", err)
	}
	months, err := investments.MonthlyMovements(context.Background(), f.conn, &integrated)
	if err != nil {
		t.Fatal(err)
	}
	// 0.2 of the 0.5 held writes off 20000 of the 50000 paid, against 23900 net.
	if len(months) != 1 || months[0].RealizedGain.String() != "3900" || !months[0].Income.IsZero() {
		t.Fatalf("months = %+v", months)
	}
}

// A position's Rendimento since inception (V(end) − net money put in) splits
// into what its sales realized and what it still holds unrealized, to the
// cent. Income operations belong to the account, not to the position, so they
// are in neither the yield nor the realized gain.
func TestPositionYieldIsRealizedGainPlusUnrealizedGain(t *testing.T) {
	eachLedger(t, func(t *testing.T, f *ledgerFixture) {
		positionID := f.addPosition(f.custodyID, "Ações")
		f.deposit("3000", day(1))
		f.buyWithCosts(f.custodyID, positionID, day(2), "600", "6", "12", "6") // basis 618
		f.valuation(positionID, day(3), "900")
		f.sellWithCosts(f.custodyID, positionID, day(7), "260", "2", "3", "1") // writes off 206
		f.buyWithCosts(f.custodyID, positionID, day(8), "330", "3", "5", "0")  // basis 412 + 335 = 747 over 7 units
		f.create(investments.OperationInput{AccountID: f.custodyID, Kind: investments.OperationIncome, OccurredOn: day(9), Amount: dec("40")})
		f.sellWithCosts(f.custodyID, positionID, day(10), "120", "1", "2", "0") // writes off 747 / 7, which does not terminate
		f.valuation(positionID, day(12), "1000")

		realized := dec("0")
		for _, month := range f.monthlyMovements(nil) {
			realized = realized.Add(month.RealizedGain)
		}
		position, err := investments.GetPosition(f.ctx, f.conn, positionID)
		if err != nil || position.TotalCost == nil {
			t.Fatalf("GetPosition = %+v, %v", position, err)
		}
		unrealized := position.CurrentValue.Sub(*position.TotalCost)
		yield := f.yield(positionID, nil, "2026-09-15")
		if got := realized.Add(unrealized); !got.Equal(yield.Value) {
			t.Fatalf("yield %s != realized %s + unrealized %s", yield.Value, realized, unrealized)
		}
		if f.monthlyMovements(nil)["2026-09"].Income.String() != "40" {
			t.Fatalf("income operation is not reported as income")
		}
	})
}

// The split also holds for a holding that arrived by custody transfer, whose
// frozen cost is both what the yield counts as money in and the basis a sale
// writes off.
func TestTransferredPositionYieldIsRealizedGainPlusUnrealizedGain(t *testing.T) {
	eachLedger(t, func(t *testing.T, f *ledgerFixture) {
		destination, err := investments.CreateAccount(f.ctx, f.conn, investments.AccountInput{Name: "Carteira"})
		if err != nil {
			t.Fatal(err)
		}
		ticker := "ABCD3"
		source, err := investments.CreatePosition(f.ctx, f.conn, investments.PositionInput{AccountID: f.custodyID, Name: "Ações", Ticker: &ticker, AssetType: "Ação"})
		if err != nil {
			t.Fatal(err)
		}
		target, err := investments.CreatePosition(f.ctx, f.conn, investments.PositionInput{AccountID: destination.ID, AssetID: source.AssetID, Name: "Ações", AssetType: "Ação"})
		if err != nil {
			t.Fatal(err)
		}
		f.deposit("2000", day(1))
		f.buyWithCosts(f.custodyID, source.ID, day(1), "1000", "10", "20", "0") // basis 1020
		if _, err := investments.CreateTransfer(f.ctx, f.conn, investments.TransferInput{SourcePositionID: source.ID, DestinationPositionID: target.ID, Quantity: dec("10"), OccurredOn: day(5)}); err != nil {
			t.Fatalf("CreateTransfer: %v", err)
		}
		f.sellWithCosts(destination.ID, target.ID, day(10), "500", "4", "2", "0")
		f.create(investments.OperationInput{AccountID: destination.ID, PositionID: &target.ID, Kind: investments.OperationValuation, OccurredOn: day(11), Amount: dec("700")})

		realized := f.monthlyMovements(&destination.ID)["2026-09"].RealizedGain
		position, err := investments.GetPosition(f.ctx, f.conn, target.ID)
		if err != nil || position.TotalCost == nil {
			t.Fatalf("GetPosition = %+v, %v", position, err)
		}
		unrealized := position.CurrentValue.Sub(*position.TotalCost)
		yield := f.yield(target.ID, nil, "2026-09-15")
		if got := realized.Add(unrealized); !got.Equal(yield.Value) {
			t.Fatalf("yield %s != realized %s + unrealized %s", yield.Value, realized, unrealized)
		}
	})
}
