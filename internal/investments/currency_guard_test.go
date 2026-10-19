package investments_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/investments"
)

// foreignAsset is a catalog entry in dollars. It is made by opening and
// closing a holding of it, then re-quoting the asset, because that is how the
// catalog gets entries on every dialect.
func (f *ledgerFixture) foreignAsset(name, ticker string) investments.Asset {
	f.t.Helper()
	position, err := investments.CreatePosition(f.ctx, f.conn, investments.PositionInput{AccountID: f.custodyID, Name: name, Ticker: &ticker, AssetType: "Ação"})
	if err != nil {
		f.t.Fatalf("CreatePosition: %v", err)
	}
	f.exec(`UPDATE investment_assets SET currency_code = 'USD' WHERE id = ?`, *position.AssetID)
	if err := investments.DeletePosition(f.ctx, f.conn, position.ID); err != nil {
		f.t.Fatalf("DeletePosition: %v", err)
	}
	asset, err := investments.GetAsset(f.ctx, f.conn, *position.AssetID)
	if err != nil {
		f.t.Fatal(err)
	}
	return asset
}

// insertLegacyPosition writes a manual holding of an asset the way a database
// from before the guard could hold one, bypassing it.
func (f *ledgerFixture) insertLegacyPosition(accountID, assetID string) string {
	f.t.Helper()
	id, now := uuid.NewString(), db.FormatTime(time.Now())
	f.exec(`INSERT INTO investment_positions (id, account_id, asset_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		id, accountID, assetID, now, now)
	return id
}

func TestManualPositionRefusesAnAssetInAnotherCurrency(t *testing.T) {
	eachLedger(t, func(t *testing.T, f *ledgerFixture) {
		usd := f.foreignAsset("Apple", "AAPL")
		ticker := "AAPL"
		f.addInvestmentMovement()
		integrated := "integrated:" + f.sourceID
		if _, err := investments.ListAccounts(f.ctx, f.conn); err != nil {
			t.Fatal(err)
		}

		for label, in := range map[string]investments.PositionInput{
			"catalog asset":      {AccountID: f.custodyID, AssetID: &usd.ID, Name: "Apple", AssetType: "Ação"},
			"same identity":      {AccountID: f.custodyID, Name: "Apple", Ticker: &ticker, AssetType: "Ação"},
			"integrated custody": {AccountID: integrated, AssetID: &usd.ID, Name: "Apple", AssetType: "Ação"},
		} {
			if _, err := investments.CreatePosition(f.ctx, f.conn, in); !errors.Is(err, investments.ErrUnsupportedCurrency) {
				t.Fatalf("%s: CreatePosition error = %v, want ErrUnsupportedCurrency", label, err)
			}
		}
		manual := string(investments.PositionSourceManual)
		positions, err := investments.ListPositions(f.ctx, f.conn, investments.PositionFilter{Source: &manual})
		if err != nil || len(positions) != 0 {
			t.Fatalf("positions = %+v, %v; want none", positions, err)
		}

		// Reais stay accepted, in either kind of custody.
		for _, accountID := range []string{f.custodyID, integrated} {
			if _, err := investments.CreatePosition(f.ctx, f.conn, investments.PositionInput{AccountID: accountID, Name: "Petrobras PN", AssetType: "Ação"}); err != nil {
				t.Fatalf("BRL position in %s: %v", accountID, err)
			}
		}
	})
}

func TestAssetCurrencyCannotChangeUnderManualHoldings(t *testing.T) {
	eachLedger(t, func(t *testing.T, f *ledgerFixture) {
		held, err := investments.CreatePosition(f.ctx, f.conn, investments.PositionInput{AccountID: f.custodyID, Name: "Petrobras PN", AssetType: "Ação"})
		if err != nil {
			t.Fatal(err)
		}
		asset, err := investments.GetAsset(f.ctx, f.conn, *held.AssetID)
		if err != nil {
			t.Fatal(err)
		}
		change := func(a investments.Asset, currency string) error {
			_, err := investments.UpdateAsset(f.ctx, f.conn, a.ID, investments.AssetInput{
				Name: a.Name, Ticker: a.Ticker, AssetType: a.AssetType, AssetClass: a.AssetClass, CurrencyCode: currency,
			})
			return err
		}
		if err := change(asset, "USD"); !errors.Is(err, investments.ErrUnsupportedCurrency) {
			t.Fatalf("currency change under a manual holding = %v, want ErrUnsupportedCurrency", err)
		}
		if got, _ := investments.GetAsset(f.ctx, f.conn, asset.ID); got.CurrencyCode != "BRL" {
			t.Fatalf("asset currency = %s after the refusal", got.CurrencyCode)
		}

		// A catalog entry nobody holds manually may be in any currency, and a
		// rename keeps working under a holding.
		loose := f.foreignAsset("Apple", "AAPL")
		if err := change(loose, "EUR"); err != nil {
			t.Fatalf("currency change without holdings: %v", err)
		}
		if err := change(investments.Asset{ID: asset.ID, Name: "Petrobras ON", Ticker: asset.Ticker, AssetType: asset.AssetType, AssetClass: asset.AssetClass}, "BRL"); err != nil {
			t.Fatalf("rename under a holding: %v", err)
		}
	})
}

// A holding that predates the guard stays readable and deletable; only new
// writes that would add reais to a foreign asset are refused.
func TestLegacyForeignHoldingIsOnlyRefusedNewWrites(t *testing.T) {
	eachLedger(t, func(t *testing.T, f *ledgerFixture) {
		usd := f.foreignAsset("Apple", "AAPL")
		legacy := f.insertLegacyPosition(f.custodyID, usd.ID)
		destination, err := investments.CreateAccount(f.ctx, f.conn, investments.AccountInput{Name: "Carteira"})
		if err != nil {
			t.Fatal(err)
		}
		other := f.insertLegacyPosition(destination.ID, usd.ID)
		f.deposit("1000", day(1))

		_, err = investments.CreateOperation(f.ctx, f.conn, investments.OperationInput{
			AccountID: f.custodyID, PositionID: &legacy, Kind: investments.OperationBuy, OccurredOn: day(2),
			Amount: dec("100"), Quantity: decRef("1"),
		})
		if !errors.Is(err, investments.ErrUnsupportedCurrency) {
			t.Fatalf("buy of a foreign asset = %v, want ErrUnsupportedCurrency", err)
		}
		_, err = investments.CreateTransfer(f.ctx, f.conn, investments.TransferInput{
			SourcePositionID: legacy, DestinationPositionID: other, Quantity: dec("1"), OccurredOn: day(3),
		})
		if !errors.Is(err, investments.ErrUnsupportedCurrency) {
			t.Fatalf("transfer of a foreign asset = %v, want ErrUnsupportedCurrency", err)
		}

		// Reads and cash records are unchanged.
		if _, err := investments.GetPosition(f.ctx, f.conn, legacy); err != nil {
			t.Fatalf("legacy position unreadable: %v", err)
		}
		if got := f.monthlyMovements(nil)["2026-09"]; got.Contributions.String() != "1000" {
			t.Fatalf("monthly movements = %+v", got)
		}
		if err := investments.DeletePosition(f.ctx, f.conn, legacy); err != nil {
			t.Fatalf("legacy position cannot be deleted: %v", err)
		}
	})
}

// A provider holding in another currency never writes its price into a
// catalog asset of the same ticker that is held in reais.
func TestSyncedHoldingInAnotherCurrencyDoesNotQuoteAReaisAsset(t *testing.T) {
	eachLedger(t, func(t *testing.T, f *ledgerFixture) {
		ticker, source := "BTC", "crypto"
		reais, err := investments.CreateAsset(f.ctx, f.conn, investments.AssetInput{Name: "Bitcoin", Ticker: &ticker, AssetType: "Criptomoeda", QuoteSource: &source})
		if err != nil {
			t.Fatal(err)
		}
		holding := investments.SyncedHolding{
			ExternalID: "usd-btc", Name: strRef("BTC"), Code: strRef("BTC"), InvestmentType: strRef("CRYPTO"),
			CurrencyCode: strRef("USD"), Quantity: decRef("1"), Amount: decRef("60000"), AsOfDate: instant("2026-09-05T12:00:00Z"),
		}
		if err := investments.RecordSyncedQuotes(f.ctx, f.conn, holding, nil); err != nil {
			t.Fatalf("RecordSyncedQuotes: %v", err)
		}
		if got := f.quotes(reais.ID); len(got) != 0 {
			t.Fatalf("a USD price reached the BRL asset: %+v", got)
		}
		if _, found, err := investments.FindSyncedAsset(f.ctx, f.conn, holding); err != nil || found {
			t.Fatalf("FindSyncedAsset = found %v, err %v; want none for a holding in another currency", found, err)
		}

		// The same holding in reais still shares the asset.
		holding.CurrencyCode = strRef("BRL")
		if err := investments.RecordSyncedQuotes(f.ctx, f.conn, holding, nil); err != nil {
			t.Fatal(err)
		}
		if got := f.quotes(reais.ID); len(got) != 1 {
			t.Fatalf("quotes = %+v, want the BRL one", got)
		}
	})
}
