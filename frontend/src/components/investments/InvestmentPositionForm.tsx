import { DatePicker, Input, InputNumber, Select } from "antd";
import dayjs, { type Dayjs } from "dayjs";
import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";

import type {
  InvestmentAccount,
  InvestmentPortfolio,
  InvestmentPosition,
  InvestmentPositionUpdate,
  InvestmentPositionWrite,
} from "../../api/contracts";
import { listInvestmentAssets } from "../../api/investmentPortfolio";
import { investmentAssetsQueryKey } from "../../hooks/useInvestmentAssets";
import { FormDrawer } from "../forms/FormDrawer";
import { FormField } from "../forms/FormField";
import { MoneyInput } from "../forms/MoneyInput";
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
    accountId: accounts.find((account) => account.active)?.id ?? "",
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

export function InvestmentPositionForm({
  open,
  position,
  accounts,
  positions,
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
  positions: InvestmentPosition[];
  portfolios: InvestmentPortfolio[];
  submitting: boolean;
  submitError: string | null;
  onCreate: (write: InvestmentPositionWrite) => void;
  onUpdate: (write: InvestmentPositionUpdate) => void;
  onCancel: () => void;
}) {
  const [draft, setDraft] = useState<Draft>(() => draftFrom(position, accounts));
  const [error, setError] = useState<string | null>(null);
  const isEditing = position !== null;
  const catalog = useQuery({
    queryKey: investmentAssetsQueryKey,
    queryFn: ({ signal }) => listInvestmentAssets(signal),
    enabled: open && !isEditing,
  });

  useEffect(() => {
    if (open) {
      setDraft(draftFrom(position, accounts));
      setError(null);
    }
  }, [open, position, accounts]);

  const editableAccounts = useMemo(
    () => accounts.filter((account) => account.active),
    [accounts],
  );
  const accountOptions = editableAccounts.map((account) => ({ value: account.id, label: account.name }));
  const portfolioOptions = portfolios.map((portfolio) => ({ value: portfolio.id, label: portfolio.name }));
  const assetOptions = Array.from(new Map([
    ...positions.filter((item) => item.asset_id !== null).map((item) => [item.asset_id!, { value: item.asset_id!, label: [item.ticker, item.name].filter(Boolean).join(" · ") }] as const),
    ...(catalog.data ?? []).map((item) => [item.id, { value: item.id, label: [item.ticker, item.name].filter(Boolean).join(" · ") }] as const),
  ]).values());

  const submit = () => {
    if (draft.name.trim() === "") {
      setError("Informe o nome da posição.");
      return;
    }
    if (draft.assetType.trim() === "") {
      setError("Informe o tipo do ativo.");
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
        portfolio_id: draft.portfolioId,
        notes: draft.notes.trim() || null,
      });
      return;
    }
    onCreate({
      account_id: draft.accountId,
      asset_id: draft.assetId,
      name: draft.name.trim(),
      ticker: draft.ticker.trim() || null,
      asset_type: draft.assetType.trim(),
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
      onSubmit={submit}
      submitting={submitting}
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
      {!isEditing && assetOptions.length > 0 && (
        <FormField
          label="Ativo existente (opcional)"
          htmlFor="investment-position-asset"
          hint="O mesmo ativo pode ser mantido em várias contas."
        >
          <Select
            id="investment-position-asset"
            value={draft.assetId ?? undefined}
            options={assetOptions}
            allowClear
            showSearch
            optionFilterProp="label"
            placeholder="Cadastre um novo ativo abaixo"
            loading={catalog.isLoading}
            onChange={(value: string | undefined) => {
              const selected = catalog.data?.find((item) => item.id === value) ?? positions.find((item) => item.asset_id === value);
              setDraft((current) => ({ ...current, assetId: value ?? null,
                name: selected?.name ?? current.name, ticker: selected?.ticker ?? current.ticker,
                assetType: selected?.asset_type ?? current.assetType }));
            }}
          />
        </FormField>
      )}
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
