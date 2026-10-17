import { parseReportingSummary, type ReportingSummary } from "./reporting";
import { decimal, isRecord, requiredRecord, requiredRecordWithOptionalKeys, parseCurrencyTotals, type CurrencyTotals } from "./shared";

export const categorySpendingSources = ["real", "projetado"] as const;

export type CategorySpendingSource = (typeof categorySpendingSources)[number];

export interface CategorySpendingItem {
  category_id: string | null;
  category_name: string;
  category_icon: string;
  category_color: string;
  amount: string;
  source: CategorySpendingSource;
}

export interface SpendingByCategory {
  month: string;
  currency_code: string;
  total: string;
  items: CategorySpendingItem[];
  reporting?: ReportingSummary;
  totals?: CurrencyTotals[];
}

const monthOnlyPattern = /^\d{4}-\d{2}$/;

function parseCategorySpendingItem(value: unknown): CategorySpendingItem {
  const item = requiredRecord(
    value,
    ["category_id", "category_name", "category_icon", "category_color", "amount", "source"],
    "Gasto por categoria inválido.",
  );
  if (item.category_id !== null && typeof item.category_id !== "string") {
    throw new TypeError("Gasto por categoria inválido.");
  }
  if (typeof item.category_name !== "string" || item.category_name === "") {
    throw new TypeError("Gasto por categoria inválido.");
  }
  if (typeof item.category_icon !== "string" || typeof item.category_color !== "string") {
    throw new TypeError("Gasto por categoria inválido.");
  }
  if (!categorySpendingSources.includes(item.source as CategorySpendingSource)) {
    throw new TypeError("Gasto por categoria inválido.");
  }
  return {
    category_id: item.category_id,
    category_name: item.category_name,
    category_icon: item.category_icon,
    category_color: item.category_color,
    amount: decimal(item.amount),
    source: item.source as CategorySpendingSource,
  };
}

export function parseSpendingByCategory(value: unknown): SpendingByCategory {
  const item = requiredRecordWithOptionalKeys(
    value,
    ["month", "currency_code", "total", "items"],
    ["reporting", "totals"],
    "Gastos por categoria inválidos.",
  );
  if (typeof item.month !== "string" || !monthOnlyPattern.test(item.month)) {
    throw new TypeError("Gastos por categoria inválidos.");
  }
  if (typeof item.currency_code !== "string" || item.currency_code === "") {
    throw new TypeError("Gastos por categoria inválidos.");
  }
  if (!Array.isArray(item.items)) {
    throw new TypeError("Gastos por categoria inválidos.");
  }
  const reporting = item.reporting === undefined ? undefined : parseReportingSummary(item.reporting);
  if (reporting?.totals === null) throw new TypeError("Gastos por categoria inválidos.");
  if (item.totals !== undefined && !Array.isArray(item.totals)) throw new TypeError("Totais por moeda inválidos.");
  return {
    ...(reporting ? { reporting } : {}),
    ...(Array.isArray(item.totals) ? { totals: item.totals.map(parseCurrencyTotals) } : {}),
    month: item.month,
    currency_code: item.currency_code,
    total: decimal(item.total),
    items: item.items.map(parseCategorySpendingItem),
  };
}

export type CategoryDirection = "inflow" | "outflow";

export type CategoryBreakdown = Omit<SpendingByCategory, "month" | "total"> & {
  total: string | null;
  reporting?: ReportingSummary;
  totals?: CurrencyTotals[];
  classification: CategoryDirection;
} & ({ month: string } | { date_from: string | null; date_to: string | null });

export function parseCategoryBreakdown(value: unknown): CategoryBreakdown {
  const periodKeys = isRecord(value) && "month" in value ? ["month"] : ["date_from", "date_to"];
  const item = requiredRecordWithOptionalKeys(value, [...periodKeys, "currency_code", "total", "items", "classification"], ["reporting", "totals"], "Distribuição por categoria inválida.");
  if (item.classification !== "inflow" && item.classification !== "outflow") {
    throw new TypeError("Distribuição por categoria inválida.");
  }
  const reporting = item.reporting === undefined ? undefined : parseReportingSummary(item.reporting);
  if (item.totals !== undefined && !Array.isArray(item.totals)) throw new TypeError("Totais por moeda inválidos.");
  const extras = { ...(reporting ? { reporting } : {}), ...(Array.isArray(item.totals) ? { totals: item.totals.map(parseCurrencyTotals) } : {}) };
  const total = item.total === null && reporting?.totals === null ? null : decimal(item.total);
  if (reporting && (reporting.totals === null) !== (total === null)) throw new TypeError("Distribuição por categoria inválida.");
  if ("month" in item) {
    if (typeof item.month !== "string" || !monthOnlyPattern.test(item.month) || typeof item.currency_code !== "string" || !Array.isArray(item.items)) throw new TypeError("Distribuição por categoria inválida.");
    return { month: item.month, currency_code: item.currency_code, total, items: item.items.map(parseCategorySpendingItem), classification: item.classification, ...extras };
  }
  const validDate = (date: unknown): date is string => {
    if (typeof date !== "string" || !/^\d{4}-\d{2}-\d{2}$/.test(date) || date < "0001-01-01") return false;
    const parsed = new Date(`${date}T00:00:00Z`);
    return Number.isFinite(parsed.getTime()) && parsed.toISOString().slice(0, 10) === date;
  };
  const { date_from: from, date_to: to } = item;
  if (!(from === null && to === null) && !(validDate(from) && validDate(to) && from <= to)) {
    throw new TypeError("Distribuição por categoria inválida.");
  }
  if (typeof item.currency_code !== "string" || !item.currency_code || !Array.isArray(item.items)) {
    throw new TypeError("Distribuição por categoria inválida.");
  }
  return {
    date_from: from as string | null, date_to: to as string | null,
    classification: item.classification, currency_code: item.currency_code,
    total, ...extras, items: item.items.map(parseCategorySpendingItem),
  };
}
