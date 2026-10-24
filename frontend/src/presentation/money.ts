import { compareDecimals, divideDecimalFloor, roundDecimal, subtractDecimals, sumDecimals } from "./decimal";

/**
 * Formats a decimal string in any currency. BRL goes through `formatBRL`
 * ("R$ 1.234,00") so every screen shows the same symbol and scale; other
 * currencies keep their code prefix and any extra precision, padded to at
 * least two decimals.
 */
export function formatMoney(value: string, currencyCode: string): string {
  if (currencyCode === "BRL") return formatBRL(value);
  const negative = !isZeroDecimal(value) && value.startsWith("-");
  const unsigned = value.replace(/^-/, "");
  const [integer, fraction = ""] = unsigned.split(".");
  const grouped = (integer ?? "0").replace(/\B(?=(\d{3})+(?!\d))/g, ".");
  return `${negative ? "-" : ""}${currencyCode}\u00a0${grouped},${fraction.padEnd(2, "0")}`;
}

function fixedTwo(value: string): string {
  const [integer, fraction] = roundDecimal(value, 2).replace(/^-/, "").split(".");
  return `${integer.replace(/\B(?=(\d{3})+(?!\d))/g, ".")},${fraction}`;
}

export function formatBRL(value: string): string {
  const rounded = roundDecimal(value, 2);
  return `${rounded.startsWith("-") ? "-" : ""}R$\u00a0${fixedTwo(rounded)}`;
}

/** `formatToneMoney` for a consolidated (reference-currency) figure. */
export function formatToneReportMoney(value: string, currencyCode: string, kind: MoneyToneKind, direction?: MoneyDirection): string {
  const magnitude = formatReportMoney(value.trim().replace(/^-/, ""), currencyCode);
  if (kind === "flow" || kind === "result") return `${moneyTone(value, kind, direction, currencyCode).sign}${magnitude}`;
  return isZeroDecimal(roundDecimal(value, 2)) || !value.trim().startsWith("-") ? magnitude : `-${magnitude}`;
}

export function formatSignedMoney(
  value: string,
  classification: "inflow" | "outflow" | "unclassified",
  currencyCode: string,
): string {
  const unsigned = value.startsWith("-") ? value.slice(1) : value;
  const zero = currencyCode === "BRL" ? roundDecimal(unsigned, 2) === "0.00" : isZeroDecimal(unsigned);
  const sign = zero ? "" : classification === "inflow" ? "+" : classification === "outflow" ? "-" : "";
  return `${sign}${formatMoney(unsigned, currencyCode)}`;
}

export const moneySourceLabel = (source: "account_currency" | "transaction_currency") =>
  source === "account_currency" ? "Valor na moeda da conta" : "Valor original";

/** Exact arithmetic; cents are a display boundary, not an accumulation unit. */
export const sumBRL = sumDecimals;
export const subtractBRL = subtractDecimals;

export function clampBRLAtZero(value: string): string {
  return compareDecimals(value, "0") < 0 ? "0.00" : sumDecimals([value]);
}

export function minBRL(values: string[]): string {
  if (values.length === 0) throw new RangeError("minBRL needs at least one value");
  return sumDecimals([values.reduce((min, value) => compareDecimals(value, min) < 0 ? value : min)]);
}

export const divideBRLFloor = divideDecimalFloor;

/** How a figure reads: individual transaction, net result, stock balance, or plain. */
export type MoneyToneKind = "flow" | "result" | "balance" | "neutral";

export type MoneyDirection = "inflow" | "outflow" | "unclassified";

export interface MoneyTone {
  /** Explicit sign to show before the amount ("" when none). */
  sign: "+" | "-" | "";
  /** Which colour the figure takes; the stylesheet maps it to a token. */
  color: "positive" | "negative" | "neutral";
}

function isZeroDecimal(value: string): boolean {
  return /^-?0*(\.0*)?$/.test(value.trim());
}

/**
 * The money display rule, in one place:
 * - flow (a transaction row): inflow is "+" in the money-in colour; outflow
 *   is "-" in the default text colour (an expense is normal, not an alarm).
 *   `direction` overrides the value's sign when the value is an unsigned
 *   magnitude, as a transaction's amount is.
 * - result (period result, net figures): positive "+" money-in, negative
 *   "-" in the default text colour (a loss is not an error).
 * - balance (saldo, patrimônio): negative is danger; otherwise default
 *   colour, never a "+".
 * - neutral: default colour, no sign logic.
 * Zero is never signed or tinted.
 */
export function moneyTone(value: string, kind: MoneyToneKind, direction?: MoneyDirection, currencyCode = "BRL"): MoneyTone {
  const zero = currencyCode === "BRL" ? isZeroDecimal(roundDecimal(value, 2)) : isZeroDecimal(value);
  if (zero) return { sign: "", color: "neutral" };
  const negative = !zero && value.trim().startsWith("-");
  switch (kind) {
    case "flow": {
      const flow = direction ?? (zero ? "unclassified" : negative ? "outflow" : "inflow");
      if (flow === "inflow") return { sign: "+", color: "positive" };
      if (flow === "outflow") return { sign: "-", color: "neutral" };
      return { sign: "", color: "neutral" };
    }
    case "result":
      if (zero) return { sign: "", color: "neutral" };
      return negative ? { sign: "-", color: "neutral" } : { sign: "+", color: "positive" };
    case "balance":
      return { sign: negative ? "-" : "", color: negative ? "negative" : "neutral" };
    case "neutral":
      return { sign: "", color: "neutral" };
  }
}

export function formatToneMoney(value: string, currencyCode: string, kind: MoneyToneKind, direction?: MoneyDirection): string {
  const magnitude = formatMoney(value.trim().replace(/^-/, ""), currencyCode);
  if (kind === "flow" || kind === "result") return `${moneyTone(value, kind, direction, currencyCode).sign}${magnitude}`;
  const zero = currencyCode === "BRL" ? isZeroDecimal(roundDecimal(value, 2)) : isZeroDecimal(value);
  return zero || !value.trim().startsWith("-") ? magnitude : `-${magnitude}`;
}

export function formatToneBRL(value: string, kind: MoneyToneKind, direction?: MoneyDirection): string {
  return formatToneMoney(value, "BRL", kind, direction);
}

export function formatSignedBRL(value: string, classification: MoneyDirection): string {
  return formatSignedMoney(value, classification, "BRL");
}

/** Consolidated figures keep the same two-decimal presentation in every currency. */
export function formatReportMoney(value: string, currencyCode: string): string {
  return currencyCode === "BRL"
    ? formatBRL(value)
    : `${value.startsWith("-") ? "-" : ""}${currencyCode}\u00a0${fixedTwo(value).replace("-", "")}`;
}
