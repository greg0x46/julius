import { parseReportingSummary, type ReportingSummary } from "./reporting";
import { categoryKinds, categoryOrigins } from "./categories";
import type { CategoryKind, CategoryOrigin } from "./categories";
import { dateOnlyPattern, decimal, isCount, isNullableString, isUuid, isValidDate, nullableDecimal, nullableText, parseCurrencyTotals, positiveCount, requiredRecord, requiredRecordWithOptionalKeys } from "./shared";
import type { CurrencyTotals } from "./shared";

export const transactionClassifications = ["inflow", "outflow", "unclassified"] as const;

export type TransactionClassification = (typeof transactionClassifications)[number];

export const transactionGroupings = ["none", "day", "week", "month", "year"] as const;

export type TransactionGrouping = (typeof transactionGroupings)[number];

export const transactionInclusionStates = ["considered", "ignored"] as const;

export type TransactionInclusionState = (typeof transactionInclusionStates)[number];

export const transactionInclusionOrigins = ["manual", "rule"] as const;

export type TransactionInclusionOrigin = (typeof transactionInclusionOrigins)[number];

export const transactionOrigins = ["synced", "manual"] as const;

export type TransactionOrigin = (typeof transactionOrigins)[number];

export interface TransactionFilters {
  origin?: "manual" | "synced" | null;
  source_provider?: "file" | "pluggy" | null;
  card_balance?: boolean | null;
  credit_card?: boolean | null;
  date_from: string | null;
  date_to: string | null;
  description: string | null;
  /**
   * Each of these is an "any of" set: empty means no filter, several values
   * are an OR. Across fields the query is still an AND. The wire field names
   * are the API's plural forms; the singular ones remain accepted server-side
   * for older links.
   */
  account_ids: string[];
  category_ids: string[];
  classification: TransactionClassification | null;
  amount_min: string | null;
  amount_max: string | null;
  uncategorized: boolean | null;
}

export interface TransactionQuery {
  timezone: string;
  group_by: TransactionGrouping;
  page: number;
  page_size: number;
  filters: TransactionFilters;
}

export interface InternalCategory {
  id: string;
  name: string;
  kind: CategoryKind;
  is_active: boolean;
  icon: string;
  color: string;
  origin: CategoryOrigin;
  changed_at: string;
}

export interface TransactionItem {
  id: string;
  external_id: string;
  origin: TransactionOrigin;
  source_provider?: string | null;
  occurred_at: string | null;
  description: string | null;
  account: {
    id: string;
    name: string | null;
    institution: string | null;
    currency_code: string | null;
  };
  source_category: string | null;
  internal_category: InternalCategory | null;
  movement_type: string | null;
  provider_status: string | null;
  classification: TransactionClassification;
  amount: string | null;
  currency_code: string | null;
  amount_in_account_currency: string | null;
  effective_money: {
    value: string;
    currency_code: string;
    source: "account_currency" | "transaction_currency";
  } | null;
  /**
   * Total already allocated from this bank transaction to investment
   * deposits/withdrawals. It is zero when no allocation exists. The bank
   * amount itself remains in effective_money.
   */
  investment_transfer_amount: string;
  /** Amount that can be reported after currency normalization, if known. */
  reportable_amount: string | null;
  card: {
    number: string;
    installment_number: number | null;
    total_installments: number | null;
  } | null;
  inclusion: {
    state: TransactionInclusionState;
    changed_at: string | null;
    origin: TransactionInclusionOrigin;
    rule_name: string | null;
  };
  totals_eligibility: {
    included: boolean;
    reason:
      | "ignored"
      | "transfer_category"
      | "unclassified"
      | "ineligible_status"
      | "missing_money_pair"
      | "zero_value"
      | "investment_transfer"
      | null;
  };
  group_key: string;
}

export interface TransactionInclusionResult {
  transaction_id: string;
  state: TransactionInclusionState;
  changed_at: string | null;
}

export interface TransactionCategoryResult {
  transaction_id: string;
  category_id: string;
  origin: "manual";
  changed_at: string;
}

// ManualTransactionWrite is the request body for both creating and editing a
// lançamento manual. amount is already signed — positive for money coming
// in, negative for money going out — matching how the backend's
// transactions.ManualInput expects it.
export interface ManualTransactionWrite {
  account_id: string;
  description: string;
  amount: string;
  occurred_at: string;
  category_id: string | null;
}

export interface TransactionGroup {
 reporting?: ReportingSummary;
  key: string;
  kind: TransactionGrouping | "undated";
  start_date: string | null;
  end_date: string | null;
  item_count: number;
  page_item_count: number;
  has_items_before: boolean;
  has_items_after: boolean;
  totals: CurrencyTotals[];
}

export interface CategoryOption {
  id: string;
  name: string;
  kind: CategoryKind;
  is_active: boolean;
  icon: string;
  color: string;
}

export interface TransactionQueryResult {
 reporting?: ReportingSummary;
  confirmed_at: string;
  stored_total: number;
  page: { number: number; size: number; total_items: number; total_pages: number };
  items: TransactionItem[];
  totals: CurrencyTotals[];
  groups: TransactionGroup[];
  available_filters: {
    accounts: { id: string; name: string | null; institution: string | null }[];
    institutions: string[];
    categories: CategoryOption[];
  };
}

function parseInternalCategory(value: unknown): InternalCategory {
  const category = requiredRecord(
    value,
    ["id", "name", "kind", "is_active", "icon", "color", "origin", "changed_at"],
    "Categoria interna inválida.",
  );
  if (
    typeof category.id !== "string" ||
    !isUuid(category.id) ||
    typeof category.name !== "string" ||
    category.name === "" ||
    !categoryKinds.includes(category.kind as CategoryKind) ||
    typeof category.is_active !== "boolean" ||
    typeof category.icon !== "string" ||
    typeof category.color !== "string" ||
    !categoryOrigins.includes(category.origin as CategoryOrigin) ||
    !isValidDate(category.changed_at)
  ) {
    throw new TypeError("Categoria interna inválida.");
  }
  return {
    id: category.id,
    name: category.name,
    kind: category.kind as CategoryKind,
    is_active: category.is_active,
    icon: category.icon,
    color: category.color,
    origin: category.origin as CategoryOrigin,
    changed_at: category.changed_at as string,
  };
}

function parseCardInfo(value: unknown): NonNullable<TransactionItem["card"]> {
  const card = requiredRecord(
    value,
    ["number", "installment_number", "total_installments"],
    "Cartão inválido.",
  );
  if (
    typeof card.number !== "string" ||
    card.number === "" ||
    !(card.installment_number === null || isCount(card.installment_number)) ||
    !(card.total_installments === null || isCount(card.total_installments))
  ) {
    throw new TypeError("Cartão inválido.");
  }
  return {
    number: card.number,
    installment_number: card.installment_number as number | null,
    total_installments: card.total_installments as number | null,
  };
}

export function parseTransactionItem(value: unknown): TransactionItem {
  const item = requiredRecordWithOptionalKeys(value, [
    "id",
    "external_id",
    "origin",
    "occurred_at",
    "description",
    "account",
    "source_category",
    "internal_category",
    "movement_type",
    "provider_status",
    "classification",
    "amount",
    "currency_code",
    "amount_in_account_currency",
    "effective_money",
    "card",
    "inclusion",
    "totals_eligibility",
    "group_key",
  ], ["investment_transfer_amount", "reportable_amount", "source_provider"]);
  const account = requiredRecord(item.account, ["id", "name", "institution", "currency_code"]);
  const eligibility = requiredRecord(item.totals_eligibility, ["included", "reason"]);
  const inclusion = requiredRecord(item.inclusion, [
    "state",
    "changed_at",
    "origin",
    "rule_name",
  ]);
  const reasons = [
    "ignored",
    "transfer_category",
    "unclassified",
    "ineligible_status",
    "missing_money_pair",
    "zero_value",
    "investment_transfer",
  ];
  if (
    typeof item.id !== "string" ||
    !isUuid(item.id) ||
    typeof item.external_id !== "string" ||
    !transactionOrigins.includes(item.origin as TransactionOrigin) ||
    !(item.occurred_at === null || isValidDate(item.occurred_at)) ||
    !transactionClassifications.includes(item.classification as TransactionClassification) ||
    typeof item.group_key !== "string" ||
    typeof account.id !== "string" ||
    !isUuid(account.id) ||
    typeof eligibility.included !== "boolean" ||
    !(eligibility.reason === null || reasons.includes(String(eligibility.reason))) ||
    !transactionInclusionStates.includes(inclusion.state as TransactionInclusionState) ||
    !(inclusion.changed_at === null || isValidDate(inclusion.changed_at)) ||
    !transactionInclusionOrigins.includes(inclusion.origin as TransactionInclusionOrigin) ||
    !isNullableString(inclusion.rule_name) ||
    !isNullableString(item.source_category)
  ) {
    throw new TypeError("Item de transação inválido.");
  }
  if (
    eligibility.included !== (eligibility.reason === null) ||
    (inclusion.state === "ignored" &&
      (eligibility.included || eligibility.reason !== "ignored")) ||
    (inclusion.state === "considered" && eligibility.reason === "ignored")
  ) {
    throw new TypeError("Elegibilidade de transação inválida.");
  }
  let effectiveMoney: TransactionItem["effective_money"] = null;
  if (item.effective_money !== null) {
    const money = requiredRecord(item.effective_money, ["value", "currency_code", "source"]);
    if (
      typeof money.currency_code !== "string" ||
      money.currency_code === "" ||
      !["account_currency", "transaction_currency"].includes(String(money.source))
    ) {
      throw new TypeError("Par monetário inválido.");
    }
    effectiveMoney = {
      value: decimal(money.value),
      currency_code: money.currency_code,
      source: money.source as "account_currency" | "transaction_currency",
    };
  }
  return {
    id: item.id,
    external_id: item.external_id,
    origin: item.origin as TransactionOrigin,
    ...(item.source_provider === undefined ? {} : { source_provider: nullableText(item.source_provider) }),
    occurred_at: item.occurred_at as string | null,
    description: nullableText(item.description),
    account: {
      id: account.id,
      name: nullableText(account.name),
      institution: nullableText(account.institution),
      currency_code: nullableText(account.currency_code),
    },
    source_category: item.source_category as string | null,
    internal_category: item.internal_category === null ? null : parseInternalCategory(item.internal_category),
    movement_type: nullableText(item.movement_type),
    provider_status: nullableText(item.provider_status),
    classification: item.classification as TransactionClassification,
    amount: nullableDecimal(item.amount),
    currency_code: nullableText(item.currency_code),
    amount_in_account_currency: nullableDecimal(item.amount_in_account_currency),
    effective_money: effectiveMoney,
    // Older persisted fixtures and servers predate investment allocation.
    // Treat their absence as an empty allocation while real API responses
    // always carry both fields.
    investment_transfer_amount:
      item.investment_transfer_amount === undefined ? "0" : decimal(item.investment_transfer_amount),
    reportable_amount:
      item.reportable_amount === undefined ? effectiveMoney?.value ?? null : nullableDecimal(item.reportable_amount),
    card: item.card === null ? null : parseCardInfo(item.card),
    inclusion: {
      state: inclusion.state as TransactionInclusionState,
      changed_at: inclusion.changed_at as string | null,
      origin: inclusion.origin as TransactionInclusionOrigin,
      rule_name: inclusion.rule_name as string | null,
    },
    totals_eligibility: {
      included: eligibility.included,
      reason: eligibility.reason as TransactionItem["totals_eligibility"]["reason"],
    },
    group_key: item.group_key,
  };
}

export function parseTransactionCategoryResult(value: unknown): TransactionCategoryResult {
  const result = requiredRecord(
    value,
    ["transaction_id", "category_id", "origin", "changed_at"],
    "Confirmação de categoria inválida.",
  );
  if (
    typeof result.transaction_id !== "string" ||
    !isUuid(result.transaction_id) ||
    typeof result.category_id !== "string" ||
    !isUuid(result.category_id) ||
    result.origin !== "manual" ||
    !isValidDate(result.changed_at)
  ) {
    throw new TypeError("Confirmação de categoria inválida.");
  }
  return {
    transaction_id: result.transaction_id,
    category_id: result.category_id,
    origin: "manual",
    changed_at: result.changed_at as string,
  };
}

export function parseTransactionInclusionResult(value: unknown): TransactionInclusionResult {
  const result = requiredRecord(
    value,
    ["transaction_id", "state", "changed_at"],
    "Confirmação de inclusão inválida.",
  );
  if (
    typeof result.transaction_id !== "string" ||
    !isUuid(result.transaction_id) ||
    !transactionInclusionStates.includes(result.state as TransactionInclusionState) ||
    !(result.changed_at === null || isValidDate(result.changed_at))
  ) {
    throw new TypeError("Confirmação de inclusão inválida.");
  }
  return {
    transaction_id: result.transaction_id,
    state: result.state as TransactionInclusionState,
    changed_at: result.changed_at as string | null,
  };
}

function parseTransactionGroup(value: unknown): TransactionGroup {
  const group = requiredRecordWithOptionalKeys(value, [
    "key",
    "kind",
    "start_date",
    "end_date",
    "item_count",
    "page_item_count",
    "has_items_before",
    "has_items_after",
    "totals",
  ], ["reporting"]);
  const kinds = [...transactionGroupings, "undated"];
  if (
    typeof group.key !== "string" ||
    !kinds.includes(group.kind as TransactionGrouping | "undated") ||
    !(
      group.start_date === null ||
      (typeof group.start_date === "string" && dateOnlyPattern.test(group.start_date))
    ) ||
    !(
      group.end_date === null ||
      (typeof group.end_date === "string" && dateOnlyPattern.test(group.end_date))
    ) ||
    typeof group.has_items_before !== "boolean" ||
    typeof group.has_items_after !== "boolean" ||
    !Array.isArray(group.totals)
  ) {
    throw new TypeError("Grupo de transações inválido.");
  }
  return {
    key: group.key,
    kind: group.kind as TransactionGrouping | "undated",
    start_date: group.start_date as string | null,
    end_date: group.end_date as string | null,
    item_count: positiveCount(group.item_count),
    page_item_count: positiveCount(group.page_item_count),
    has_items_before: group.has_items_before,
    has_items_after: group.has_items_after,
    totals: group.totals.map(parseCurrencyTotals),
    ...(group.reporting === undefined ? {} : { reporting: parseReportingSummary(group.reporting) }),
  };
}

export function parseTransactionQueryResult(value: unknown): TransactionQueryResult {
  const result = requiredRecordWithOptionalKeys(value, [
    "confirmed_at",
    "stored_total",
    "page",
    "items",
    "totals",
    "groups",
    "available_filters",
  ], ["reporting"]);
  const page = requiredRecord(result.page, ["number", "size", "total_items", "total_pages"]);
  const filters = requiredRecord(result.available_filters, [
    "accounts",
    "institutions",
    "categories",
  ]);
  if (
    !isValidDate(result.confirmed_at) ||
    !isCount(result.stored_total) ||
    !Array.isArray(result.items) ||
    !Array.isArray(result.totals) ||
    !Array.isArray(result.groups) ||
    !Array.isArray(filters.accounts) ||
    !Array.isArray(filters.institutions) ||
    !Array.isArray(filters.categories)
  ) {
    throw new TypeError("Resposta de transações inválida.");
  }
  const accounts = filters.accounts.map((value) => {
    const account = requiredRecord(value, ["id", "name", "institution"]);
    if (typeof account.id !== "string" || !isUuid(account.id)) {
      throw new TypeError("Opção de conta inválida.");
    }
    return {
      id: account.id,
      name: nullableText(account.name),
      institution: nullableText(account.institution),
    };
  });
  if (!filters.institutions.every((item) => typeof item === "string")) {
    throw new TypeError("Facetas inválidas.");
  }
  const categories = filters.categories.map((value) => {
    const option = requiredRecord(value, ["id", "name", "kind", "is_active", "icon", "color"]);
    if (
      typeof option.id !== "string" ||
      !isUuid(option.id) ||
      typeof option.name !== "string" ||
      option.name === "" ||
      !categoryKinds.includes(option.kind as CategoryKind) ||
      typeof option.is_active !== "boolean" ||
      typeof option.icon !== "string" ||
      typeof option.color !== "string"
    ) {
      throw new TypeError("Opção de categoria inválida.");
    }
    return {
      id: option.id,
      name: option.name,
      kind: option.kind as CategoryKind,
      is_active: option.is_active,
      icon: option.icon,
      color: option.color,
    };
  });
  const number = positiveCount(page.number);
  const size = positiveCount(page.size);
  const totalItems = isCount(page.total_items) ? page.total_items : NaN;
  const totalPages = isCount(page.total_pages) ? page.total_pages : NaN;
  if (size > 100 || Number.isNaN(totalItems) || Number.isNaN(totalPages)) {
    throw new TypeError("Paginação inválida.");
  }
  return {
    confirmed_at: result.confirmed_at,
    stored_total: result.stored_total,
    page: { number, size, total_items: totalItems, total_pages: totalPages },
    items: result.items.map(parseTransactionItem),
    totals: result.totals.map(parseCurrencyTotals),
    ...(result.reporting === undefined ? {} : { reporting: parseReportingSummary(result.reporting) }),
    groups: result.groups.map(parseTransactionGroup),
    available_filters: {
      accounts,
      institutions: filters.institutions as string[],
      categories,
    },
  };
}
