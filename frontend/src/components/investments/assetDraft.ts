import type { MessageKey } from "../../i18n/messages";
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
  t: (key: MessageKey) => string,
): InvestmentAssetWrite | string {
  const name = draft.name.trim();
  const assetType = (draft.assetType === customAssetType ? draft.customType : draft.assetType).trim();
  const currencyCode = draft.currencyCode.trim().toUpperCase();
  if (name === "") return t("assetNameRequired");
  if (draft.assetClass === "") return t("assetClassRequired");
  if (assetType === "") return t("assetTypeRequired");
  if (!/^[A-Z]{3}$/.test(currencyCode)) return t("assetCurrencyInvalid");
  if (quoteSource && quoteSymbol === "" && draft.ticker.trim() === "") {
    return t("assetTickerForQuote");
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
