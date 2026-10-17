package main

import (
	"bytes"
	"context"
	"log"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/investments"
)

func TestOpenDatabaseSeedsCatalogOnEveryStartup(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "julius.db")
	var logs bytes.Buffer
	previousOutput := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previousOutput) })

	conn, err := openDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	assets, err := investments.ListAssets(ctx, conn)
	if err != nil || len(assets) != 27 {
		t.Fatalf("fresh catalog = %d assets, %v", len(assets), err)
	}
	byTicker := map[string]investments.Asset{}
	for _, asset := range assets {
		byTicker[*asset.Ticker] = asset
	}
	for _, ticker := range []string{"BTC", "ETH", "SOL", "USDC", "B3SA3"} {
		if _, ok := byTicker[ticker]; !ok {
			t.Errorf("missing starter asset %s", ticker)
		}
	}
	if !strings.Contains(logs.String(), "27 new assets inserted") {
		t.Fatalf("fresh startup log = %s", logs.String())
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	logs.Reset()
	conn, err = openDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := investments.ListAssets(ctx, conn)
	if err != nil || !reflect.DeepEqual(assets, repeated) {
		t.Fatalf("repeated startup changed catalog: %v", err)
	}
	if !strings.Contains(logs.String(), "0 new assets inserted") {
		t.Fatalf("repeated startup log = %s", logs.String())
	}

	btc := byTicker["BTC"]
	customSymbol := "ETH"
	customized, err := investments.UpdateAsset(ctx, conn, btc.ID, investments.AssetInput{
		Name: "Meu ativo", Ticker: btc.Ticker, AssetType: "Personalizado",
		AssetClass: investments.AssetClassOther, CurrencyCode: "USD",
		QuoteSource: btc.QuoteSource, QuoteSymbol: &customSymbol,
	})
	if err != nil {
		t.Fatal(err)
	}
	petr := byTicker["PETR4"]
	disabled, err := investments.UpdateAsset(ctx, conn, petr.ID, investments.AssetInput{
		Name: "Minha Petrobras", Ticker: petr.Ticker, AssetType: petr.AssetType,
		AssetClass: petr.AssetClass, CurrencyCode: petr.CurrencyCode,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a database from a version whose catalog did not include SOL.
	if err := investments.DeleteAsset(ctx, conn, byTicker["SOL"].ID); err != nil {
		t.Fatal(err)
	}
	before, err := investments.ListAssets(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	logs.Reset()
	conn, err = openDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range before {
		got, err := investments.GetAsset(ctx, conn, want.ID)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("existing asset %s changed: %+v, %v (customized %s, disabled %s)", want.ID, got, err, customized.ID, disabled.ID)
		}
	}
	after, err := investments.ListAssets(ctx, conn)
	if err != nil || len(after) != len(before)+1 {
		t.Fatalf("updated catalog = %d assets, %v", len(after), err)
	}
	if _, err := investments.GetAsset(ctx, conn, byTicker["SOL"].ID); err != nil {
		t.Fatalf("missing catalog entry was not inserted: %v", err)
	}
	if !strings.Contains(logs.String(), "1 new assets inserted") {
		t.Fatalf("catalog update log = %s", logs.String())
	}
	for _, table := range []string{
		"investment_positions", "investment_accounts", "investment_operations",
		"financial_accounts", "financial_transactions", "financial_investments",
		"financial_investment_transactions", "investment_asset_quotes", "investment_asset_quote_coverage",
	} {
		var count int
		if err := conn.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s = %d, %v; want no financial or market data", table, count, err)
		}
	}
}

func TestOpenDatabasePreservesPreexistingAsset(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "julius.db")
	conn, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	ticker, market, symbol := "BTC", "crypto", "ETH"
	want, err := investments.CreateAsset(ctx, conn, investments.AssetInput{
		Name: "Meu Bitcoin", Ticker: &ticker, AssetType: "Personalizado",
		AssetClass: investments.AssetClassOther, CurrencyCode: "USD",
		QuoteSource: &market, QuoteSymbol: &symbol,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	conn, err = openDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := investments.GetAsset(ctx, conn, want.ID)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("preexisting asset changed: %+v, %v", got, err)
	}
}
