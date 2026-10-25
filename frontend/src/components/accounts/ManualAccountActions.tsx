import { Alert, Button, Popconfirm, Space } from "../ui";
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { writeManualAccount } from "../../api/accounts";
import type { Account } from "../../api/contracts";
import { invalidateAfterManualAccountChange } from "../../api/queryKeys";
import { ManualAccountForm } from "./ManualAccountForm";

export function ManualAccountActions({ account }: { account: Account }) {
  const client = useQueryClient();
  const [form, setForm] = useState<"edit" | "adjust" | null>(null);
  const archived = Boolean(account.archived_at);
  const mutation = useMutation({
    mutationFn: () => writeManualAccount(account.id, { name: account.name ?? "", currency_code: account.currency_code ?? "BRL", opening_balance: account.opening_balance ?? "0", archived: !archived }),
    onSuccess: () => invalidateAfterManualAccountChange(client),
  });
  if (account.origin !== "manual") return null;
  return (
    <>
      {archived && <Alert type="info" showIcon message="Conta arquivada. O saldo e o histórico continuam disponíveis; novas movimentações estão bloqueadas." />}
      {mutation.isError && <Alert type="error" showIcon message={mutation.error.message} />}
      <Space wrap style={{ marginBlock: 16 }}>
        <Button onClick={() => setForm("edit")}>Editar conta</Button>
        {!archived && <Button onClick={() => setForm("adjust")}>Ajustar saldo</Button>}
        {archived ? <Button loading={mutation.isPending} onClick={() => mutation.mutate()}>Reativar conta</Button> :
          <Popconfirm title="Arquivar conta?" description="O saldo e o histórico serão preservados. Esta conta deixará de aceitar novas movimentações." okText="Arquivar" cancelText="Cancelar" onConfirm={() => mutation.mutateAsync()}>
            <Button loading={mutation.isPending}>Arquivar conta</Button>
          </Popconfirm>}
      </Space>
      {form && <ManualAccountForm key={form} account={account} adjusting={form === "adjust"} onClose={() => setForm(null)} onSaved={() => setForm(null)} />}
    </>
  );
}
