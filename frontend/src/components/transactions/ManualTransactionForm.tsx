import { useTranslation } from "../../i18n/context";
import type { Language } from "../../i18n/messages";
import { DatePicker, Input, Segmented, Select } from "../ui";
import { useEffect, useState } from "react";

import type { Account, Category, ManualTransactionWrite } from "../../api/contracts";
import { categoryKindName, renderCategoryIcon } from "../../presentation/categoryLabels";
import { useAccounts } from "../../hooks/useAccounts";
import { useTimezone } from "../../hooks/useTimezone";
import { AccountSelect } from "../forms/AccountSelect";
import { FormDrawer } from "../forms/FormDrawer";
import { FormField } from "../forms/FormField";
import { MoneyInput } from "../forms/MoneyInput";
import {
  blankManualDraft,
  manualDraftIssue,
  manualDraftToWrite,
  manualFieldId,
  matchesDirection,
  nextIssue,
  type Direction,
  type ManualDraft,
  type ManualField,
  type ManualIssue,
} from "./manualTransactionDraft";

/**
 * Sends focus to the field that failed validation (scrolling it into view),
 * once per failed submit — a message alone can sit out of view above or
 * below the fold, and a keyboard or screen-reader user would not know where
 * to go.
 */
function useFocusIssue(issue: ManualIssue | null) {
  const attempt = issue?.attempt;
  const field = issue?.field;
  useEffect(() => {
    if (field === undefined) return;
    const control = document.getElementById(manualFieldId(field));
    if (!control) return;
    control.scrollIntoView?.({ block: "center", behavior: "smooth" });
    control.focus({ preventScroll: true });
  }, [attempt, field]);
}


function categoryOptions(categories: Category[], direction: Direction, language: Language) {
  const kindOrder = ["expense", "income", "transfer"] as const;
  return categories
    .filter((category) => category.is_active && matchesDirection(category.kind, direction))
    .sort((a, b) => {
      if (a.kind !== b.kind) return kindOrder.indexOf(a.kind) - kindOrder.indexOf(b.kind);
      return a.name.localeCompare(b.name, "pt-BR");
    })
    .map((category) => ({
      value: category.id,
      label: `${categoryKindName(category.kind, language)}: ${category.name}`,
      icon: category.icon,
      color: category.color,
    }));
}

/**
 * The fields of a manual transaction — one the person authors by hand
 * instead of it arriving through a sync, on an account that already exists
 * (see docs/development/contexts/transactions.md). Controlled and container-less: the
 * new-transaction drawer and the panel's edit screen both put them inside
 * their own `<form>`, so the two flows cannot drift apart.
 */
export function ManualTransactionFields({
  draft,
  onChange,
  categories,
  isEditing,
  issue = null,
}: {
  draft: ManualDraft;
  onChange: (update: (current: ManualDraft) => ManualDraft) => void;
  categories: Category[];
  isEditing: boolean;
  /** The validation problem of the last submit: marks its field and says why under it. */
  issue?: ManualIssue | null;
}) {
  const { t, language } = useTranslation();
  const { accounts } = useAccounts();
  const account = accounts.find((item) => item.id === draft.accountId);
  useFocusIssue(issue);
  const messageFor = (field: ManualField) => (issue?.field === field ? issue.message : undefined);
  const invalid = (field: ManualField) => (issue?.field === field ? true : undefined);
  const set = <Key extends keyof ManualDraft>(key: Key, value: ManualDraft[Key]) =>
    onChange((current) => ({ ...current, [key]: value }));

  return (
    <>
      <FormField
        label={t("account")}
        htmlFor={manualFieldId("account")}
        error={messageFor("account")}
        hint={account?.origin === "manual" ? t("manualAccountTransactionHint") : t("syncedAccountTransactionHint")}
      >
        <AccountSelect
          id={manualFieldId("account")}
          value={draft.accountId}
          onChange={(accountId) => set("accountId", accountId ?? "")}
          invalid={invalid("account")}
        />
      </FormField>

      <FormField label={t("description")} htmlFor={manualFieldId("description")} error={messageFor("description")}>
        <Input
          id={manualFieldId("description")}
          aria-invalid={invalid("description")}
          aria-describedby={invalid("description") ? `${manualFieldId("description")}-error` : undefined}
          status={invalid("description") ? "error" : undefined}
          value={draft.description}
          onChange={(event) => set("description", event.target.value)}
          placeholder={t("manualDescriptionExample")}
        />
      </FormField>

      <FormField label={t("type")} labelId="manual-transaction-direction-label">
        <Segmented
          block
          aria-labelledby="manual-transaction-direction-label"
          value={draft.direction}
          options={[
            { value: "outflow", label: t("expenseSingular") },
            { value: "inflow", label: t("incomeSingular") },
          ]}
          onChange={(value) => {
            const direction = value as Direction;
            onChange((current) => {
              const selected = categories.find((category) => category.id === current.categoryId);
              const categoryId = selected && !matchesDirection(selected.kind, direction) ? null : current.categoryId;
              return { ...current, direction, categoryId };
            });
          }}
        />
      </FormField>

      <FormField label={t("amount")} htmlFor={manualFieldId("amount")} error={messageFor("amount")}>
        <MoneyInput
          currencyCode={account?.currency_code ?? "BRL"}
          id={manualFieldId("amount")}
          aria-invalid={invalid("amount")}
          aria-describedby={invalid("amount") ? `${manualFieldId("amount")}-error` : undefined}
          status={invalid("amount") ? "error" : undefined}
          min="0"
          value={draft.amount}
          onChange={(value) => set("amount", value)}
        />
      </FormField>

      <FormField label={t("date")} htmlFor="manual-transaction-date">
        <DatePicker
          id="manual-transaction-date"
          value={draft.date}
          onChange={(value) => value && set("date", value)}
          allowClear={false}
        />
      </FormField>

      <FormField label={t("optionalCategory")} htmlFor="manual-transaction-category">
        <Select
          id="manual-transaction-category"
          value={draft.categoryId ?? undefined}
          options={categoryOptions(categories, draft.direction, language)}
          allowClear={!isEditing}
          showSearch
          optionFilterProp="label"
          placeholder={t("uncategorized")}
          optionRender={(option) => (
            <span>
              <span style={{ color: option.data.color }} aria-hidden="true">
                {renderCategoryIcon(option.data.icon)}
              </span>{" "}
              {option.label}
            </span>
          )}
          onChange={(value: string | undefined) => set("categoryId", value ?? null)}
        />
      </FormField>
    </>
  );
}

/** Creating a manual transaction from the list: the fields inside the standard form drawer. */
export function ManualTransactionForm({
  open,
  accounts,
  categories,
  submitting,
  submitError,
  onSubmit,
  onCancel,
}: {
  open: boolean;
  accounts: Account[];
  categories: Category[];
  submitting: boolean;
  submitError: string | null;
  onSubmit: (write: ManualTransactionWrite) => void;
  onCancel: () => void;
}) {
  const { t, language } = useTranslation();
  const defaultAccountId = accounts.find((account) => !account.archived_at)?.id ?? "";
  const timezone = useTimezone();
  const [draft, setDraft] = useState<ManualDraft>(() => blankManualDraft(defaultAccountId, timezone));
  const [issue, setIssue] = useState<ManualIssue | null>(null);

  // Every opening starts from a blank draft, on the first account.
  const [wasOpen, setWasOpen] = useState(open);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) {
      setDraft(blankManualDraft(defaultAccountId, timezone));
      setIssue(null);
    }
  }
  // Accounts that arrive after the drawer opened must not be typed over.
  useEffect(() => {
    if (open && defaultAccountId !== "") {
      setDraft((current) => (current.accountId === "" ? { ...current, accountId: defaultAccountId } : current));
    }
  }, [open, defaultAccountId]);

  const submit = () => {
    const problem = manualDraftIssue(draft, language);
    setIssue((previous) => nextIssue(previous, problem));
    if (problem === null) onSubmit(manualDraftToWrite(draft));
  };

  return (
    <FormDrawer
      title={t("newTransaction")}
      open={open}
      onClose={onCancel}
      onSubmit={submit}
      submitting={submitting}
      error={submitError}
    >
      <ManualTransactionFields
        draft={draft}
        onChange={setDraft}
        categories={categories}
        isEditing={false}
        issue={issue}
      />
    </FormDrawer>
  );
}
