import { Input, Select, Switch } from "antd";

import type {
  InvestmentAssetClass,
  InvestmentAssetClassDefinition,
  InvestmentAssetWrite,
} from "../../api/contracts";
import { FormField } from "../forms/FormField";

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

/**
 * The asset's classification, identity and quote fields, shared by the asset
 * registry and the position form. Choosing a quotable type turns the
 * automatic quote on; the switch stays as the override.
 */
export function InvestmentAssetFields({
  draft,
  onChange,
  classification,
  quoteMarket,
}: {
  draft: AssetDraft;
  onChange: (update: (current: AssetDraft) => AssetDraft) => void;
  classification: InvestmentAssetClassDefinition[];
  quoteMarket: string | null;
}) {
  const classDefinition = classification.find((item) => item.asset_class === draft.assetClass);
  const quoteSource = draft.quoteEnabled ? quoteMarket : null;
  const classOptions = classification.map((item) => ({ value: item.asset_class, label: item.label }));
  const typeOptions = [
    ...(classDefinition?.types.map((item) => ({ value: item.name, label: item.name })) ?? []),
    { value: customAssetType, label: "Outro tipo" },
  ];

  return (
    <>
      <FormField label="Classe" htmlFor="investment-asset-class">
        <Select
          id="investment-asset-class"
          value={draft.assetClass || undefined}
          placeholder="Selecione a classe financeira"
          options={classOptions}
          onChange={(value: InvestmentAssetClass) =>
            onChange((current) => ({
              ...current,
              assetClass: value,
              assetType: "",
              customType: "",
              quoteEnabled: false,
              quoteSymbol: "",
            }))
          }
        />
      </FormField>
      <FormField label="Tipo" htmlFor="investment-asset-type">
        <Select
          id="investment-asset-type"
          value={draft.assetType || undefined}
          disabled={!draft.assetClass}
          placeholder="Selecione o tipo de instrumento"
          options={typeOptions}
          showSearch
          optionFilterProp="label"
          onChange={(value: string) =>
            onChange((current) => ({
              ...current,
              assetType: value,
              customType: "",
              quoteEnabled: Boolean(classDefinition?.types.find((item) => item.name === value)?.quote_market),
              quoteSymbol: "",
            }))
          }
        />
      </FormField>
      {draft.assetType === customAssetType && (
        <FormField label="Descrição do tipo" htmlFor="investment-asset-custom-type">
          <Input
            id="investment-asset-custom-type"
            value={draft.customType}
            onChange={(event) => onChange((current) => ({ ...current, customType: event.target.value }))}
          />
        </FormField>
      )}
      <FormField label="Nome" htmlFor="investment-asset-name">
        <Input
          id="investment-asset-name"
          value={draft.name}
          placeholder="Ex.: Tesouro Selic 2029"
          onChange={(event) => onChange((current) => ({ ...current, name: event.target.value }))}
        />
      </FormField>
      <FormField label={quoteSource ? "Código" : "Código (opcional)"} htmlFor="investment-asset-ticker">
        <Input
          id="investment-asset-ticker"
          value={draft.ticker}
          placeholder="Ex.: PETR4"
          onChange={(event) => onChange((current) => ({ ...current, ticker: event.target.value }))}
        />
      </FormField>
      <FormField label="Moeda" htmlFor="investment-asset-currency">
        <Input
          id="investment-asset-currency"
          value={draft.currencyCode}
          maxLength={3}
          placeholder="BRL"
          onChange={(event) => onChange((current) => ({ ...current, currencyCode: event.target.value.toUpperCase() }))}
        />
      </FormField>
      <FormField
        label="Cotação automática"
        htmlFor="investment-asset-quote-enabled"
        hint={
          quoteMarket
            ? "Atualiza o preço e busca o histórico do ativo em reais."
            : "Para este tipo, registre avaliações manualmente ou use os valores da instituição integrada."
        }
      >
        <div>
          <Switch
            id="investment-asset-quote-enabled"
            checked={draft.quoteEnabled && Boolean(quoteMarket)}
            disabled={!quoteMarket}
            onChange={(checked) => onChange((current) => ({ ...current, quoteEnabled: checked }))}
          />
        </div>
      </FormField>
      {quoteSource && (
        <details>
          <summary>Código alternativo para cotação</summary>
          <FormField
            label="Código para cotação (opcional)"
            htmlFor="investment-asset-quote-symbol"
            hint="Em branco, usa o código do ativo. Preencha somente quando o código cotado for diferente."
          >
            <Input
              id="investment-asset-quote-symbol"
              value={draft.quoteSymbol}
              placeholder={quoteSource === "crypto" ? "BTC" : "PETR4"}
              onChange={(event) => onChange((current) => ({ ...current, quoteSymbol: event.target.value }))}
            />
          </FormField>
        </details>
      )}
    </>
  );
}
