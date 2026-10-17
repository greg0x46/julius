import { Alert, Button, Checkbox, Input, Select } from "antd";
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
  const client = useQueryClient();
  const [name, setName] = useState(account?.name ?? "");
  const [currency, setCurrency] = useState(account?.currency_code ?? "BRL");
  const [otherCurrency, setOtherCurrency] = useState(false);
  const [amount, setAmount] = useState(adjusting ? "" : account?.opening_balance ?? "0");
  const [reason, setReason] = useState("");
  const [issue, setIssue] = useState<{ field: string; message: string } | null>(null);
  const currencies = useQuery({ queryKey: ["account-currencies"], queryFn: ({ signal }) => listAccountCurrencies(signal), enabled: !adjusting, staleTime: 24 * 60 * 60 * 1000 });
  const names = new Intl.DisplayNames(["pt-BR"], { type: "currency" });
  const selectedCoverage = currencies.data?.currencies.find((option) => option.code === currency);
  const coverageUnavailable = currencies.isError || currencies.data?.status === "unavailable";
  const coverageStale = currencies.data?.status === "stale" || (currencies.isError && Boolean(currencies.data));
  const codes = [...new Set(["BRL", ...(currencies.data?.currencies.map((option) => option.code) ?? []), ...(/^[A-Z]{3}$/.test(currency) ? [currency] : [])])];
  const formatDate = (date: string) => date.split("-").reverse().join("/");
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
    if (!adjusting && !/^[A-Z]{3}$/.test(currency)) return fail(otherCurrency ? "currency-code" : "currency", "Informe um código ISO 4217 de três letras.");
    if (!adjusting && !name.trim()) return fail("name", "Informe um nome para a conta.");
    if (!/^-?\d+(?:[.,]\d+)?$/.test(amount) || (adjusting && /^-?0+(?:[.,]0+)?$/.test(amount))) return fail("amount", "Informe um valor decimal válido" + (adjusting ? " diferente de zero." : "."));
    if (adjusting && !reason.trim()) return fail("reason", "Informe o motivo do ajuste.");
    mutation.mutate();
  };
  const title = adjusting ? "Ajustar saldo" : account ? "Editar conta" : "Nova conta bancária";
  const errorFor = (field: string) => issue?.field === field ? issue.message : undefined;
  return (
    <FormDrawer open title={title} onClose={onClose} onSubmit={submit}
      submitLabel={adjusting ? "Registrar ajuste" : account ? "Salvar" : "Criar conta"}
      submitting={mutation.isPending}
      error={mutation.error?.message}>
      {!adjusting && <>
        <FormField label="Nome" htmlFor="manual-account-name" error={errorFor("name")}>
          <Input id="manual-account-name" autoFocus value={name} maxLength={200} onChange={(event) => setName(event.target.value)} aria-invalid={Boolean(errorFor("name"))} aria-describedby={errorFor("name") ? "manual-account-name-error" : undefined} />
        </FormField>
        <FormField label="Moeda" htmlFor="manual-account-currency" hint={locked ? "Esta conta já tem movimentações. A moeda e o saldo inicial não podem ser alterados." : undefined}>
          <Select id="manual-account-currency" value={currency} onChange={setCurrency} disabled={locked || otherCurrency} loading={currencies.isLoading} showSearch optionFilterProp="label"
            aria-describedby={locked ? "manual-account-currency-hint manual-account-currency-support" : "manual-account-currency-support"}
            options={codes.map((code) => ({ value: code, label: `${code} — ${names.of(code) ?? code}` }))} />
        </FormField>
        {!locked && <Checkbox checked={otherCurrency} onChange={(event) => { setOtherCurrency(event.target.checked); setCurrency(event.target.checked ? "" : "BRL"); }}>Usar outra moeda</Checkbox>}
        {otherCurrency && <FormField label="Código da moeda" htmlFor="manual-account-currency-code" error={errorFor("currency-code")} hint="Informe o código ISO 4217 de três letras, por exemplo USD.">
          <Input id="manual-account-currency-code" value={currency} maxLength={3} onChange={(event) => setCurrency(event.target.value.toUpperCase())}
            aria-invalid={Boolean(errorFor("currency-code"))} aria-describedby={`manual-account-currency-code-${errorFor("currency-code") ? "error" : "hint"} manual-account-currency-support`} />
        </FormField>}
        <div id="manual-account-currency-support" role="status">
          {(coverageUnavailable || coverageStale) && <Alert type="warning"
            message={coverageStale ? "Não foi possível atualizar a lista de moedas. Usando a última lista disponível." : "Não foi possível verificar as moedas disponíveis. BRL continua disponível; a conversão de outras moedas não pode ser confirmada agora."}
            action={<Button onClick={() => currencies.refetch()} loading={currencies.isFetching}>Tentar novamente</Button>} />}
          {currency !== "BRL" && /^[A-Z]{3}$/.test(currency) && <>
            {selectedCoverage ? <p>Conversão para BRL pelo Frankfurter. A disponibilidade depende da data e do par de moedas.
              {selectedCoverage.start_date && selectedCoverage.end_date && <> Histórico informado: de {formatDate(selectedCoverage.start_date)} até {formatDate(selectedCoverage.end_date)}. Não há garantia de cotação para todas as datas.</>}</p>
              : <Alert type="warning" message={currencies.data?.status === "complete" && !currencies.isError ? `Sem conversão para BRL para ${currency}.` : `Conversão de ${currency} para BRL não confirmada.`}
                description={currencies.data?.status === "complete" && !currencies.isError ? "Você pode manter o saldo nesta moeda. Relatórios que precisarem dessa conversão ficarão indisponíveis; sincronizar não resolve essa limitação." : "Você pode manter o saldo nesta moeda. Relatórios em BRL dependerão da disponibilidade de cotações."} />}
          </>}
        </div>
      </>}
      <FormField label={adjusting ? "Valor do ajuste" : "Saldo inicial"} htmlFor="manual-account-amount" error={errorFor("amount")}
        hint={adjusting ? "Informe um valor positivo para aumentar o saldo ou negativo para diminuir. Este ajuste não é uma receita ou despesa." : "O saldo considera o valor inicial, as transações efetivadas e os ajustes de saldo."}>
        <Input id="manual-account-amount" prefix={currency === "BRL" ? "R$" : currency} inputMode="decimal" value={amount} onChange={(event) => setAmount(event.target.value)} disabled={!adjusting && locked}
          aria-invalid={Boolean(errorFor("amount"))} aria-describedby={`manual-account-amount-${errorFor("amount") ? "error" : "hint"}`} />
      </FormField>
      {adjusting && <FormField label="Motivo do ajuste" htmlFor="manual-account-reason" error={errorFor("reason")}>
        <Input id="manual-account-reason" maxLength={200} value={reason} onChange={(event) => setReason(event.target.value)} aria-invalid={Boolean(errorFor("reason"))} aria-describedby={errorFor("reason") ? "manual-account-reason-error" : undefined} />
      </FormField>}
    </FormDrawer>
  );
}
