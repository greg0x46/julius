import { Button, DatePicker, Input, InputNumber, Select } from "antd";
import dayjs, { type Dayjs } from "dayjs";
import { useEffect, useMemo, useState } from "react";

import type {
  InvestmentAccount,
  InvestmentAsset,
  InvestmentPortfolio,
  InvestmentPosition,
  InvestmentPositionUpdate,
  InvestmentPositionWrite,
} from "../../api/contracts";
import { useInvestmentAssets } from "../../hooks/useInvestmentAssets";
import { errorMessage } from "../../presentation/errors";
import { FormDrawer } from "../forms/FormDrawer";
import { FormField } from "../forms/FormField";
import { MoneyInput } from "../forms/MoneyInput";
import {
  assetWriteFrom,
  catalogQuoteMarket,
  emptyAssetDraft,
  InvestmentAssetFields,
  type AssetDraft,
} from "./InvestmentAssetFields";
import { fromMoneyInput, toMoneyInput } from "./moneyDraft";

type Draft = {
  accountId: string;
  assetId: string | null;
  portfolioId: string | null;
  name: string;
  ticker: string;
  assetType: string;
  initialQuantity: string | null;
  initialUnitCost: string | null;
  initialValue: string | null;
  occurredOn: Dayjs;
  notes: string;
};

function blankDraft(accounts: InvestmentAccount[]): Draft {
  return {
    accountId: accounts.find((account) => account.kind === "manual" && account.active)?.id ?? "",
    assetId: null,
    portfolioId: null,
    name: "",
    ticker: "",
    assetType: "Ativo",
    initialQuantity: null,
    initialUnitCost: null,
    initialValue: null,
    occurredOn: dayjs(),
    notes: "",
  };
}

function draftFrom(position: InvestmentPosition | null, accounts: InvestmentAccount[]): Draft {
  if (!position) return blankDraft(accounts);
  return {
    accountId: position.account_id,
    assetId: position.asset_id,
    portfolioId: position.portfolio_id,
    name: position.name,
    ticker: position.ticker ?? "",
    assetType: position.asset_type,
    initialQuantity: null,
    initialUnitCost: null,
    initialValue: null,
    occurredOn: dayjs(),
    notes: position.notes ?? "",
  };
}

function positive(value: string | null): boolean {
  return value !== null && Number(value) > 0;
}

/** Case-, accent- and punctuation-insensitive form used for search and duplicates. */
function searchKey(value: string): string {
  return value.normalize("NFD").replace(/[^\p{L}\p{N}]/gu, "").toUpperCase();
}

function assetLabel(asset: InvestmentAsset): string {
  return [asset.ticker, asset.name].filter(Boolean).join(" · ");
}

/** Mirrors the backend's canonical key: the ticker when there is one, else type and name. */
function findDuplicate(assets: InvestmentAsset[], write: { name: string; ticker: string | null; asset_type: string }) {
  const ticker = searchKey(write.ticker ?? "");
  return assets.find((asset) =>
    ticker !== ""
      ? searchKey(asset.ticker ?? "") === ticker
      : searchKey(asset.ticker ?? "") === "" &&
        searchKey(asset.name) === searchKey(write.name) &&
        searchKey(asset.asset_type) === searchKey(write.asset_type),
  );
}

/** A short code-like search (BTC, PETR4) starts the new asset's ticker; anything else its name. */
function assetDraftFromSearch(search: string): AssetDraft {
  const text = search.trim();
  const looksLikeTicker = /^[A-Za-z0-9]+$/.test(text) && (/\d/.test(text) || text.length <= 5);
  return looksLikeTicker ? { ...emptyAssetDraft, ticker: text.toUpperCase() } : { ...emptyAssetDraft, name: text };
}

export function InvestmentPositionForm({
  open,
  position,
  accounts,
  portfolios,
  submitting,
  submitError,
  onCreate,
  onUpdate,
  onCancel,
}: {
  open: boolean;
  position: InvestmentPosition | null;
  accounts: InvestmentAccount[];
  portfolios: InvestmentPortfolio[];
  submitting: boolean;
  submitError: string | null;
  onCreate: (write: InvestmentPositionWrite) => void;
  onUpdate: (write: InvestmentPositionUpdate) => void;
  onCancel: () => void;
}) {
  const [draft, setDraft] = useState<Draft>(() => draftFrom(position, accounts));
  const [error, setError] = useState<string | null>(null);
  const [creatingAsset, setCreatingAsset] = useState(false);
  const [assetDraft, setAssetDraft] = useState<AssetDraft>(emptyAssetDraft);
  const [assetSearch, setAssetSearch] = useState("");
  const [savingAsset, setSavingAsset] = useState(false);
  const isEditing = position !== null;
  const catalog = useInvestmentAssets({ enabled: open && !isEditing });

  useEffect(() => {
    if (open) {
      setDraft(draftFrom(position, accounts));
      setError(null);
      setCreatingAsset(false);
      setAssetDraft(emptyAssetDraft);
      setAssetSearch("");
    }
  }, [open, position, accounts]);

  const editableAccounts = useMemo(
    () => accounts.filter((account) => account.kind === "manual" && account.active),
    [accounts],
  );
  const accountOptions = editableAccounts.map((account) => ({ value: account.id, label: account.name }));
  const portfolioOptions = portfolios.map((portfolio) => ({ value: portfolio.id, label: portfolio.name }));
  const assetOptions = catalog.assets.map((asset) => ({ value: asset.id, label: assetLabel(asset) }));
  const selectedAsset = catalog.assets.find((asset) => asset.id === draft.assetId);
  // With nothing in the catalog there is nothing to pick: the form opens on a new asset.
  const catalogEmpty = !catalog.isLoading && !catalog.error && catalog.assets.length === 0;
  const showNewAsset = creatingAsset || catalogEmpty;
  const assetQuoteMarket = catalogQuoteMarket(assetDraft, catalog.classification);
  const classLabel = (asset: InvestmentAsset) =>
    catalog.classification.find((item) => item.asset_class === asset.asset_class)?.label ?? asset.asset_class;

  const startNewAsset = () => {
    if (assetDraft === emptyAssetDraft) setAssetDraft(assetDraftFromSearch(assetSearch));
    setCreatingAsset(true);
  };
  const pickAsset = (asset: InvestmentAsset | undefined) => {
    setCreatingAsset(false);
    setDraft((current) => ({
      ...current,
      assetId: asset?.id ?? null,
      name: asset?.name ?? "",
      ticker: asset?.ticker ?? "",
      assetType: asset?.asset_type ?? "",
    }));
  };

  // A new asset is created first, through the same validation as the asset
  // registry, and the position then refers to it by id. Once created it
  // becomes the selection, so retrying a failed position never creates it twice.
  const createAsset = async (): Promise<InvestmentAsset | null> => {
    const quoteSource = assetDraft.quoteEnabled ? assetQuoteMarket : null;
    const write = assetWriteFrom(assetDraft, quoteSource, quoteSource ? assetDraft.quoteSymbol.trim() : "");
    if (typeof write === "string") {
      setError(write);
      return null;
    }
    const duplicate = findDuplicate(catalog.assets, write);
    if (duplicate) {
      pickAsset(duplicate);
      setError(`Já existe o ativo ${assetLabel(duplicate)} no catálogo. Ele foi selecionado; confira e salve.`);
      return null;
    }
    setSavingAsset(true);
    try {
      const asset = await catalog.createAsset(write);
      pickAsset(asset);
      setAssetDraft(emptyAssetDraft);
      return asset;
    } catch (failure) {
      setError(errorMessage(failure, "Não foi possível cadastrar o ativo."));
      return null;
    } finally {
      setSavingAsset(false);
    }
  };

  const submit = async () => {
    if (isEditing && draft.name.trim() === "") {
      setError("Informe o nome da posição.");
      return;
    }
    if (isEditing && draft.assetType.trim() === "") {
      setError("Informe o tipo do ativo.");
      return;
    }
    if (!isEditing && !showNewAsset && !selectedAsset) {
      setError("Escolha um ativo ou cadastre um novo.");
      return;
    }
    if (!isEditing && draft.accountId === "") {
      setError("Selecione uma conta de investimento manual.");
      return;
    }
    if ((draft.initialQuantity !== null && !positive(draft.initialQuantity)) || (draft.initialUnitCost !== null && !positive(draft.initialUnitCost))) {
      setError("Quantidade e custo unitário devem ser maiores que zero.");
      return;
    }
    if (!isEditing && ((draft.initialQuantity !== null || draft.initialUnitCost !== null) && !positive(draft.initialValue) && !(positive(draft.initialQuantity) && positive(draft.initialUnitCost)))) {
      setError("Para informar um saldo inicial, preencha quantidade e custo unitário, ou o valor inicial.");
      return;
    }
    setError(null);
    if (isEditing) {
      onUpdate({
        name: draft.name.trim(),
        ticker: draft.ticker.trim() || null,
        asset_type: draft.assetType.trim(),
        portfolio_id: draft.portfolioId,
        notes: draft.notes.trim() || null,
      });
      return;
    }
    const asset = showNewAsset ? await createAsset() : selectedAsset;
    if (!asset) return;
    onCreate({
      account_id: draft.accountId,
      asset_id: asset.id,
      name: asset.name,
      ticker: asset.ticker,
      asset_type: asset.asset_type,
      portfolio_id: draft.portfolioId,
      initial_quantity: draft.initialQuantity ?? undefined,
      initial_unit_cost: draft.initialUnitCost ?? undefined,
      initial_value: draft.initialValue ?? undefined,
      occurred_on: draft.occurredOn.format("YYYY-MM-DD"),
      notes: draft.notes.trim() || null,
    });
  };

  // Quantity and unit cost keep more than two decimals, so they stay plain
  // string-mode numbers; only the initial value is a money field.
  const setDecimal = (field: "initialQuantity" | "initialUnitCost", value: string | number | null) =>
    setDraft((current) => ({ ...current, [field]: value === null ? null : String(value) }));

  return (
    <FormDrawer
      title={isEditing ? "Editar posição manual" : "Nova posição manual"}
      open={open}
      onClose={onCancel}
      onSubmit={() => void submit()}
      submitting={submitting || savingAsset}
      error={error ?? submitError}
    >
      {!isEditing && (
        <p className="form-field-hint">
          Posição manual em reais. Ativos com cotação automática buscam preços de mercado. Para os demais, registre
          avaliações manuais; sem avaliação, o valor exibido é o custo acumulado.
        </p>
      )}
      <FormField
        label="Conta de investimento"
        htmlFor="investment-position-account"
        hint={isEditing ? "A conta é definida na criação." : undefined}
      >
        <Select
          id="investment-position-account"
          value={draft.accountId || undefined}
          options={accountOptions}
          disabled={isEditing}
          placeholder="Selecione uma conta manual"
          onChange={(value: string) => setDraft((current) => ({ ...current, accountId: value }))}
        />
      </FormField>
      {!isEditing && !showNewAsset && (
        <FormField
          label="Ativo"
          htmlFor="investment-position-asset"
          error={catalog.error ? (
            <>
              Não foi possível carregar os ativos.{" "}
              <Button type="link" size="small" htmlType="button" onClick={() => void catalog.refetch()}>
                Tentar novamente
              </Button>
            </>
          ) : undefined}
          hint={selectedAsset && (
            <span aria-live="polite">
              {[classLabel(selectedAsset), selectedAsset.asset_type, selectedAsset.currency_code,
                selectedAsset.quote_source ? "Cotação automática" : "Avaliação manual"].join(" · ")}
            </span>
          )}
        >
          <Select
            id="investment-position-asset"
            aria-describedby={selectedAsset ? "investment-position-asset-hint" : undefined}
            value={draft.assetId ?? undefined}
            options={assetOptions}
            showSearch
            autoFocus
            filterOption={(input, option) => searchKey(option?.label ?? "").includes(searchKey(input))}
            onSearch={setAssetSearch}
            notFoundContent="Nenhum ativo encontrado."
            placeholder={catalog.isLoading ? "Carregando ativos…" : "Busque por código ou nome (ex.: PETR4, Bitcoin)"}
            loading={catalog.isLoading}
            onChange={(value: string) => pickAsset(catalog.assets.find((asset) => asset.id === value))}
          />
          <Button type="link" htmlType="button" className="investment-new-asset-link" onClick={startNewAsset}>
            + Cadastrar novo ativo
          </Button>
        </FormField>
      )}
      {!isEditing && showNewAsset && (
        <section className="investment-new-asset" role="group" aria-labelledby="investment-new-asset-title">
          <div className="investment-new-asset-header">
            <h3 id="investment-new-asset-title">Novo ativo</h3>
            {!catalogEmpty && (
              <Button type="link" htmlType="button" onClick={() => setCreatingAsset(false)}>
                Escolher ativo existente
              </Button>
            )}
          </div>
          {catalogEmpty && <p className="form-field-hint">Nenhum ativo no catálogo ainda.</p>}
          <InvestmentAssetFields
            draft={assetDraft}
            onChange={setAssetDraft}
            classification={catalog.classification}
            quoteMarket={assetQuoteMarket}
          />
        </section>
      )}
      {isEditing && (
        <>
          <FormField label="Nome" htmlFor="investment-position-name">
            <Input
              id="investment-position-name"
              value={draft.name}
              autoFocus
              onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))}
              placeholder="Ex.: Tesouro Selic 2029"
            />
          </FormField>
          <div className="investment-form-pair">
            <FormField label="Código (opcional)" htmlFor="investment-position-ticker">
              <Input
                id="investment-position-ticker"
                value={draft.ticker}
                onChange={(event) => setDraft((current) => ({ ...current, ticker: event.target.value }))}
                placeholder="Ex.: BOVA11"
              />
            </FormField>
            <FormField label="Tipo do ativo" htmlFor="investment-position-type">
              <Input
                id="investment-position-type"
                value={draft.assetType}
                onChange={(event) => setDraft((current) => ({ ...current, assetType: event.target.value }))}
                placeholder="Ex.: ETF, CDB, Fundo"
              />
            </FormField>
          </div>
        </>
      )}
      <FormField label="Objetivo (opcional)" htmlFor="investment-position-goal">
        <Select
          id="investment-position-goal"
          value={draft.portfolioId ?? undefined}
          options={portfolioOptions}
          allowClear
          placeholder="Sem objetivo"
          showSearch
          optionFilterProp="label"
          onChange={(value: string | undefined) => setDraft((current) => ({ ...current, portfolioId: value ?? null }))}
        />
      </FormField>
      {!isEditing && (
        <>
          <div className="investment-form-pair">
            <FormField label="Quantidade inicial" htmlFor="investment-position-quantity">
              <InputNumber
                id="investment-position-quantity"
                style={{ width: "100%" }}
                min="0.00000001"
                stringMode
                inputMode="decimal"
                decimalSeparator=","
                value={draft.initialQuantity}
                onChange={(value) => setDecimal("initialQuantity", value)}
              />
            </FormField>
            <FormField label="Custo unitário" htmlFor="investment-position-unit-cost">
              <InputNumber
                id="investment-position-unit-cost"
                style={{ width: "100%" }}
                min="0.00000001"
                stringMode
                inputMode="decimal"
                prefix="R$"
                decimalSeparator=","
                value={draft.initialUnitCost}
                onChange={(value) => setDecimal("initialUnitCost", value)}
              />
            </FormField>
          </div>
          <FormField
            label="Valor inicial"
            htmlFor="investment-position-value"
            hint="Informe o valor, ou a quantidade e o custo unitário."
          >
            <MoneyInput
              id="investment-position-value"
              min={0.01}
              value={toMoneyInput(draft.initialValue)}
              onChange={(value) => setDraft((current) => ({ ...current, initialValue: fromMoneyInput(value) }))}
            />
          </FormField>
          <FormField label="Data do saldo inicial" htmlFor="investment-position-date">
            <DatePicker
              id="investment-position-date"
              style={{ width: "100%" }}
              value={draft.occurredOn}
              allowClear={false}
              format="DD/MM/YYYY"
              onChange={(value) => value && setDraft((current) => ({ ...current, occurredOn: value }))}
            />
          </FormField>
        </>
      )}
      <FormField label="Observações (opcional)" htmlFor="investment-position-notes">
        <Input.TextArea
          id="investment-position-notes"
          value={draft.notes}
          rows={3}
          onChange={(event) => setDraft((current) => ({ ...current, notes: event.target.value }))}
        />
      </FormField>
    </FormDrawer>
  );
}
