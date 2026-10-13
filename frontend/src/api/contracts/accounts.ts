import { isCount, isUuid, nullableDate, nullableDecimal, nullableText, requiredRecord } from "./shared";

const isClosingDay = (value: unknown): boolean =>
  value === null || (isCount(value) && value >= 1 && value <= 31);

const isClosingDaySource = (value: unknown): boolean =>
  value === null || accountClosingDaySources.includes(value as AccountClosingDaySource);

export type AccountType = "BANK" | "CREDIT";

const accountTypes = ["BANK", "CREDIT"] as const;

/** Where a card's closing day came from, in decreasing order of trust. */
export const accountClosingDaySources = ["manual", "informado", "estimado"] as const;

export type AccountClosingDaySource = (typeof accountClosingDaySources)[number];

export interface Account {
  id: string;
  origin: "manual" | "synced";
  opening_balance: string | null;
  archived_at: string | null;
  has_movements: boolean;
  external_id: string | null;
  source_display_name: string | null;
  institution: string | null;
  /** Display-safe institution: the user's connection label when set,
   *  otherwise the provider's institution with Pluggy's own proxy connector
   *  name filtered out. Null when neither is available — never render
   *  `institution` in that case, it may be integration plumbing. */
  institution_name: string | null;
  name: string | null;
  number: string | null;
  account_type: AccountType | null;
  account_subtype: string | null;
  balance: string | null;
  credit_limit: string | null;
  available_credit_limit: string | null;
  credit_usage_ratio: string | null;
  currency_code: string | null;
  balance_close_date: string | null;
  balance_due_date: string | null;
  /** Day of month the card's invoice closes, 1-31. */
  closing_day: number | null;
  closing_day_source: AccountClosingDaySource | null;
  provider_updated_at: string | null;
  updated_at: string | null;
}

const accountKeys = [
  "id",
  "origin",
  "opening_balance",
  "archived_at",
  "has_movements",
  "external_id",
  "source_display_name",
  "institution",
  "institution_name",
  "name",
  "number",
  "account_type",
  "account_subtype",
  "balance",
  "credit_limit",
  "available_credit_limit",
  "credit_usage_ratio",
  "currency_code",
  "balance_close_date",
  "balance_due_date",
  "closing_day",
  "closing_day_source",
  "provider_updated_at",
  "updated_at",
] as const;

export function parseAccount(value: unknown): Account {
  const item = requiredRecord(value, accountKeys, "Conta inválida.");
  if (
    typeof item.id !== "string" ||
    !isUuid(item.id) ||
    !["manual", "synced"].includes(item.origin as string) ||
    typeof item.has_movements !== "boolean" ||
    !(item.origin === "manual" ? item.external_id === null : typeof item.external_id === "string" && item.external_id !== "") ||
    !(item.account_type === null || accountTypes.includes(item.account_type as AccountType)) ||
    !isClosingDay(item.closing_day) ||
    !isClosingDaySource(item.closing_day_source) ||
    (item.closing_day === null) !== (item.closing_day_source === null)
  ) {
    throw new TypeError("Conta inválida.");
  }
  return {
    id: item.id,
    origin: item.origin as Account["origin"],
    opening_balance: nullableDecimal(item.opening_balance),
    archived_at: nullableDate(item.archived_at),
    has_movements: item.has_movements as boolean,
    external_id: nullableText(item.external_id),
    source_display_name: nullableText(item.source_display_name),
    institution: nullableText(item.institution),
    institution_name: nullableText(item.institution_name),
    name: nullableText(item.name),
    number: nullableText(item.number),
    account_type: item.account_type as AccountType | null,
    account_subtype: nullableText(item.account_subtype),
    balance: nullableDecimal(item.balance),
    credit_limit: nullableDecimal(item.credit_limit),
    available_credit_limit: nullableDecimal(item.available_credit_limit),
    credit_usage_ratio: nullableDecimal(item.credit_usage_ratio),
    currency_code: nullableText(item.currency_code),
    balance_close_date: nullableDate(item.balance_close_date),
    balance_due_date: nullableDate(item.balance_due_date),
    closing_day: item.closing_day as number | null,
    closing_day_source: item.closing_day_source as AccountClosingDaySource | null,
    provider_updated_at: nullableDate(item.provider_updated_at),
    updated_at: nullableDate(item.updated_at),
  };
}

export function parseAccountList(value: unknown): Account[] {
  if (!Array.isArray(value)) {
    throw new TypeError("Lista de contas inválida.");
  }
  return value.map(parseAccount);
}

/** One physical card spending against a credit account. */
export interface AccountCard {
  card_number: string;
  transaction_count: number;
  last_transaction_at: string | null;
}

const accountCardKeys = ["card_number", "transaction_count", "last_transaction_at"] as const;

export function parseAccountCard(value: unknown): AccountCard {
  const item = requiredRecord(value, accountCardKeys, "Cartão inválido.");
  if (
    typeof item.card_number !== "string" ||
    item.card_number === "" ||
    !isCount(item.transaction_count)
  ) {
    throw new TypeError("Cartão inválido.");
  }
  return {
    card_number: item.card_number,
    transaction_count: item.transaction_count,
    last_transaction_at: nullableDate(item.last_transaction_at),
  };
}

export function parseAccountCardList(value: unknown): AccountCard[] {
  if (!Array.isArray(value)) {
    throw new TypeError("Lista de cartões inválida.");
  }
  return value.map(parseAccountCard);
}

export interface AccountBill {
  id: string;
  external_id: string;
  due_date: string | null;
  closing_date: string | null;
  total_amount: string | null;
  currency_code: string | null;
  minimum_payment_amount: string | null;
}

const accountBillKeys = [
  "id",
  "external_id",
  "due_date",
  "closing_date",
  "total_amount",
  "currency_code",
  "minimum_payment_amount",
] as const;

export function parseAccountBill(value: unknown): AccountBill {
  const item = requiredRecord(value, accountBillKeys, "Fatura inválida.");
  if (
    typeof item.id !== "string" ||
    !isUuid(item.id) ||
    typeof item.external_id !== "string" ||
    item.external_id === ""
  ) {
    throw new TypeError("Fatura inválida.");
  }
  return {
    id: item.id,
    external_id: item.external_id,
    due_date: nullableDate(item.due_date),
    closing_date: nullableDate(item.closing_date),
    total_amount: nullableDecimal(item.total_amount),
    currency_code: nullableText(item.currency_code),
    minimum_payment_amount: nullableDecimal(item.minimum_payment_amount),
  };
}

export function parseAccountBillList(value: unknown): AccountBill[] {
  if (!Array.isArray(value)) {
    throw new TypeError("Lista de faturas inválida.");
  }
  return value.map(parseAccountBill);
}
