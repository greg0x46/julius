/**
 * Formats a decimal string in any currency. BRL goes through `formatBRL`
 * ("R$ 1.234,00") so every screen shows the same symbol and scale; other
 * currencies keep their code prefix and any extra precision, padded to at
 * least two decimals.
 */
export function formatMoney(value: string, currencyCode: string): string {
  if (currencyCode === "BRL") return formatBRL(value);
  const negative = value.startsWith("-");
  const unsigned = negative ? value.slice(1) : value;
  const [integer, fraction = ""] = unsigned.split(".");
  const grouped = (integer ?? "0").replace(/\B(?=(\d{3})+(?!\d))/g, ".");
  return `${negative ? "-" : ""}${currencyCode}\u00a0${grouped},${fraction.padEnd(2, "0")}`;
}

function fixedTwo(value: string): string {
  const negative = value.startsWith("-");
  const unsigned = negative ? value.slice(1) : value;
  const [integer = "0", fraction = ""] = unsigned.split(".");
  const padded = fraction.padEnd(3, "0");
  let cents = BigInt(`${integer}${padded.slice(0, 2)}`);
  if (padded[2] >= "5") cents += 1n;
  const digits = cents.toString().padStart(3, "0");
  const whole = digits.slice(0, -2).replace(/\B(?=(\d{3})+(?!\d))/g, ".");
  const decimals = digits.slice(-2);
  return `${negative ? "-" : ""}${whole},${decimals}`;
}

export function formatBRL(value: string): string {
  return `${value.startsWith("-") ? "-" : ""}R$\u00a0${fixedTwo(value).replace("-", "")}`;
}

export function formatSignedBRL(
  value: string,
  classification: "inflow" | "outflow" | "unclassified",
): string {
  const unsigned = value.startsWith("-") ? value.slice(1) : value;
  const sign = classification === "inflow" ? "+" : classification === "outflow" ? "-" : "";
  return `${sign}R$\u00a0${fixedTwo(unsigned)}`;
}

export const moneySourceLabel = (source: "account_currency" | "transaction_currency") =>
  source === "account_currency" ? "Valor na moeda da conta" : "Valor original";

// toCents/fromCents let callers sum a handful of BRL decimal strings
// (e.g. accumulating several MonthSummary rows client-side) without a
// bignum dependency — same BigInt-cents technique fixedTwo already uses,
// just exposed for addition instead of only formatting.
function toCents(value: string): bigint {
  const negative = value.startsWith("-");
  const unsigned = negative ? value.slice(1) : value;
  const [integer = "0", fraction = ""] = unsigned.split(".");
  const cents = BigInt(`${integer}${fraction.padEnd(2, "0").slice(0, 2)}`);
  return negative ? -cents : cents;
}

function fromCents(cents: bigint): string {
  const negative = cents < 0n;
  const digits = (negative ? -cents : cents).toString().padStart(3, "0");
  const whole = digits.slice(0, -2);
  const decimals = digits.slice(-2);
  return `${negative ? "-" : ""}${whole}.${decimals}`;
}

export function sumBRL(values: string[]): string {
  return fromCents(values.reduce((total, value) => total + toCents(value), 0n));
}

export function subtractBRL(minuend: string, subtrahend: string): string {
  return fromCents(toCents(minuend) - toCents(subtrahend));
}

/** The amount itself, or "0.00" when it is negative; "-0.00" counts as zero. */
export function clampBRLAtZero(value: string): string {
  const cents = toCents(value);
  return fromCents(cents < 0n ? 0n : cents);
}

/** The smallest of one or more amounts. */
export function minBRL(values: string[]): string {
  if (values.length === 0) throw new RangeError("minBRL needs at least one value");
  return fromCents(values.map(toCents).reduce((min, cents) => (cents < min ? cents : min)));
}

/**
 * Splits a non-negative amount into `divisor` shares, dropping the leftover
 * fraction of a cent. BigInt division truncates toward zero, which is only a
 * floor for non-negative amounts, so a negative one is rejected.
 */
export function divideBRLFloor(value: string, divisor: number): string {
  if (!Number.isInteger(divisor) || divisor <= 0) throw new RangeError("divisor must be a positive integer");
  const cents = toCents(value);
  if (cents < 0n) throw new RangeError("value must not be negative");
  return fromCents(cents / BigInt(divisor));
}

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
 *   "-" danger.
 * - balance (saldo, patrimônio): negative is danger; otherwise default
 *   colour, never a "+".
 * - neutral: default colour, no sign logic.
 * Zero is never signed or tinted.
 */
export function moneyTone(value: string, kind: MoneyToneKind, direction?: MoneyDirection): MoneyTone {
  const zero = isZeroDecimal(value);
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
      return negative ? { sign: "-", color: "negative" } : { sign: "+", color: "positive" };
    case "balance":
      return { sign: negative ? "-" : "", color: negative ? "negative" : "neutral" };
    case "neutral":
      return { sign: "", color: "neutral" };
  }
}

/**
 * The BRL text for a figure under the tone rule above: flow and result carry
 * the explicit "+"/"-" from `moneyTone`; balance and neutral show only the
 * natural minus of a negative value.
 */
export function formatToneBRL(value: string, kind: MoneyToneKind, direction?: MoneyDirection): string {
  const magnitude = formatBRL(value.trim().replace(/^-/, ""));
  if (kind === "flow" || kind === "result") return `${moneyTone(value, kind, direction).sign}${magnitude}`;
  return isZeroDecimal(value) || !value.trim().startsWith("-") ? magnitude : `-${magnitude}`;
}
