import { useTranslation } from "../../i18n/context";
import { Input, Select, Switch } from "../ui";

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
  const { t } = useTranslation();
  const classDefinition = classification.find((item) => item.asset_class === draft.assetClass);
  const quoteSource = draft.quoteEnabled ? quoteMarket : null;
  const classOptions = classification.map((item) => ({ value: item.asset_class, label: item.label }));
  const typeOptions = [
    ...(classDefinition?.types.map((item) => ({ value: item.name, label: item.name })) ?? []),
    { value: customAssetType, label: t("otherType") },
  ];

  return (
    <>
      <FormField label={t("classLabel")} htmlFor="investment-asset-class">
        <Select
          id="investment-asset-class"
          value={draft.assetClass || undefined}
          placeholder={t("selectFinancialClass")}
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
      <FormField label={t("type")} htmlFor="investment-asset-type">
        <Select
          id="investment-asset-type"
          value={draft.assetType || undefined}
          disabled={!draft.assetClass}
          placeholder={t("selectInstrumentType")}
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
        <FormField label={t("typeDescription")} htmlFor="investment-asset-custom-type">
          <Input
            id="investment-asset-custom-type"
            value={draft.customType}
            onChange={(event) => onChange((current) => ({ ...current, customType: event.target.value }))}
          />
        </FormField>
      )}
      <FormField label={t("name")} htmlFor="investment-asset-name">
        <Input
          id="investment-asset-name"
          value={draft.name}
          placeholder={t("assetNameExample")}
          onChange={(event) => onChange((current) => ({ ...current, name: event.target.value }))}
        />
      </FormField>
      <FormField label={quoteSource ? t("codeLabel") : t("codeOptional")} htmlFor="investment-asset-ticker">
        <Input
          id="investment-asset-ticker"
          value={draft.ticker}
          placeholder={t("tickerExample")}
          onChange={(event) => onChange((current) => ({ ...current, ticker: event.target.value }))}
        />
      </FormField>
      <FormField label={t("currency")} htmlFor="investment-asset-currency">
        <Input
          id="investment-asset-currency"
          value={draft.currencyCode}
          maxLength={3}
          placeholder="BRL"
          onChange={(event) => onChange((current) => ({ ...current, currencyCode: event.target.value.toUpperCase() }))}
        />
      </FormField>
      <FormField
        label={t("autoQuote")}
        htmlFor="investment-asset-quote-enabled"
        hint={
          quoteMarket
            ? t("autoQuoteHint")
            : t("manualQuoteHint")
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
          <summary>{t("altQuoteCode")}</summary>
          <FormField
            label={t("quoteCodeOptional")}
            htmlFor="investment-asset-quote-symbol"
            hint={t("quoteCodeHint")}
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
