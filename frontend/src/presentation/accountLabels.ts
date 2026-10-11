import type { Account, AccountClosingDaySource, AccountType } from "../api/contracts";
import { formatBRL, formatMoney } from "./money";

const accountTypeLabels: Record<AccountType, string> = {
  BANK: "Conta bancária",
  CREDIT: "Cartão de crédito",
};

export function accountTypeLabel(type: AccountType | null): string {
  if (type === null) return "Tipo não informado";
  return accountTypeLabels[type];
}

// Pluggy's account subtype is free text from the provider, not a closed enum
// github.com/greg0x46/julius validates, so this map only translates the values we've
// actually seen — anything else falls back to the raw string.
const accountSubtypeLabels: Record<string, string> = {
  CHECKING_ACCOUNT: "Conta corrente",
  SAVINGS_ACCOUNT: "Poupança",
  CREDIT_CARD: "Cartão de crédito",
};

export function accountSubtypeLabel(subtype: string | null): string | null {
  if (subtype === null) return null;
  return accountSubtypeLabels[subtype] ?? subtype;
}

export function closingDayLabel(day: number | null): string {
  return day === null ? "Não informado" : `Todo dia ${day}`;
}

const closingDaySourceHints: Record<AccountClosingDaySource, string> = {
  manual: "Definido por você",
  informado: "Informado pela instituição",
  estimado: "Estimado a partir da última fatura fechada",
};

export function closingDaySourceHint(source: AccountClosingDaySource | null): string {
  return source === null
    ? "A instituição não informa o fechamento deste cartão — defina o dia manualmente"
    : closingDaySourceHints[source];
}

export function maskedCardNumber(number: string): string {
  return `•••• ${number}`;
}

/** An account's own number is the full string the institution sends (often
 *  with a check digit or dashes); only the last four digits are meaningful
 *  as identity, so that's all this shows. */
export function maskedAccountNumber(number: string | null): string | null {
  if (number === null) return null;
  const digits = number.replace(/\D/g, "");
  if (digits === "") return null;
  return maskedCardNumber(digits.slice(-4));
}

/** Formats a 0–1 usage ratio as a pt-BR percentage. Values above 100% are
 *  real (the limit was exceeded) and are shown as-is. */
export function creditUsagePercent(ratio: string): string {
  return `${(Number(ratio) * 100).toLocaleString("pt-BR", { maximumFractionDigits: 1 })}%`;
}

/** The share of the limit to paint in the meter, clamped so an over-limit
 *  card still renders a full bar instead of overflowing it. */
export function creditUsageShare(ratio: string): number {
  return Math.min(100, Math.max(0, Number(ratio) * 100));
}

export type CreditUsageLevel = "normal" | "warning" | "critical";

/** No domain rule pins these thresholds today, so the meter escalates color
 *  only once usage genuinely needs attention: comfortable below 70%, a
 *  heads-up from 70%, critical from 90%. */
export function creditUsageLevel(ratio: string): CreditUsageLevel {
  const share = Number(ratio) * 100;
  if (share >= 90) return "critical";
  if (share >= 70) return "warning";
  return "normal";
}

export function isCreditAccount(account: Account): boolean {
  return account.account_type === "CREDIT";
}

/**
 * The record's one name, in the list row and as the detail page's heading
 * alike: the account's own name ("Conta Corrente", "Cartão Platinum" — what
 * tells two accounts of one bank apart), falling back to the short
 * institution name for an account that has none. The institution rides
 * along on the identity line instead (see `accountMetaLine`).
 */
export function accountDisplayName(account: Account): string {
  if (account.name !== null) return account.name;
  return account.institution_name !== null ? shortInstitutionName(account.institution_name) : "Conta sem nome";
}

// A registered institution name often carries a corporate suffix and a
// trailing descriptor ("Nu Pagamentos S.A. - Instituição de Pagamento") that
// nobody needs to recognize the bank — the part before the first " - " is
// already the brand, and the suffix on it is noise.
const institutionSuffixPattern = /\s+(s\.?\s?a\.?|s\/a|ltda\.?|eireli\.?)$/i;

export function shortInstitutionName(name: string): string {
  const brand = name.split(" - ")[0].trim();
  const short = brand.replace(institutionSuffixPattern, "").trim();
  return short || brand;
}

/** The account detail page's heading. It is the same name the list row shows
 *  (`accountDisplayName`): a record is not called one thing in the list and
 *  another once opened. */
export function accountHeaderTitle(account: Account): string {
  return accountDisplayName(account);
}

/** "Conta corrente · •••• 0966" — the compact identity line under a name,
 *  shared by the account list rows and the detail page header. */
export function accountIdentityLine(account: Account): string {
  const parts = [accountSubtypeLabel(account.account_subtype), maskedAccountNumber(account.number)];
  return parts.filter((part): part is string => part !== null).join(" · ");
}

/** "Conta corrente · •••• 0966 · Nu Pagamentos": the identity line plus the
 *  institution — the part of the account the name does not already carry. Only
 *  the parts that exist, so an account with nothing to say gets "" and no line,
 *  never a lone dash. Shared by the list rows and the detail header. */
export function accountMetaLine(account: Account): string {
  if (account.origin === "manual") return ["Conta manual", account.currency_code, account.archived_at ? "Arquivada" : ""].filter(Boolean).join(" · ");
  const name = accountDisplayName(account);
  const institution = account.institution_name !== null ? shortInstitutionName(account.institution_name) : "";
  return [accountIdentityLine(account), institution !== name ? institution : ""]
    .filter((part) => part !== "")
    .join(" · ");
}

/** Accounts carry their own currency, unlike the rest of the app, which is
 *  BRL by construction — so a non-BRL account keeps its own code instead of
 *  being mislabelled with a real. */
export function formatAccountMoney(value: string | null, currencyCode: string | null): string {
  if (value === null) return "—";
  if (currencyCode === null || currencyCode === "BRL") return formatBRL(value);
  return formatMoney(value, currencyCode);
}
