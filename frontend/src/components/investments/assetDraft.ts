import type {
  InvestmentAssetClass,
  InvestmentAssetClassDefinition,
  InvestmentAssetWrite,
} from "../../api/contracts";

/** The "Tipo" select's escape hatch: the instrument is not in the class's catalog. */
export const customAssetType = "__custom";

export type AssetDraft = {
  name: string;
  ticker: string;
  assetClass: InvestmentAssetClass | "";
  assetType: string;
  customType: string;
  currencyCode: string;
  quoteEnabled: boolean;
  quoteSymbol: string;
};

export const emptyAssetDraft: AssetDraft = {
  name: "",
  ticker: "",
  assetClass: "",
  assetType: "",
  customType: "",
  currencyCode: "BRL",
  quoteEnabled: false,
  quoteSymbol: "",
};

/** The quote market the catalog assigns to the draft's class and type. */
export function catalogQuoteMarket(draft: AssetDraft, classification: InvestmentAssetClassDefinition[]): string | null {
  return (
    classification
      .find((item) => item.asset_class === draft.assetClass)
      ?.types.find((item) => item.name === draft.assetType)?.quote_market ?? null
  );
}

/** Validates the draft and builds the write, or returns the message to show. */
export function assetWriteFrom(
  draft: AssetDraft,
  quoteSource: string | null,
  quoteSymbol: string,
): InvestmentAssetWrite | string {
  const name = draft.name.trim();
  const assetType = (draft.assetType === customAssetType ? draft.customType : draft.assetType).trim();
  const currencyCode = draft.currencyCode.trim().toUpperCase();
  if (name === "") return "Informe o nome do ativo.";
  if (draft.assetClass === "") return "Escolha a classe do ativo.";
  if (assetType === "") return "Informe o tipo do ativo.";
  if (!/^[A-Z]{3}$/.test(currencyCode)) return "Informe a moeda com três letras, como BRL ou USD.";
  if (quoteSource && quoteSymbol === "" && draft.ticker.trim() === "") {
    return "Informe o código do ativo, como PETR4 ou BTC, para buscar a cotação.";
  }
  return {
    name,
    ticker: draft.ticker.trim() || null,
    asset_type: assetType,
    asset_class: draft.assetClass,
    currency_code: currencyCode,
    quote_source: quoteSource || null,
    quote_symbol: quoteSymbol || null,
  };
}
