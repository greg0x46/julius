import { useTranslation } from "../../i18n/context";
import { Alert, Button, Popconfirm, Space } from "../ui";
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import { writeManualAccount } from "../../api/accounts";
import type { Account } from "../../api/contracts";
import { invalidateAfterManualAccountChange } from "../../api/queryKeys";
import { ManualAccountForm } from "./ManualAccountForm";

export function ManualAccountActions({ account }: { account: Account }) {
  const { t } = useTranslation();
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
      {archived && <Alert type="info" showIcon message={t("archivedAccountNotice")} />}
      {mutation.isError && <Alert type="error" showIcon message={mutation.error.message} />}
      <Space wrap style={{ marginBlock: 16 }}>
        <Button onClick={() => setForm("edit")}>{t("editAccount")}</Button>
        {!archived && <Button onClick={() => setForm("adjust")}>{t("adjustBalance")}</Button>}
        {archived ? <Button loading={mutation.isPending} onClick={() => mutation.mutate()}>{t("reactivateAccount")}</Button> :
          <Popconfirm title={t("archiveAccountQuestion")} description={t("archiveAccountHint")} okText={t("archive")} cancelText={t("cancel")} onConfirm={() => mutation.mutateAsync()}>
            <Button loading={mutation.isPending}>{t("archiveAccount")}</Button>
          </Popconfirm>}
      </Space>
      {form && <ManualAccountForm key={form} account={account} adjusting={form === "adjust"} onClose={() => setForm(null)} onSaved={() => setForm(null)} />}
    </>
  );
}
