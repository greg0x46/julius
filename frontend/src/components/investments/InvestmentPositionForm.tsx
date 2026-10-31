import { isPositiveDecimal } from "../../presentation/decimal";
import { Button, DatePicker, Input, InputNumber, Select } from "../ui";
import type { CalendarDate } from "../../presentation/calendar";
import { todayIn } from "../../presentation/timezone";
import { useEffect, useMemo, useRef, useState } from "react";

import type {
  InvestmentAccount,
  InvestmentAsset,
  InvestmentPosition,
  InvestmentPositionUpdate,
  InvestmentPositionWrite,
} from "../../api/contracts";
import { useInvestmentAssets } from "../../hooks/useInvestmentAssets";
import { useTimezone } from "../../hooks/useTimezone";
import { errorMessage } from "../../presentation/errors";
import { FormDrawer } from "../forms/FormDrawer";
import { FormField } from "../forms/FormField";
import { MoneyInput } from "../forms/MoneyInput";
import { assetWriteFrom, catalogQuoteMarket, emptyAssetDraft, type AssetDraft } from "./assetDraft";
import { InvestmentAssetFields } from "./InvestmentAssetFields";
import { fromMoneyInput, toMoneyInput } from "./moneyDraft";

type Draft = {
  accountId: string;
  assetId: string | null;
  name: string;
  ticker: string;
  assetType: string;
  initialQuantity: string | null;
  initialUnitCost: string | null;
  initialValue: string | null;
  occurredOn: CalendarDate;
  notes: string;
};

function blankDraft(accounts: InvestmentAccount[], timezone: string): Draft {
  return {
    accountId: accounts.find((account) => account.active)?.id ?? "",
    assetId: null,
    name: "",
    ticker: "",
    assetType: "Ativo",
    initialQuantity: null,
    initialUnitCost: null,
    initialValue: null,
    occurredOn: todayIn(timezone),
    notes: "",
  };
}

function draftFrom(position: InvestmentPosition | null, accounts: InvestmentAccount[], timezone: string): Draft {
  if (!position) return blankDraft(accounts, timezone);
  return {
    accountId: position.account_id,
    assetId: position.asset_id,
    name: position.name,
    ticker: position.ticker ?? "",
    assetType: position.asset_type,
    initialQuantity: null,
    initialUnitCost: null,
    initialValue: null,
    occurredOn: todayIn(timezone),
    notes: position.notes ?? "",
  };
}

function positive(value: string | null): boolean {
  return value !== null && isPositiveDecimal(value);
}

/** Case-, accent- and punctuation-insensitive search. */
function searchKey(value: string): string {
  return value.normalize("NFD").replace(/[^\p{L}\p{N}]/gu, "").toUpperCase();
}

function assetLabel(asset: InvestmentAsset): string {
  return [asset.ticker, asset.name].filter(Boolean).join(" · ");
}

/** Mirrors the backend's canonical key: the ticker when there is one, else type and name. */
function assetCanonicalKey(asset: { name: string; ticker: string | null; asset_type: string }): string {
  const clean = (value: string) => Array.from(value.replace(/[^\p{L}\p{Nd}]/gu, ""), (letter) => {
    const upper = letter.toUpperCase();
    // Go's Unicode uppercase maps one rune at a time.
    return Array.from(upper).length === 1 ? upper : letter;
  }).join("");
  const ticker = clean(asset.ticker ?? "");
  return ticker ? `ticker:${ticker}` : `name:${clean(asset.asset_type)}:${clean(asset.name)}`;
}

function findDuplicate(assets: InvestmentAsset[], write: { name: string; ticker: string | null; asset_type: string }) {
  const key = assetCanonicalKey(write);
  return assets.find((asset) => assetCanonicalKey(asset) === key);
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
  submitting,
  submitError,
  onCreate,
  onUpdate,
  onCancel,
}: {
  open: boolean;
  position: InvestmentPosition | null;
  accounts: InvestmentAccount[];
  submitting: boolean;
  submitError: string | null;
  onCreate: (write: InvestmentPositionWrite) => void;
  onUpdate: (write: InvestmentPositionUpdate) => void;
  onCancel: () => void;
}) {
  const timezone = useTimezone();
  const [draft, setDraft] = useState<Draft>(() => draftFrom(position, accounts, timezone));
  const [error, setError] = useState<string | null>(null);
  const [creatingAsset, setCreatingAsset] = useState(false);
  const [assetDraft, setAssetDraft] = useState<AssetDraft>(emptyAssetDraft);
  const [assetSearch, setAssetSearch] = useState("");
  const [savingAsset, setSavingAsset] = useState(false);
  const [createdAsset, setCreatedAsset] = useState<InvestmentAsset | null>(null);
  const isEditing = position !== null;
  const catalog = useInvestmentAssets({ enabled: open && !isEditing });
  const session = useRef<{ open: boolean; positionId: string | null }>({ open: false, positionId: null });

  useEffect(() => {
    const positionId = position?.id ?? null;
    const reset = open && (!session.current.open || session.current.positionId !== positionId);
    session.current = { open, positionId };
    if (reset) {
      setDraft(draftFrom(position, accounts, timezone));
      setError(null);
      setCreatingAsset(false);
      setAssetDraft(emptyAssetDraft);
      setAssetSearch("");
      setCreatedAsset(null);
    } else if (open && !position) {
      setDraft((current) => current.accountId ? current : { ...current, accountId: blankDraft(accounts, timezone).accountId });
    }
  }, [open, position, accounts, timezone]);

  const editableAccounts = useMemo(
    () => accounts.filter((account) => account.active),
    [accounts],
  );
  const accountOptions = editableAccounts.map((account) => ({ value: account.id, label: account.name }));
  // The asset this form just created stays selectable until the catalog refetch brings it in.
  const assets = createdAsset && !catalog.assets.some((asset) => asset.id === createdAsset.id)
    ? [...catalog.assets, createdAsset]
    : catalog.assets;
  const assetOptions = assets.map((asset) => ({ value: asset.id, label: assetLabel(asset) }));
  const selectedAsset = assets.find((asset) => asset.id === draft.assetId);
  // With nothing in the catalog there is nothing to pick: the form opens on a new asset.
  const catalogEmpty = !catalog.isLoading && !catalog.error && assets.length === 0;
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
    const duplicate = findDuplicate(assets, write);
    if (duplicate) {
      pickAsset(duplicate);
      setError(`Já existe o ativo ${assetLabel(duplicate)} no catálogo. Ele foi selecionado; confira e salve.`);
      return null;
    }
    setSavingAsset(true);
    try {
      const asset = await catalog.createAsset(write);
      setCreatedAsset(asset);
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
      setError("Selecione uma conta de investimento.");
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
          avaliações manuais; sem avaliação, o valor exibido é o custo acumulado. Numa conta integrada, a posição
          fica ao lado das posições da instituição e a sincronização não a altera.
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
          placeholder="Selecione uma conta de investimento"
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
            // Leaving the field clears its search; the last text typed still seeds a new asset.
            onSearch={(value) => value !== "" && setAssetSearch(value)}
            notFoundContent="Nenhum ativo encontrado."
            placeholder={catalog.isLoading ? "Carregando ativos…" : "Busque por código ou nome"}
            loading={catalog.isLoading}
            onChange={(value: string) => pickAsset(assets.find((asset) => asset.id === value))}
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
              min="0"
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
