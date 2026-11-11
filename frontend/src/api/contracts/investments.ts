import { isNullableString, isUuid, nullableDate, nullableDecimal, requiredRecord } from "./shared";

/**
 * Why the backend could not state a yield. "sem_historico": no movements
 * synced at all. "historico_incompleto": movements arrived, but they cannot
 * account for the position (no aplicação behind the resgates, more cotas sold
 * than bought, or a movement whose direction or amount is unknown) — netting
 * them would report principal as profit. "saldo_indisponivel": the history
 * nets fine, there is just no current balance to net it against.
 */
export const yieldUnavailableReasons = [
  "sem_historico",
  "historico_incompleto",
  "saldo_indisponivel",
] as const;

export type YieldUnavailableReason = (typeof yieldUnavailableReasons)[number];

export interface Investment {
  id: string;
  external_id: string;
  source_display_name: string | null;
  investment_type: string | null;
  subtype: string | null;
  name: string | null;
  balance: string | null;
  currency_code: string | null;
  quantity: string | null;
  value: string | null;
  amount: string | null;
  amount_profit: string | null;
  amount_withdrawal: string | null;
  /** Principal originally applied (Pluggy's amountOriginal). FIXED_INCOME only, null for EQUITY. */
  amount_original: string | null;
  /** IR (income tax) provisioned on the position (Pluggy's taxes). FIXED_INCOME only, null for EQUITY. */
  taxes: string | null;
  /** IOF, regressive, nonzero only in the first 30 days (Pluggy's taxes2). FIXED_INCOME only, null for EQUITY. */
  taxes2: string | null;
  rate: string | null;
  rate_type: string | null;
  fixed_annual_rate: string | null;
  annual_rate: string | null;
  last_twelve_months_rate: string | null;
  issuer: string | null;
  due_date: string | null;
  as_of_date: string | null;
  provider_updated_at: string | null;
  yield_value: string | null;
  yield_source: "informado" | "calculado" | null;
  /** Why yield_value is null. Always null when yield_value is present. */
  yield_unavailable_reason: YieldUnavailableReason | null;
}

const investmentKeys = [
  "id",
  "external_id",
  "source_display_name",
  "investment_type",
  "subtype",
  "name",
  "balance",
  "currency_code",
  "quantity",
  "value",
  "amount",
  "amount_profit",
  "amount_withdrawal",
  "amount_original",
  "taxes",
  "taxes2",
  "rate",
  "rate_type",
  "fixed_annual_rate",
  "annual_rate",
  "last_twelve_months_rate",
  "issuer",
  "due_date",
  "as_of_date",
  "provider_updated_at",
  "yield_value",
  "yield_source",
  "yield_unavailable_reason",
] as const;

export function parseInvestment(value: unknown): Investment {
  const item = requiredRecord(value, investmentKeys, "Investimento inválido.");
  if (
    typeof item.id !== "string" ||
    !isUuid(item.id) ||
    typeof item.external_id !== "string" ||
    item.external_id === "" ||
    !isNullableString(item.source_display_name) ||
    !isNullableString(item.investment_type) ||
    !isNullableString(item.subtype) ||
    !isNullableString(item.name) ||
    !isNullableString(item.currency_code) ||
    !isNullableString(item.rate_type) ||
    !isNullableString(item.issuer) ||
    !(item.yield_source === null || item.yield_source === "informado" || item.yield_source === "calculado") ||
    (item.yield_source === null) !== (item.yield_value === null) ||
    !(item.yield_unavailable_reason === null ||
      yieldUnavailableReasons.includes(item.yield_unavailable_reason as YieldUnavailableReason))
  ) {
    throw new TypeError("Investimento inválido.");
  }
  return {
    id: item.id,
    external_id: item.external_id,
    source_display_name: item.source_display_name,
    investment_type: item.investment_type,
    subtype: item.subtype,
    name: item.name,
    balance: nullableDecimal(item.balance),
    currency_code: item.currency_code,
    quantity: nullableDecimal(item.quantity),
    value: nullableDecimal(item.value),
    amount: nullableDecimal(item.amount),
    amount_profit: nullableDecimal(item.amount_profit),
    amount_withdrawal: nullableDecimal(item.amount_withdrawal),
    amount_original: nullableDecimal(item.amount_original),
    taxes: nullableDecimal(item.taxes),
    taxes2: nullableDecimal(item.taxes2),
    rate: nullableDecimal(item.rate),
    rate_type: item.rate_type,
    fixed_annual_rate: nullableDecimal(item.fixed_annual_rate),
    annual_rate: nullableDecimal(item.annual_rate),
    last_twelve_months_rate: nullableDecimal(item.last_twelve_months_rate),
    issuer: item.issuer,
    due_date: nullableDate(item.due_date),
    as_of_date: nullableDate(item.as_of_date),
    provider_updated_at: nullableDate(item.provider_updated_at),
    yield_value: nullableDecimal(item.yield_value),
    yield_source: item.yield_source as "informado" | "calculado" | null,
    yield_unavailable_reason: item.yield_unavailable_reason as YieldUnavailableReason | null,
  };
}

export function parseInvestmentList(value: unknown): Investment[] {
  if (!Array.isArray(value)) {
    throw new TypeError("Lista de investimentos inválida.");
  }
  return value.map(parseInvestment);
}

/** Which way the money moved, normalized by the backend at ingestion. */
export const investmentDirections = ["inflow", "outflow"] as const;

export type InvestmentDirection = (typeof investmentDirections)[number];

/** Who recorded a movement: the provider's sync, or the owner for what it never reported. */
export const investmentTransactionOrigins = ["provider", "manual"] as const;

export type InvestmentTransactionOrigin = (typeof investmentTransactionOrigins)[number];

export interface InvestmentTransaction {
  id: string;
  external_id: string;
  origin: InvestmentTransactionOrigin;
  movement_type: string | null;
  direction: InvestmentDirection | null;
  quantity: string | null;
  value: string | null;
  amount: string | null;
  occurred_at: string | null;
  trade_date: string | null;
  notes: string | null;
}

/** The kinds a manual movement can have, in the provider's vocabulary. */
export const investmentMovementKinds = ["BUY", "SELL", "INTEREST"] as const;

export type InvestmentMovementKind = (typeof investmentMovementKinds)[number];

/** `amount` is in reais, costs included; the server derives it from quantity × unit price when absent. */
export interface InvestmentMovementWrite {
  investment_id: string;
  kind: InvestmentMovementKind;
  trade_date: string;
  quantity?: string | null;
  unit_price?: string | null;
  amount?: string | null;
  notes?: string | null;
}

const investmentTransactionKeys = [
  "id",
  "external_id",
  "origin",
  "movement_type",
  "direction",
  "quantity",
  "value",
  "amount",
  "occurred_at",
  "trade_date",
  "notes",
] as const;

export function parseInvestmentTransaction(value: unknown): InvestmentTransaction {
  const item = requiredRecord(value, investmentTransactionKeys, "Movimentação de investimento inválida.");
  if (
    typeof item.id !== "string" ||
    !isUuid(item.id) ||
    typeof item.external_id !== "string" ||
    item.external_id === "" ||
    !investmentTransactionOrigins.includes(item.origin as InvestmentTransactionOrigin) ||
    !isNullableString(item.movement_type) ||
    !isNullableString(item.notes) ||
    !(item.direction === null || investmentDirections.includes(item.direction as InvestmentDirection))
  ) {
    throw new TypeError("Movimentação de investimento inválida.");
  }
  return {
    id: item.id,
    external_id: item.external_id,
    origin: item.origin as InvestmentTransactionOrigin,
    movement_type: item.movement_type,
    direction: item.direction as InvestmentDirection | null,
    quantity: nullableDecimal(item.quantity),
    value: nullableDecimal(item.value),
    amount: nullableDecimal(item.amount),
    occurred_at: nullableDate(item.occurred_at),
    trade_date: nullableDate(item.trade_date),
    notes: item.notes,
  };
}

export function parseInvestmentTransactionList(value: unknown): InvestmentTransaction[] {
  if (!Array.isArray(value)) {
    throw new TypeError("Lista de movimentações de investimento inválida.");
  }
  return value.map(parseInvestmentTransaction);
}
