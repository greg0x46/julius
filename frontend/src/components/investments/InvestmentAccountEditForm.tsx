import { useTranslation } from "../../i18n/context";
import { Button, Input, Select, Switch } from "../ui";
import { useEffect, useState } from "react";

import type { Account, InvestmentAccount, InvestmentAccountUpdate } from "../../api/contracts";
import { FormDrawer } from "../forms/FormDrawer";
import { FormField } from "../forms/FormField";

/**
 * InvestmentAccountForm only creates. Editing an investment account is a much
 * smaller decision — the name and whether it is still in use — so it gets its
 * own short form instead of a create form with half its fields disabled.
 */
export function InvestmentAccountEditForm({
  open,
  account,
  financialAccounts,
  submitting,
  submitError,
  onSubmit,
  onCancel,
}: {
  open: boolean;
  account: InvestmentAccount | null;
  financialAccounts: Account[];
  submitting: boolean;
  submitError: string | null;
  onSubmit: (write: InvestmentAccountUpdate) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [financialAccountId, setFinancialAccountId] = useState<string | null>(null);
  const [active, setActive] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open && account) {
      setName(account.name);
      setActive(account.active);
      setFinancialAccountId(account.financial_account_id);
      setError(null);
    }
  }, [open, account]);

  const submit = () => {
    if (name.trim() === "") {
      setError(t("investmentAccountNameRequired"));
      return;
    }
    setError(null);
    onSubmit({ name: name.trim(), active, financial_account_id: financialAccountId });
  };

  return (
    <FormDrawer
      title={t("editInvestmentAccount")}
      open={open}
      onClose={onCancel}
      onSubmit={submit}
      submitting={submitting}
      error={error ?? submitError}
      width={440}
    >
      <FormField label={t("name")} htmlFor="investment-account-edit-name">
        <Input
          id="investment-account-edit-name"
          disabled={account?.kind === "integrated"}
          value={name}
          autoFocus
          onChange={(event) => setName(event.target.value)}
        />
      </FormField>
      <FormField label={t("importedBrokerageCash")} htmlFor="investment-account-edit-financial">
        <Select
          id="investment-account-edit-financial"
          allowClear
          value={financialAccountId ?? undefined}
          options={financialAccounts
            .filter((item) => item.currency_code === "BRL" && item.account_type !== "CREDIT")
            .map((item) => ({ value: item.id, label: item.name ?? item.id }))}
          onChange={(value: string | undefined) => setFinancialAccountId(value ?? null)}
          placeholder={t("noLinkedAccount")}
        />
        {/* The Select's clear icon only shows on hover, which hides the fact
            that a link can be undone; the button makes it a visible action. */}
        {financialAccountId !== null && (
          <Button
            type="link"
            size="small"
            style={{ justifySelf: "start", paddingInline: 0 }}
            onClick={() => setFinancialAccountId(null)}
          >
            {t("unlinkAccount")}
          </Button>
        )}
      </FormField>
      <FormField label={t("accountInUse")} htmlFor="investment-account-edit-active">
        <Switch
          id="investment-account-edit-active"
          disabled={account?.kind === "integrated"}
          checked={active}
          onChange={setActive}
          checkedChildren={t("yes")}
          unCheckedChildren={t("no")}
          style={{ width: "fit-content" }}
        />
      </FormField>
    </FormDrawer>
  );
}
