package investments

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/marketdata"
)

// This is a starter catalog, not a portfolio or an investment recommendation.
// Symbols and financial exposure are documented in the investment reference.
var starterAssets = []struct{ name, ticker, kind string }{
	{"Petrobras PN", "PETR4", "Ação"},
	{"Petrobras ON", "PETR3", "Ação"},
	{"Vale ON", "VALE3", "Ação"},
	{"Itaú Unibanco PN", "ITUB4", "Ação"},
	{"Bradesco PN", "BBDC4", "Ação"},
	{"Banco do Brasil ON", "BBAS3", "Ação"},
	{"B3 ON", "B3SA3", "Ação"},
	{"PRIO ON", "PRIO3", "Ação"},
	{"BTG Pactual Unit", "BPAC11", "Unit"},
	{"Kinea Rendimentos Imobiliários", "KNCR11", "FII"},
	{"Maxi Renda", "MXRF11", "FII"},
	{"HGLG Logística", "HGLG11", "FII"},
	{"XP Malls", "XPML11", "FII"},
	{"iShares Ibovespa", "BOVA11", "ETF de ações"},
	{"iShares Small Cap", "SMAL11", "ETF de ações"},
	{"iShares S&P 500", "IVVB11", "ETF de ações"},
	{"Investo Tesouro 760 dias", "LFTB11", "ETF de renda fixa"},
	{"Investo Teva Tesouro Selic", "LFTS11", "ETF de renda fixa"},
	{"Hashdex Nasdaq Crypto Index", "HASH11", "ETF de criptoativos"},
	{"Apple BDR", "AAPL34", "BDR de ação"},
	{"Microsoft BDR", "MSFT34", "BDR de ação"},
	{"Amazon BDR", "AMZO34", "BDR de ação"},
	{"NVIDIA BDR", "NVDC34", "BDR de ação"},
	{"Bitcoin", "BTC", "Criptomoeda"},
	{"Ether", "ETH", "Criptomoeda"},
	{"Solana", "SOL", "Criptomoeda"},
	{"USD Coin", "USDC", "Stablecoin"},
}

func assetQuoteMarket(class AssetClass, kind string) string {
	for _, definition := range AssetClassification() {
		if definition.Class != class {
			continue
		}
		for _, candidate := range definition.Types {
			if candidate.Name == kind && candidate.QuoteMarket != nil {
				return *candidate.QuoteMarket
			}
		}
	}
	return ""
}

// SeedInvestmentAssets inserts missing catalog entries atomically. Existing
// names, classification and quote preferences are never overwritten.
func SeedInvestmentAssets(ctx context.Context, conn *sql.DB) (int, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	existing, err := ListAssets(ctx, tx)
	if err != nil {
		return 0, err
	}
	instruments := map[marketdata.Instrument]bool{}
	for _, asset := range existing {
		market := assetQuoteMarket(asset.AssetClass, asset.AssetType)
		if asset.QuoteSource != nil {
			market = *asset.QuoteSource
		}
		if market == "" {
			market = "b3"
			if asset.AssetClass == AssetClassCrypto {
				market = "crypto"
			}
		}
		symbol := asset.Ticker
		if asset.QuoteSymbol != nil {
			symbol = asset.QuoteSymbol
		}
		if symbol != nil {
			if instrument, err := marketdata.ParseInstrument(marketdata.Market(market), *symbol); err == nil {
				instruments[instrument] = true
			}
		}
	}
	now := db.FormatTime(time.Now())
	inserted := 0
	for _, entry := range starterAssets {
		class := InferAssetClass(entry.kind)
		market := assetQuoteMarket(class, entry.kind)
		instrument, err := marketdata.ParseInstrument(marketdata.Market(market), entry.ticker)
		if err != nil {
			return 0, fmt.Errorf("seed %s: %w", entry.ticker, err)
		}
		if instruments[instrument] {
			continue
		}
		in, key, err := normalizeAssetInput(AssetInput{
			Name: entry.name, Ticker: &entry.ticker, AssetType: entry.kind, AssetClass: class,
			CurrencyCode: "BRL", QuoteSource: &market, QuoteSymbol: &instrument.Symbol,
		})
		if err != nil {
			return 0, fmt.Errorf("seed %s: %w", entry.ticker, err)
		}
		id := uuid.NewSHA1(uuid.NameSpaceURL, []byte("contadinho:investment-asset:"+key)).String()
		result, err := tx.ExecContext(ctx, `
			INSERT INTO investment_assets
			(id, canonical_key, name, ticker, asset_type, asset_class, currency_code, quote_source, quote_symbol, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT DO NOTHING`,
			id, key, in.Name, nullableString(in.Ticker), in.AssetType, in.AssetClass,
			in.CurrencyCode, nullableString(in.QuoteSource), nullableString(in.QuoteSymbol), now, now)
		if err != nil {
			return 0, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		inserted += int(count)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return inserted, nil
}
