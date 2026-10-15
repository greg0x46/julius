import { DatePicker, Input, Segmented, Select } from "antd";
import { useEffect, useState } from "react";

import type { Account, Category, ManualTransactionWrite } from "../../api/contracts";
import { categoryKindLabel, renderCategoryIcon } from "../../presentation/categoryLabels";
import { useAccounts } from "../../hooks/useAccounts";
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


function categoryOptions(categories: Category[], direction: Direction) {
  const kindOrder = ["expense", "income", "transfer"] as const;
  return categories
    .filter((category) => category.is_active && matchesDirection(category.kind, direction))
    .sort((a, b) => {
      if (a.kind !== b.kind) return kindOrder.indexOf(a.kind) - kindOrder.indexOf(b.kind);
      return a.name.localeCompare(b.name, "pt-BR");
    })
    .map((category) => ({
      value: category.id,
      label: `${categoryKindLabel[category.kind]}: ${category.name}`,
      icon: category.icon,
      color: category.color,
    }));
}

/**
 * The fields of a manual transaction — one the person authors by hand
 * instead of it arriving through a sync, on an account that already exists
 * (see .specs/lancamentos-manuais.md). Controlled and container-less: the
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
        label="Conta"
        htmlFor={manualFieldId("account")}
        error={messageFor("account")}
        hint={account?.origin === "manual" ? "Transações efetivadas alteram o saldo desta conta na moeda selecionada." : "Não altera o saldo da conta, que vem do banco: só entra no extrato e nos totais."}
      >
        <AccountSelect
          id={manualFieldId("account")}
          value={draft.accountId}
          onChange={(accountId) => set("accountId", accountId ?? "")}
          invalid={invalid("account")}
        />
      </FormField>

      <FormField label="Descrição" htmlFor={manualFieldId("description")} error={messageFor("description")}>
        <Input
          id={manualFieldId("description")}
          aria-invalid={invalid("description")}
          aria-describedby={invalid("description") ? `${manualFieldId("description")}-error` : undefined}
          status={invalid("description") ? "error" : undefined}
          value={draft.description}
          onChange={(event) => set("description", event.target.value)}
          placeholder="Ex.: Almoço em dinheiro"
        />
      </FormField>

      <FormField label="Tipo" labelId="manual-transaction-direction-label">
        <Segmented
          block
          aria-labelledby="manual-transaction-direction-label"
          value={draft.direction}
          options={[
            { value: "outflow", label: "Saída" },
            { value: "inflow", label: "Entrada" },
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

      <FormField label="Valor" htmlFor={manualFieldId("amount")} error={messageFor("amount")}>
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

      <FormField label="Data" htmlFor="manual-transaction-date">
        <DatePicker
          id="manual-transaction-date"
          format="DD/MM/YYYY"
          value={draft.date}
          onChange={(value) => value && set("date", value)}
          allowClear={false}
        />
      </FormField>

      <FormField label="Categoria (opcional)" htmlFor="manual-transaction-category">
        <Select
          id="manual-transaction-category"
          value={draft.categoryId ?? undefined}
          options={categoryOptions(categories, draft.direction)}
          allowClear={!isEditing}
          showSearch
          optionFilterProp="label"
          placeholder="Sem categoria"
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
  const defaultAccountId = accounts.find((account) => !account.archived_at)?.id ?? "";
  const [draft, setDraft] = useState<ManualDraft>(() => blankManualDraft(defaultAccountId));
  const [issue, setIssue] = useState<ManualIssue | null>(null);

  // Every opening starts from a blank draft, on the first account.
  const [wasOpen, setWasOpen] = useState(open);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) {
      setDraft(blankManualDraft(defaultAccountId));
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
    const problem = manualDraftIssue(draft);
    setIssue((previous) => nextIssue(previous, problem));
    if (problem === null) onSubmit(manualDraftToWrite(draft));
  };

  return (
    <FormDrawer
      title="Nova transação"
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
