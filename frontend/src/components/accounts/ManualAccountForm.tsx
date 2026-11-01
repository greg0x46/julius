import { formatDay } from "../../presentation/dates";
import { useTranslation } from "../../i18n/context";
import { Alert, Button, Checkbox, Input, Select } from "../ui";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { adjustManualAccount, listAccountCurrencies, writeManualAccount } from "../../api/accounts";
import type { Account } from "../../api/contracts";
import { invalidateAfterManualAccountChange } from "../../api/queryKeys";
import { FormDrawer } from "../forms/FormDrawer";
import { FormField } from "../forms/FormField";

export function ManualAccountForm({ account, adjusting = false, onClose, onSaved }: {
  account?: Account;
  adjusting?: boolean;
  onClose: () => void;
  onSaved: (account: Account) => void;
}) {
  const { t, language } = useTranslation();
  const client = useQueryClient();
  const [name, setName] = useState(account?.name ?? "");
  const [currency, setCurrency] = useState(account?.currency_code ?? "BRL");
  const [otherCurrency, setOtherCurrency] = useState(false);
  const [amount, setAmount] = useState(adjusting ? "" : account?.opening_balance ?? "0");
  const [reason, setReason] = useState("");
  const [issue, setIssue] = useState<{ field: string; message: string } | null>(null);
  const currencies = useQuery({ queryKey: ["account-currencies"], queryFn: ({ signal }) => listAccountCurrencies(signal), enabled: !adjusting, staleTime: 24 * 60 * 60 * 1000 });
  const names = new Intl.DisplayNames([language], { type: "currency" });
  const selectedCoverage = currencies.data?.currencies.find((option) => option.code === currency);
  const coverageUnavailable = currencies.isError || currencies.data?.status === "unavailable";
  const coverageStale = currencies.data?.status === "stale" || (currencies.isError && Boolean(currencies.data));
  const codes = [...new Set(["BRL", ...(currencies.data?.currencies.map((option) => option.code) ?? []), ...(/^[A-Z]{3}$/.test(currency) ? [currency] : [])])];
  const formatDate = (date: string) => formatDay(date, language);
  const locked = account?.has_movements ?? false;
  const mutation = useMutation({
    mutationFn: () => adjusting && account
      ? adjustManualAccount(account.id, amount.replace(",", "."), reason.trim())
      : writeManualAccount(account?.id ?? null, { name: name.trim(), currency_code: currency, opening_balance: amount.replace(",", "."), archived: Boolean(account?.archived_at) }),
    onSuccess: async (saved) => { await invalidateAfterManualAccountChange(client); onSaved(saved); },
  });
  const fail = (field: string, message: string) => {
    setIssue({ field, message });
    document.getElementById(`manual-account-${field}`)?.focus();
  };
  const submit = () => {
    setIssue(null);
    if (!adjusting && !/^[A-Z]{3}$/.test(currency)) return fail(otherCurrency ? "currency-code" : "currency", t("isoCurrencyError"));
    if (!adjusting && !name.trim()) return fail("name", t("accountNameError"));
    if (!/^-?\d+(?:[.,]\d+)?$/.test(amount) || (adjusting && /^-?0+(?:[.,]0+)?$/.test(amount))) return fail("amount", t(adjusting ? "nonzeroAmountError" : "decimalAmountError"));
    if (adjusting && !reason.trim()) return fail("reason", t("adjustmentReasonError"));
    mutation.mutate();
  };
  const title = adjusting ? t("adjustBalance") : account ? t("editAccount") : t("newBankAccount");
  const errorFor = (field: string) => issue?.field === field ? issue.message : undefined;
  return (
    <FormDrawer open title={title} onClose={onClose} onSubmit={submit}
      submitLabel={adjusting ? t("recordAdjustment") : account ? t("save") : t("createAccount")}
      submitting={mutation.isPending}
      error={mutation.error?.message}>
      {!adjusting && <>
        <FormField label={t("name")} htmlFor="manual-account-name" error={errorFor("name")}>
          <Input id="manual-account-name" autoFocus value={name} maxLength={200} onChange={(event) => setName(event.target.value)} aria-invalid={Boolean(errorFor("name"))} aria-describedby={errorFor("name") ? "manual-account-name-error" : undefined} />
        </FormField>
        <FormField label={t("currency")} htmlFor="manual-account-currency" hint={locked ? t("accountCurrencyLocked") : undefined}>
          <Select id="manual-account-currency" value={currency} onChange={setCurrency} disabled={locked || otherCurrency} loading={currencies.isLoading} showSearch optionFilterProp="label"
            aria-describedby={locked ? "manual-account-currency-hint manual-account-currency-support" : "manual-account-currency-support"}
            options={codes.map((code) => ({ value: code, label: `${code} — ${names.of(code) ?? code}` }))} />
        </FormField>
        {!locked && <Checkbox checked={otherCurrency} onChange={(event) => { setOtherCurrency(event.target.checked); setCurrency(event.target.checked ? "" : "BRL"); }}>{t("useOtherCurrency")}</Checkbox>}
        {otherCurrency && <FormField label={t("currencyCode")} htmlFor="manual-account-currency-code" error={errorFor("currency-code")} hint={t("currencyCodeHint")}>
          <Input id="manual-account-currency-code" value={currency} maxLength={3} onChange={(event) => setCurrency(event.target.value.toUpperCase())}
            aria-invalid={Boolean(errorFor("currency-code"))} aria-describedby={`manual-account-currency-code-${errorFor("currency-code") ? "error" : "hint"} manual-account-currency-support`} />
        </FormField>}
        <div id="manual-account-currency-support" role="status">
          {(coverageUnavailable || coverageStale) && <Alert type="warning"
            message={coverageStale ? t("currencyStale") : t("currencyUnavailable")}
            action={<Button onClick={() => currencies.refetch()} loading={currencies.isFetching}>{t("retry")}</Button>} />}
          {currency !== "BRL" && /^[A-Z]{3}$/.test(currency) && <>
            {selectedCoverage ? <p>{t("currencyConversionHint")}
              {selectedCoverage.start_date && selectedCoverage.end_date && <> {t("currencyHistoryHint").replace("{start}", formatDate(selectedCoverage.start_date)).replace("{end}", formatDate(selectedCoverage.end_date))}</>}</p>
              : <Alert type="warning" message={currencies.data?.status === "complete" && !currencies.isError ? t("currencyNoConversion").replace("{currency}", currency) : t("currencyConversionUnconfirmed").replace("{currency}", currency)}
                description={currencies.data?.status === "complete" && !currencies.isError ? t("currencyNoConversionHint") : t("currencyUnconfirmedHint")} />}
          </>}
        </div>
      </>}
      <FormField label={adjusting ? t("adjustmentAmount") : t("openingBalance")} htmlFor="manual-account-amount" error={errorFor("amount")}
        hint={adjusting ? t("adjustmentAmountHint") : t("openingBalanceHint")}>
        <Input id="manual-account-amount" prefix={currency === "BRL" ? "R$" : currency} inputMode="decimal" value={amount} onChange={(event) => setAmount(event.target.value)} disabled={!adjusting && locked}
          aria-invalid={Boolean(errorFor("amount"))} aria-describedby={`manual-account-amount-${errorFor("amount") ? "error" : "hint"}`} />
      </FormField>
      {adjusting && <FormField label={t("adjustmentReason")} htmlFor="manual-account-reason" error={errorFor("reason")}>
        <Input id="manual-account-reason" maxLength={200} value={reason} onChange={(event) => setReason(event.target.value)} aria-invalid={Boolean(errorFor("reason"))} aria-describedby={errorFor("reason") ? "manual-account-reason-error" : undefined} />
      </FormField>}
    </FormDrawer>
  );
}
