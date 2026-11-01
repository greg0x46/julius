import { useTranslation } from "../../i18n/context";
import { translate, type Language } from "../../i18n/messages";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Alert, Button, Spin } from "../ui";
import { useEffect, useState } from "react";

import type { TransactionItem } from "../../api/contracts";
import { invalidateAfterTransactionChange, queryKeys } from "../../api/queryKeys";
import { createTransfer, deleteTransfer, getTransactionTransfer, type AccountTransfer, type TransferMovement } from "../../api/transfers";
import { useTimezone } from "../../hooks/useTimezone";
import { currentMonthFilters, useTransactions } from "../../hooks/useTransactions";
import { formatMoney } from "../../presentation/money";
import { transferRate } from "../../presentation/transferRate";
import { FormField } from "../forms/FormField";
import { PanelFooter, PanelSection } from "../shared/PanelStack";
import { TransactionPicker, type PickerTransaction } from "../shared/TransactionPicker";
import { useConfirm } from "../shared/useConfirm";
import { TransferDetail } from "./TransferDetail";

function movement(item: TransactionItem, language: Language): TransferMovement & { amount: string } {
  return {
    transaction_id: item.id,
    account_id: item.account.id,
    account_name: item.account.name ?? translate(language, "account"),
    amount: item.amount_in_account_currency ?? "0",
    currency_code: item.account.currency_code ?? "BRL",
  };
}
function pickable(item: TransactionItem, language: Language): PickerTransaction {
  return {
    id: item.id,
    description: item.description ?? translate(language, "noDescription"),
    account: item.account.name,
    date: item.occurred_at,
    direction: item.classification === "unclassified" ? null : item.classification,
    amount: movement(item, language).amount.replace(/^-/, ""),
    currency_code: movement(item, language).currency_code,
  };
}
function usable(row: TransactionItem) {
  return row.inclusion.state !== "ignored" && row.amount_in_account_currency !== null &&
    (row.provider_status === "POSTED" || row.provider_status === "PENDING") &&
    !/[1-9]/.test(row.investment_transfer_amount) && row.totals_eligibility.reason !== "linked_transfer";
}

export function TransactionTransferScreen({ item }: { item: TransactionItem }) {
  const { t, language } = useTranslation();
  const client = useQueryClient();
  const confirm = useConfirm();
  const timezone = useTimezone();
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
    ...currentMonthFilters(timezone), date_from: null, date_to: null,
    classification: isOutflow ? "inflow" : "outflow", description: search || null,
  }, "none", page);
  const fees = useTransactions({
    ...currentMonthFilters(timezone), date_from: null, date_to: null,
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
      setError(caught instanceof Error ? caught.message : t("transferSaveError"));
    } finally {
      setSaving(false);
    }
  };
  if (relation.isPending) return <PanelSection><Spin aria-label={t("loadingTransfer")} /></PanelSection>;
  if (relation.isError) return (
    <PanelSection>
      <Alert type="error" message={t("transferLookupError")} action={<Button onClick={() => void relation.refetch()}>{t("retry")}</Button>} />
    </PanelSection>
  );
  if (relation.data) return (
    <>
      <TransferDetail transfer={relation.data} />
      {error && <Alert type="error" showIcon message={error} />}
      <PanelFooter>
        <Button danger block loading={saving} onClick={() => confirm({
          title: t("unlinkTransfer"),
          okText: t("unlink"),
          description: t("unlinkTransferHint"),
          onConfirm: () => write(async () => { await deleteTransfer(relation.data!.id); return null; }),
        })}>{t("unlinkTransfer")}</Button>
      </PanelFooter>
    </>
  );
  const counterpartLabel = isOutflow ? t("destinationInflow") : t("sourceOutflow");
  return (
    <>
      <PanelSection>
        <p>{item.description} · {item.account.name} · {formatMoney(movement(item, language).amount, movement(item, language).currency_code)}</p>
        <FormField label={counterpartLabel} htmlFor="transfer-counterpart">
          <TransactionPicker
            id="transfer-counterpart" label={counterpartLabel}
            value={counterpart?.id ?? null} transactions={options.map((option) => pickable(option, language))}
            loading={candidates.isFetching} disabled={saving}
            emptyText={t("noCompatibleMovements")}
            search={{ value: search, onChange: (value) => { setSearch(value); setPage(1); } }}
            onSelect={(id) => {
              setCounterpart(options.find((row) => row.id === id) ?? null);
              setFee(null); setFeeSearch(""); setFeePage(1);
            }}
          />
        </FormField>
        {candidates.isError && <Alert type="error" message={t("movementsLookupError")} action={<Button onClick={() => void candidates.refetch()}>{t("retry")}</Button>} />}
        {(candidates.data?.page.total_pages ?? 0) > 1 && (
          <div>
            <Button disabled={page === 1 || candidates.isFetching} onClick={() => setPage(page - 1)}>{t("previousMovements")}</Button>
            <Button disabled={page >= (candidates.data?.page.total_pages ?? 1) || candidates.isFetching} onClick={() => setPage(page + 1)}>{t("nextMovements")}</Button>
          </div>
        )}
        <FormField label={t("optionalRecordedFee")} htmlFor="transfer-fee" hint={t("selectExistingFeeHint")}>
          <TransactionPicker
            id="transfer-fee" label={t("optionalRecordedFee")}
            value={fee?.id ?? null} transactions={feeOptions.map((option) => pickable(option, language))} allowClear
            disabled={!counterpart || saving} loading={fees.isFetching}
            search={{ value: feeSearch, onChange: (value) => { setFeeSearch(value); setFeePage(1); } }}
            onSelect={(id) => setFee(feeOptions.find((row) => row.id === id) ?? null)}
          />
        </FormField>
        {fees.isError && <Alert type="error" message={t("feesLookupError")} action={<Button onClick={() => void fees.refetch()}>{t("retry")}</Button>} />}
        {counterpart && (fees.data?.page.total_pages ?? 0) > 1 && (
          <div>
            <Button disabled={feePage === 1 || fees.isFetching} onClick={() => setFeePage(feePage - 1)}>{t("previousFees")}</Button>
            <Button disabled={feePage >= (fees.data?.page.total_pages ?? 1) || fees.isFetching} onClick={() => setFeePage(feePage + 1)}>{t("nextFees")}</Button>
          </div>
        )}
        {error && <Alert type="error" showIcon message={error} />}
      </PanelSection>
      {source && destination && <TransferDetail transfer={{
        id: "", source: movement(source, language), destination: movement(destination, language),
        effective_rate: transferRate(movement(source, language).amount, movement(destination, language).amount),
        fee: fee ? movement(fee, language) : null,
      }} />}
      <PanelFooter>
        <Button type="primary" block loading={saving} disabled={!source || !destination} onClick={() => void write(() => createTransfer({
          source_transaction_id: source!.id,
          destination_transaction_id: destination!.id,
          ...(fee ? { fee_transaction_id: fee.id } : {}),
        }))}>{t("linkTransfer")}</Button>
      </PanelFooter>
    </>
  );
}
