import { useTranslation } from "../../i18n/context";
import { Input, Select } from "../ui";
import { useEffect, useState } from "react";

import type { Account, InvestmentAccountWrite } from "../../api/contracts";
import { FormDrawer } from "../forms/FormDrawer";
import { FormField } from "../forms/FormField";

type Draft = { name: string; financialAccountId: string | null };

const blankDraft: Draft = { name: "", financialAccountId: null };

export function InvestmentAccountForm({
  open,
  financialAccounts,
  submitting,
  submitError,
  onSubmit,
  onCancel,
}: {
  open: boolean;
  financialAccounts: Account[];
  submitting: boolean;
  submitError: string | null;
  onSubmit: (write: InvestmentAccountWrite) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<Draft>(blankDraft);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setDraft(blankDraft);
      setError(null);
    }
  }, [open]);

  const submit = () => {
    if (draft.name.trim() === "") {
      setError(t("investmentAccountNameRequired"));
      return;
    }
    setError(null);
    onSubmit({
      name: draft.name.trim(),
      currency_code: "BRL",
      financial_account_id: draft.financialAccountId,
    });
  };

  const accountOptions = financialAccounts.filter((account) => account.currency_code === "BRL" && account.account_type !== "CREDIT").map((account) => ({
    value: account.id,
    label: [account.name, account.institution_name].filter(Boolean).join(" · ") || account.id,
  }));

  return (
    <FormDrawer
      title={t("newInvestmentAccount")}
      open={open}
      onClose={onCancel}
      onSubmit={submit}
      submitLabel={t("createAccount")}
      submitting={submitting}
      error={error ?? submitError}
      width={440}
    >
      <p className="form-field-hint">
        {t("manualInvestmentAccountHint")}
      </p>
      <FormField label={t("name")} htmlFor="investment-account-name">
        <Input
          id="investment-account-name"
          value={draft.name}
          onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))}
          placeholder={t("investmentAccountExample")}
          autoFocus
        />
      </FormField>
      <FormField
        label={t("linkedBankAccountOptional")}
        htmlFor="investment-financial-account"
        hint={t("linkedBankAccountHint")}
      >
        <Select
          id="investment-financial-account"
          value={draft.financialAccountId ?? undefined}
          options={accountOptions}
          placeholder={t("noAssociatedAccount")}
          allowClear
          showSearch
          optionFilterProp="label"
          onChange={(value: string | undefined) =>
            setDraft((current) => ({ ...current, financialAccountId: value ?? null }))
          }
        />
      </FormField>
    </FormDrawer>
  );
}
