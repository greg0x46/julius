import { Input, Select, Switch } from "antd";

import type { InvestmentAssetClass, InvestmentAssetClassDefinition } from "../../api/contracts";
import { FormField } from "../forms/FormField";
import { customAssetType, type AssetDraft } from "./assetDraft";

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
