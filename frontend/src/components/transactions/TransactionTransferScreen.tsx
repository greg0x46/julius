import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Alert, Button, Spin } from "antd";
import { useEffect, useState } from "react";

import type { TransactionItem } from "../../api/contracts";
import { invalidateAfterTransactionChange, queryKeys } from "../../api/queryKeys";
import { createTransfer, deleteTransfer, getTransactionTransfer, type AccountTransfer, type TransferMovement } from "../../api/transfers";
import { currentMonthFilters, useTransactions } from "../../hooks/useTransactions";
import { formatMoney } from "../../presentation/money";
import { transferRate } from "../../presentation/transferRate";
import { FormField } from "../forms/FormField";
import { PanelFooter, PanelSection } from "../shared/PanelStack";
import { TransactionPicker, type PickerTransaction } from "../shared/TransactionPicker";
import { useConfirm } from "../shared/useConfirm";
import { TransferDetail } from "./TransferDetail";

function movement(item: TransactionItem): TransferMovement & { amount: string } {
  return {
    transaction_id: item.id,
    account_id: item.account.id,
    account_name: item.account.name ?? "Conta",
    amount: item.amount_in_account_currency ?? "0",
    currency_code: item.account.currency_code ?? "BRL",
  };
}
function pickable(item: TransactionItem): PickerTransaction {
  return {
    id: item.id,
    description: item.description ?? "Sem descrição",
    account: item.account.name,
    date: item.occurred_at,
    direction: item.classification === "unclassified" ? null : item.classification,
    amount: movement(item).amount.replace(/^-/, ""),
    currency_code: movement(item).currency_code,
  };
}
function usable(row: TransactionItem) {
  return row.inclusion.state !== "ignored" && row.amount_in_account_currency !== null &&
    (row.provider_status === "POSTED" || row.provider_status === "PENDING") &&
    !/[1-9]/.test(row.investment_transfer_amount) && row.totals_eligibility.reason !== "linked_transfer";
}

export function TransactionTransferScreen({ item }: { item: TransactionItem }) {
  const client = useQueryClient();
  const confirm = useConfirm();
  const relation = useQuery({
    queryKey: queryKeys.transactionTransfer(item.id),
    queryFn: ({ signal }) => getTransactionTransfer(item.id, signal),
    retry: false,
  });
  const [counterpart, setCounterpart] = useState<TransactionItem | null>(null);
  const [fee, setFee] = useState<TransactionItem | null>(null);
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const [feeSearch, setFeeSearch] = useState("");
  const [feePage, setFeePage] = useState(1);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const isOutflow = item.classification === "outflow";
  const source = isOutflow ? item : counterpart;
  const destination = isOutflow ? counterpart : item;
  const candidates = useTransactions({
    ...currentMonthFilters(), date_from: null, date_to: null,
    classification: isOutflow ? "inflow" : "outflow", description: search || null,
  }, "none", page);
  const fees = useTransactions({
    ...currentMonthFilters(), date_from: null, date_to: null,
    classification: "outflow", description: feeSearch || null,
    account_ids: [source?.account.id, destination?.account.id].filter((id): id is string => !!id),
  }, "none", feePage);
  // Linking elsewhere can shrink a list under the page being viewed.
  const candidatePages = candidates.data?.page;
  const feePages = fees.data?.page;
  useEffect(() => {
    if (candidatePages && candidatePages.total_items > 0 && page > candidatePages.total_pages) setPage(candidatePages.total_pages);
  }, [candidatePages, page]);
  useEffect(() => {
    if (feePages && feePages.total_items > 0 && feePage > feePages.total_pages) setFeePage(feePages.total_pages);
  }, [feePages, feePage]);
  const options = (candidates.data?.items ?? []).filter((row) => row.id !== item.id && row.account.id !== item.account.id && usable(row));
  const feeOptions = (fees.data?.items ?? []).filter((row) => row.id !== item.id && row.id !== counterpart?.id && usable(row) && row.totals_eligibility.included);

  const write = async (action: () => Promise<AccountTransfer | null>) => {
    if (saving) return;
    setSaving(true);
    setError(null);
    try {
      const result = await action();
      client.setQueryData(queryKeys.transactionTransfer(item.id), result);
      await invalidateAfterTransactionChange(client);
      setCounterpart(null);
      setFee(null);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Não foi possível salvar a transferência.");
    } finally {
      setSaving(false);
    }
  };
  if (relation.isPending) return <PanelSection><Spin aria-label="Carregando transferência" /></PanelSection>;
  if (relation.isError) return (
    <PanelSection>
      <Alert type="error" message="Não foi possível consultar a transferência" action={<Button onClick={() => void relation.refetch()}>Tentar novamente</Button>} />
    </PanelSection>
  );
  if (relation.data) return (
    <>
      <TransferDetail transfer={relation.data} />
      {error && <Alert type="error" showIcon message={error} />}
      <PanelFooter>
        <Button danger block loading={saving} onClick={() => confirm({
          title: "Desvincular transferência",
          okText: "Desvincular",
          description: "Os lançamentos serão mantidos e voltarão a seguir suas decisões anteriores nos relatórios.",
          onConfirm: () => write(async () => { await deleteTransfer(relation.data!.id); return null; }),
        })}>Desvincular transferência</Button>
      </PanelFooter>
    </>
  );
  const counterpartLabel = isOutflow ? "Entrada na conta de destino" : "Saída da conta de origem";
  return (
    <>
      <PanelSection>
        <p>{item.description} · {item.account.name} · {formatMoney(movement(item).amount, movement(item).currency_code)}</p>
        <FormField label={counterpartLabel} htmlFor="transfer-counterpart">
          <TransactionPicker
            id="transfer-counterpart" label={counterpartLabel}
            value={counterpart?.id ?? null} transactions={options.map(pickable)}
            loading={candidates.isFetching} disabled={saving}
            emptyText="Nenhuma movimentação compatível disponível."
            search={{ value: search, onChange: (value) => { setSearch(value); setPage(1); } }}
            onSelect={(id) => {
              setCounterpart(options.find((row) => row.id === id) ?? null);
              setFee(null); setFeeSearch(""); setFeePage(1);
            }}
          />
        </FormField>
        {candidates.isError && <Alert type="error" message="Não foi possível consultar as movimentações" action={<Button onClick={() => void candidates.refetch()}>Tentar novamente</Button>} />}
        {(candidates.data?.page.total_pages ?? 0) > 1 && (
          <div>
            <Button disabled={page === 1 || candidates.isFetching} onClick={() => setPage(page - 1)}>Movimentações anteriores</Button>
            <Button disabled={page >= (candidates.data?.page.total_pages ?? 1) || candidates.isFetching} onClick={() => setPage(page + 1)}>Próximas movimentações</Button>
          </div>
        )}
        <FormField label="Tarifa registrada (opcional)" htmlFor="transfer-fee" hint="Selecione a cobrança registrada separadamente. A diferença de câmbio não cria uma tarifa.">
          <TransactionPicker
            id="transfer-fee" label="Tarifa registrada (opcional)"
            value={fee?.id ?? null} transactions={feeOptions.map(pickable)} allowClear
            disabled={!counterpart || saving} loading={fees.isFetching}
            search={{ value: feeSearch, onChange: (value) => { setFeeSearch(value); setFeePage(1); } }}
            onSelect={(id) => setFee(feeOptions.find((row) => row.id === id) ?? null)}
          />
        </FormField>
        {fees.isError && <Alert type="error" message="Não foi possível consultar as tarifas" action={<Button onClick={() => void fees.refetch()}>Tentar novamente</Button>} />}
        {counterpart && (fees.data?.page.total_pages ?? 0) > 1 && (
          <div>
            <Button disabled={feePage === 1 || fees.isFetching} onClick={() => setFeePage(feePage - 1)}>Tarifas anteriores</Button>
            <Button disabled={feePage >= (fees.data?.page.total_pages ?? 1) || fees.isFetching} onClick={() => setFeePage(feePage + 1)}>Próximas tarifas</Button>
          </div>
        )}
        {error && <Alert type="error" showIcon message={error} />}
      </PanelSection>
      {source && destination && <TransferDetail transfer={{
        id: "", source: movement(source), destination: movement(destination),
        effective_rate: transferRate(movement(source).amount, movement(destination).amount),
        fee: fee ? movement(fee) : null,
      }} />}
      <PanelFooter>
        <Button type="primary" block loading={saving} disabled={!source || !destination} onClick={() => void write(() => createTransfer({
          source_transaction_id: source!.id,
          destination_transaction_id: destination!.id,
          ...(fee ? { fee_transaction_id: fee.id } : {}),
        }))}>Vincular transferência</Button>
      </PanelFooter>
    </>
  );
}
