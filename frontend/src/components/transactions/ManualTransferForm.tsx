import { useTranslation } from "../../i18n/context";
import { useQueryClient } from "@tanstack/react-query";
import { Alert, DatePicker, Input, Select, Spin } from "../ui";
import { calendar } from "../../presentation/calendar";
import { useState } from "react";
import { createTransfer, type TransferMovement } from "../../api/transfers";
import { invalidateAfterTransactionChange } from "../../api/queryKeys";
import { useAccounts } from "../../hooks/useAccounts";
import { transferRate } from "../../presentation/transferRate";
import { FormDrawer } from "../forms/FormDrawer";
import { FormField } from "../forms/FormField";
import { MoneyInput } from "../forms/MoneyInput";
import { TransferDetail } from "./TransferDetail";

const positive = (value: string) => /^(0|[1-9]\d*)(\.\d+)?$/.test(value) && /[1-9]/.test(value);

export function ManualTransferForm({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const { t } = useTranslation();
  const accounts = useAccounts();
  const client = useQueryClient();
  const active = accounts.accounts.filter((account) => account.origin === "manual" && !account.archived_at);
  const [sourceId, setSourceId] = useState<string>();
  const [destinationId, setDestinationId] = useState<string>();
  const [sent, setSent] = useState("");
  const [received, setReceived] = useState("");
  const [fee, setFee] = useState("");
  const [date, setDate] = useState(calendar());
  const [description, setDescription] = useState(() => t("transferBetweenAccounts"));
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const source = active.find((account) => account.id === sourceId);
  const destination = active.find((account) => account.id === destinationId);
  const ready = !!source && !!destination && positive(sent) && positive(received) && description.trim() !== "" && (fee === "" || positive(fee));
  const movement = (account: NonNullable<typeof source>, amount: string): TransferMovement => ({ transaction_id: "", account_id: account.id, account_name: account.name ?? t("account"), currency_code: account.currency_code ?? "BRL", amount });
  const submit = async () => {
    if (!ready || saving) return;
    setSaving(true); setError(null);
    const write = (accountId: string, amount: string, text: string) => ({ account_id: accountId, amount, description: text.trim(), occurred_at: date.format("YYYY-MM-DD"), category_id: null });
    try {
      await createTransfer({ source: write(source!.id, `-${sent}`, description), destination: write(destination!.id, received, description), ...(fee ? { fee: write(source!.id, `-${fee}`, t("currencyExchangeFee")) } : {}) });
      await invalidateAfterTransactionChange(client);
      onCreated();
    } catch (caught) { setError(caught instanceof Error ? caught.message : t("transferCreationError")); }
    finally { setSaving(false); }
  };
  const options = (exclude?: string) => active.filter((account) => account.id !== exclude).map((account) => ({ value: account.id, label: `${account.name} · ${account.currency_code}` }));
  const amountField = (id: string, label: string, currency: string, value: string, onChange: (value: string) => void, hint?: string) => <FormField label={label} htmlFor={id} hint={hint}>
    <MoneyInput id={id} value={value || null} currencyCode={currency} min="0" onChange={(next) => onChange(next ?? "")} disabled={saving} aria-describedby={hint ? `${id}-hint` : undefined} />
  </FormField>;
  return <FormDrawer open title={t("newTransfer")} onClose={onClose} onSubmit={() => void submit()} submitLabel={t("createTransfer")} submitting={saving} submitDisabled={!ready || !!accounts.error} error={error}>
    {accounts.isLoading && <Spin aria-label={t("loadingAccounts")} />}
    {accounts.error && <Alert type="error" message={t("accountsConsultError")} />}
    {!accounts.isLoading && !accounts.error && active.length < 2 && <Alert type="info" message={t("twoManualAccountsRequired")} />}
    <FormField label={t("sourceAccount")} htmlFor="transfer-source-account"><Select id="transfer-source-account" value={sourceId} options={options(destinationId)} onChange={setSourceId} disabled={saving} showSearch optionFilterProp="label" /></FormField>
    {amountField("transfer-sent", t("amountSent"), source?.currency_code ?? "BRL", sent, setSent)}
    <FormField label={t("destinationAccount")} htmlFor="transfer-destination-account"><Select id="transfer-destination-account" value={destinationId} options={options(sourceId)} onChange={setDestinationId} disabled={saving} showSearch optionFilterProp="label" /></FormField>
    {amountField("transfer-received", t("amountReceived"), destination?.currency_code ?? "BRL", received, setReceived)}
    <FormField label={t("date")} htmlFor="transfer-date"><DatePicker id="transfer-date" value={date} onChange={(next) => next && setDate(next)} allowClear={false} disabled={saving} /></FormField>
    <FormField label={t("description")} htmlFor="transfer-description"><Input id="transfer-description" value={description} onChange={(event) => setDescription(event.target.value)} disabled={saving} /></FormField>
    {amountField("transfer-manual-fee", t("optionalRecordedFee"), source?.currency_code ?? "BRL", fee, setFee, t("recordedFeeHint"))}
    {source && destination && positive(sent) && positive(received) && <TransferDetail transfer={{ id: "", source: movement(source, `-${sent}`), destination: movement(destination, received), effective_rate: transferRate(sent, received), fee: positive(fee) ? movement(source, `-${fee}`) : null }} />}
  </FormDrawer>;
}
