import { translate, type Language } from "../../i18n/messages";
import { isPositiveDecimal, sumDecimals } from "../../presentation/decimal";
import { calendar, type CalendarDate } from "../../presentation/calendar";

import type { Category, ManualTransactionWrite, TransactionItem } from "../../api/contracts";

const dateFormat = "YYYY-MM-DD";

export type Direction = "inflow" | "outflow";

/** What the person is typing: one value per field, the amount unsigned. */
export type ManualDraft = {
  accountId: string;
  description: string;
  direction: Direction;
  amount: string | null;
  date: CalendarDate;
  categoryId: string | null;
};

export function blankManualDraft(accountId: string): ManualDraft {
  return { accountId, description: "", direction: "outflow", amount: null, date: calendar(), categoryId: null };
}

/** `transaction === null` is "create"; otherwise the draft starts from that line's own fields. */
export function manualDraftFrom(transaction: TransactionItem | null, defaultAccountId: string): ManualDraft {
  if (!transaction) return blankManualDraft(defaultAccountId);
  return {
    accountId: transaction.account.id,
    description: transaction.description ?? "",
    direction: transaction.classification === "inflow" ? "inflow" : "outflow",
    amount: transaction.amount === null ? null : transaction.amount.replace(/^-/, ""),
    date: transaction.occurred_at ? calendar(transaction.occurred_at) : calendar(),
    categoryId: transaction.internal_category?.id ?? null,
  };
}

export type ManualField = "account" | "description" | "amount";

/** The first thing wrong with the draft: which field, and the message to show. Null when it can be saved. */
export function manualDraftIssue(draft: ManualDraft, language: Language = "pt-BR"): { field: ManualField; message: string } | null {
  if (draft.accountId === "") return { field: "account", message: translate(language, "selectAccountRequired") };
  if (draft.description.trim() === "") return { field: "description", message: translate(language, "descriptionRequired") };
  if (draft.amount === null || !isPositiveDecimal(draft.amount)) {
    return { field: "amount", message: translate(language, "positiveAmountRequired") };
  }
  return null;
}

/** What a failed submit leaves behind: the field to mark and focus, and what to say about it. */
export type ManualIssue = { field: ManualField; message: string; attempt: number };

/** A new issue object for each failed submit, so the same field failing twice refocuses. */
export function nextIssue(
  previous: ManualIssue | null,
  found: { field: ManualField; message: string } | null,
): ManualIssue | null {
  return found === null ? null : { ...found, attempt: (previous?.attempt ?? 0) + 1 };
}

/** The DOM id of a field's control, so a failed submit can send focus to it. */
export const manualFieldId = (field: ManualField) => `manual-transaction-${field}`;

/** The API body: the amount carries the sign the direction implies. Only call on a valid draft. */
export function manualDraftToWrite(draft: ManualDraft): ManualTransactionWrite {
  const amount = draft.amount ?? "0";
  const signed = draft.direction === "outflow" ? `-${amount}` : amount;
  return {
    account_id: draft.accountId,
    description: draft.description.trim(),
    amount: sumDecimals([signed]),
    occurred_at: draft.date.format(dateFormat),
    category_id: draft.categoryId,
  };
}

export function matchesDirection(kind: Category["kind"], direction: Direction): boolean {
  return direction === "inflow" ? kind === "income" || kind === "transfer" : kind === "expense" || kind === "transfer";
}
