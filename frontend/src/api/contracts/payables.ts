import { decimal, isCount, isNullableString, isUuid, isValidDate, nullableText, requiredRecord } from "./shared";

export const payableKinds = ["debt", "receivable"] as const;

export type PayableKind = (typeof payableKinds)[number];

export const payableStatuses = ["open", "settled"] as const;

export type PayableStatus = (typeof payableStatuses)[number];

export interface Payable {
  id: string;
  kind: PayableKind;
  name: string;
  total_amount: string;
  starting_settled_amount: string;
  settled_amount: string;
  remaining_amount: string;
  status: PayableStatus;
  link_count: number;
  created_at: string;
  updated_at: string;
}

export interface PayableLinkedTransaction {
  id: string;
  transaction_id: string;
  occurred_at: string | null;
  description: string | null;
  linked_amount: string;
  current_amount: string;
  linked_at: string;
}

export interface PayableDetail extends Payable {
  links: PayableLinkedTransaction[];
}

export interface EligibleTransaction {
  id: string;
  occurred_at: string | null;
  description: string | null;
  account_name: string | null;
  effective_money: { value: string; currency_code: string };
}

export interface PayableCreate {
  kind: PayableKind;
  name: string;
  total_amount: string;
  initial_remaining_amount?: string | null;
}

export interface PayableUpdate {
  name: string;
  total_amount: string;
}

export interface PayableLinkCreate {
  transaction_id: string;
}

export interface PayableLink {
  id: string;
  transaction_id: string;
  linked_amount: string;
  linked_at: string;
}

const payableKeys = [
  "id",
  "kind",
  "name",
  "total_amount",
  "starting_settled_amount",
  "settled_amount",
  "remaining_amount",
  "status",
  "link_count",
  "created_at",
  "updated_at",
] as const;

function payableFieldsFrom(payable: Record<string, unknown>): Payable {
  if (
    typeof payable.id !== "string" ||
    !isUuid(payable.id) ||
    !payableKinds.includes(payable.kind as PayableKind) ||
    typeof payable.name !== "string" ||
    payable.name === "" ||
    !payableStatuses.includes(payable.status as PayableStatus) ||
    !isCount(payable.link_count) ||
    !isValidDate(payable.created_at) ||
    !isValidDate(payable.updated_at)
  ) {
    throw new TypeError("Pendência inválida.");
  }
  return {
    id: payable.id,
    kind: payable.kind as PayableKind,
    name: payable.name,
    total_amount: decimal(payable.total_amount),
    starting_settled_amount: decimal(payable.starting_settled_amount),
    settled_amount: decimal(payable.settled_amount),
    remaining_amount: decimal(payable.remaining_amount),
    status: payable.status as PayableStatus,
    link_count: payable.link_count,
    created_at: payable.created_at,
    updated_at: payable.updated_at,
  };
}

export function parsePayable(value: unknown): Payable {
  return payableFieldsFrom(requiredRecord(value, payableKeys, "Pendência inválida."));
}

export function parsePayableList(value: unknown): Payable[] {
  if (!Array.isArray(value)) {
    throw new TypeError("Lista de pendências inválida.");
  }
  return value.map(parsePayable);
}

export interface PayableTotalOwed {
  remaining_debts_total: string;
  future_installments_total: string;
  total_owed: string;
  currency_code: string;
}

export function parsePayableTotalOwed(value: unknown): PayableTotalOwed {
  const item = requiredRecord(
    value,
    ["remaining_debts_total", "future_installments_total", "total_owed", "currency_code"],
    "Total de dívida inválido.",
  );
  if (typeof item.currency_code !== "string" || item.currency_code === "") {
    throw new TypeError("Total de dívida inválido.");
  }
  return {
    remaining_debts_total: decimal(item.remaining_debts_total),
    future_installments_total: decimal(item.future_installments_total),
    total_owed: decimal(item.total_owed),
    currency_code: item.currency_code,
  };
}

export interface PayableTotalToReceive {
  remaining_receivables_total: string;
  total_to_receive: string;
  currency_code: string;
}

export function parsePayableTotalToReceive(value: unknown): PayableTotalToReceive {
  const item = requiredRecord(
    value,
    ["remaining_receivables_total", "total_to_receive", "currency_code"],
    "Total a receber inválido.",
  );
  if (typeof item.currency_code !== "string" || item.currency_code === "") {
    throw new TypeError("Total a receber inválido.");
  }
  return {
    remaining_receivables_total: decimal(item.remaining_receivables_total),
    total_to_receive: decimal(item.total_to_receive),
    currency_code: item.currency_code,
  };
}

function parsePayableLinkedTransaction(value: unknown): PayableLinkedTransaction {
  const link = requiredRecord(
    value,
    [
      "id",
      "transaction_id",
      "occurred_at",
      "description",
      "linked_amount",
      "current_amount",
      "linked_at",
    ],
    "Vínculo inválido.",
  );
  if (
    typeof link.id !== "string" ||
    !isUuid(link.id) ||
    typeof link.transaction_id !== "string" ||
    !isUuid(link.transaction_id) ||
    !(link.occurred_at === null || isValidDate(link.occurred_at)) ||
    !isNullableString(link.description) ||
    !isValidDate(link.linked_at)
  ) {
    throw new TypeError("Vínculo inválido.");
  }
  return {
    id: link.id,
    transaction_id: link.transaction_id,
    occurred_at: link.occurred_at as string | null,
    description: nullableText(link.description),
    linked_amount: decimal(link.linked_amount),
    current_amount: decimal(link.current_amount),
    linked_at: link.linked_at,
  };
}

export function parsePayableDetail(value: unknown): PayableDetail {
  const detail = requiredRecord(value, [...payableKeys, "links"], "Detalhe de pendência inválido.");
  if (!Array.isArray(detail.links)) {
    throw new TypeError("Detalhe de pendência inválido.");
  }
  return { ...payableFieldsFrom(detail), links: detail.links.map(parsePayableLinkedTransaction) };
}

export function parsePayableLink(value: unknown): PayableLink {
  const link = requiredRecord(
    value,
    ["id", "transaction_id", "linked_amount", "linked_at"],
    "Vínculo inválido.",
  );
  if (
    typeof link.id !== "string" ||
    !isUuid(link.id) ||
    typeof link.transaction_id !== "string" ||
    !isUuid(link.transaction_id) ||
    !isValidDate(link.linked_at)
  ) {
    throw new TypeError("Vínculo inválido.");
  }
  return {
    id: link.id,
    transaction_id: link.transaction_id,
    linked_amount: decimal(link.linked_amount),
    linked_at: link.linked_at,
  };
}

export function parseEligibleTransaction(value: unknown): EligibleTransaction {
  const item = requiredRecord(
    value,
    ["id", "occurred_at", "description", "account_name", "effective_money"],
    "Transação elegível inválida.",
  );
  const money = requiredRecord(
    item.effective_money,
    ["value", "currency_code"],
    "Valor efetivo inválido.",
  );
  if (
    typeof item.id !== "string" ||
    !isUuid(item.id) ||
    !(item.occurred_at === null || isValidDate(item.occurred_at)) ||
    !isNullableString(item.description) ||
    !isNullableString(item.account_name) ||
    typeof money.currency_code !== "string" ||
    money.currency_code === ""
  ) {
    throw new TypeError("Transação elegível inválida.");
  }
  return {
    id: item.id,
    occurred_at: item.occurred_at as string | null,
    description: nullableText(item.description),
    account_name: nullableText(item.account_name),
    effective_money: { value: decimal(money.value), currency_code: money.currency_code },
  };
}

export function parseEligibleTransactionList(value: unknown): EligibleTransaction[] {
  if (!Array.isArray(value)) {
    throw new TypeError("Lista de transações elegíveis inválida.");
  }
  return value.map(parseEligibleTransaction);
}
