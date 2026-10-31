import { translate, type Language } from "../i18n/messages";
import { subtractDecimals, sumDecimals } from "./decimal";
import type { Investment, InvestmentTransaction } from "../api/contracts";

// Pluggy's investment "type"/"subtype"/movement "type" fields are free text
// from the provider (not a closed enum github.com/greg0x46/julius validates), so these
// maps only translate the values we've actually seen — anything else falls
// back to the raw string instead of failing.
/**
 * A provider value with no label of its own: a raw enum ("OTHER_MOVEMENT")
 * reads as a bug, so it becomes sentence case ("Other movement"); anything
 * that already looks like text is kept as the provider wrote it.
 */
function humanizeProviderValue(value: string): string {
  if (!/^[A-Z0-9]+(_[A-Z0-9]+)*$/.test(value)) return value;
  const words = value.toLowerCase().replace(/_/g, " ");
  return words.charAt(0).toUpperCase() + words.slice(1);
}

export function investmentTypeLabel(type: string | null, language: Language = "pt-BR"): string {
  if (type === null) return translate(language, "unknownAccountType");
  if (type === "COE" || type === "ETF") return type;
  const keys = { MUTUAL_FUND: "investmentTypeFund", FIXED_INCOME: "assetClassFixedIncome", EQUITY: "assetClassVariableIncome", SECURITY: "investmentTypeSecurity", PENSION: "investmentTypePension" } as const;
  const key = keys[type as keyof typeof keys];
  return key ? translate(language, key) : humanizeProviderValue(type);
}

/**
 * A decimal string ("10.65", "0.0215", "1234.5") the way pt-BR writes it:
 * comma as the mark, dots between thousands, no trailing zeros ("100.00" is
 * "100"). Works on the text, so nothing passes through a float.
 */
export function formatDecimal(value: string, language: Language = "pt-BR"): string {
  const negative = value.startsWith("-");
  const unsigned = negative ? value.slice(1) : value;
  const [integer = "0", fraction = ""] = unsigned.split(".");
  const grouped = integer.replace(/\B(?=(\d{3})+(?!\d))/g, language === "en" ? "," : ".");
  const decimals = fraction.replace(/0+$/, "");
  return `${negative ? "-" : ""}${grouped}${decimals === "" ? "" : `${language === "en" ? "." : ","}${decimals}`}`;
}

/** "1 cota", "0,2 cotas": the unit agrees with the number. */
export function quotaCount(quantity: string, language: Language = "pt-BR"): string {
  return `${formatDecimal(quantity, language)} ${translate(language, Number(quantity) === 1 ? "quotaOne" : "quotaMany")}`;
}

export function movementTypeLabel(type: string | null, language: Language = "pt-BR"): string {
  if (type === null) return translate(language, "movementGeneric");
  const keys = { BUY: "movementBuy", SELL: "movementSell", APPLICATION: "movementBuy", REDEMPTION: "movementSell", DIVIDEND: "movementDividend", INTEREST: "movementInterest", INCOME: "movementInterest", TAX: "movementTax", FEE: "movementFee", TRANSFER: "movementTransfer", OTHER_MOVEMENT: "movementOther" } as const;
  const key = keys[type as keyof typeof keys];
  return key ? translate(language, key) : humanizeProviderValue(type);
}

export type YieldEstimate = {
  value: string;
  source: "informado" | "calculado";
} | null;

// The backend computes this (GET /api/investments[/:id]): Pluggy's own
// amount_profit when the provider sends it ("informado"), otherwise balance
// minus net contributed from the investment's buy/sell history ("calculado").
// Computed server-side so the list page gets it without an N+1 fetch of every
// investment's transactions. Do not resurrect a client-side balance - amount
// fallback: investment.amount for renda fixa holdings is quantity × value
// (current gross mark value), not invested principal, and using it produced
// false "negative yields" on healthy CDBs (see two CDBs flagged in August
// 2026 — the gap was withheld tax on early redemption, not a loss).
export function investmentYield(investment: Investment): YieldEstimate {
  if (investment.yield_value === null || investment.yield_source === null) return null;
  return { value: investment.yield_value, source: investment.yield_source };
}

/** What to show in place of a yield the backend refused to state, and why. */
export function yieldUnavailable(investment: Investment, language: Language = "pt-BR"): { label: string; hint: string } {
  const reason = investment.yield_unavailable_reason;
  if (reason === null) return { label: translate(language, "yieldNotAvailable"), hint: "" };
  const keys = {
    sem_historico: ["yieldNoHistory", "yieldNoHistoryHint"],
    historico_incompleto: ["yieldIncompleteHistory", "yieldIncompleteHistoryHint"],
    saldo_indisponivel: ["yieldNoCurrentBalance", "yieldNoBalanceHint"],
  } as const;
  return { label: translate(language, keys[reason][0]), hint: translate(language, keys[reason][1]) };
}

// Returns null when the history cannot be netted: a movement whose direction
// the backend could not establish leaves the sum ambiguous, one with no
// amount leaves a hole in it, and an all-outflow history means the original
// purchase was never captured, so there's no principal to net the redemptions
// against. Skipping either kind of movement and totalling the rest is the
// worst option — what is missing is invisible in the result. This mirrors
// httpapi.historyCovers, which refuses the same three shapes.
//
// Direction comes from the normalized `direction` field, never from
// movement_type. Classifying by movement type is what made INTEREST — a
// dividend leaving the investment — count as an aporte.
export function netContributed(transactions: InvestmentTransaction[]): string | null {
  if (transactions.some((t) => t.direction === null || t.amount === null)) return null;
  if (!transactions.some((t) => t.direction === "inflow")) return null;
  return sumDecimals(transactions.map((t) => t.direction === "inflow" ? t.amount! : subtractDecimals("0", t.amount!)));
}
