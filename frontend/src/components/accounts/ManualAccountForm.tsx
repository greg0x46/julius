import { Alert, Button, Input, Select } from "antd";
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
  const [amount, setAmount] = useState(adjusting ? "" : account?.opening_balance ?? "0");
  const [reason, setReason] = useState("");
  const [issue, setIssue] = useState<{ field: string; message: string } | null>(null);
  const currencies = useQuery({ queryKey: ["account-currencies"], queryFn: ({ signal }) => listAccountCurrencies(signal), enabled: !adjusting });
  const names = new Intl.DisplayNames(["pt-BR"], { type: "currency" });
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
      submitting={mutation.isPending} submitDisabled={!adjusting && !locked && !currencies.data}
      error={mutation.error?.message}>
      {!adjusting && <>
        <FormField label="Nome" htmlFor="manual-account-name" error={errorFor("name")}>
          <Input id="manual-account-name" autoFocus value={name} maxLength={200} onChange={(event) => setName(event.target.value)} aria-invalid={Boolean(errorFor("name"))} aria-describedby={errorFor("name") ? "manual-account-name-error" : undefined} />
        </FormField>
        <FormField label="Moeda" htmlFor="manual-account-currency" hint={locked ? "Esta conta já tem movimentações. A moeda e o saldo inicial não podem ser alterados." : undefined}>
          <Select id="manual-account-currency" value={currency} onChange={setCurrency} disabled={locked} loading={currencies.isLoading} showSearch optionFilterProp="label"
            aria-describedby={locked ? "manual-account-currency-hint" : undefined}
            options={(currencies.data ?? [currency]).map((code) => ({ value: code, label: `${code} — ${names.of(code) ?? code}` }))} />
        </FormField>
        {currencies.isError && <Alert type="error" message="Não foi possível carregar as moedas" action={<Button onClick={() => currencies.refetch()}>Tentar novamente</Button>} />}
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
