import { dateFormatter, formatTime } from "../i18n/dateFormats";
import { translate, type Language } from "../i18n/messages";
import { isPositiveDecimal } from "./decimal";
import type { Category, TransactionItem } from "../api/contracts";
import type { RecurringCommitmentDraft } from "../components/recurringCommitments/RecurringCommitmentForm";
import { isZeroDecimal } from "./decimal";
import { categoryKindName } from "./categoryLabels";
import { formatBRL, formatSignedMoney } from "./money";

/** Pure derivations the transaction panel's screens share. */

export function detailValue(item: TransactionItem, language: Language = "pt-BR"): string {
  if (!item.effective_money) {
    return translate(language, "transferAmountUnavailable");
  }
  return formatSignedMoney(item.effective_money.value, item.classification, item.effective_money.currency_code);
}

/**
 * Linking a bank line to an aporte changes reporting, not cash: the bank still
 * moved the whole amount. So the headline value stays the full one and the
 * transferred parcel is spelled out beside it.
 */
export function investmentSplit(item: TransactionItem): { transferred: string; remaining: string } | null {
  if (isZeroDecimal(item.investment_transfer_amount)) return null;
  return { transferred: item.investment_transfer_amount, remaining: item.reportable_amount ?? "0" };
}

/**
 * The remainder lands on the side the line already belongs to (income for a
 * resgate, spending for an aporte). An ignored line has no side at all: its
 * reportable amount comes back as zero, and "R$ 0,00 remaining" would read as
 * a fully-linked line rather than one kept out of the totals.
 */
export function remainderLabel(item: TransactionItem, remaining: string, language: Language = "pt-BR"): string {
  if (item.inclusion.state === "ignored") return translate(language, "excludedLower");
  const side = translate(language, item.classification === "inflow" ? "remainingIncome" : "remainingExpense");
  return `${side}: ${formatBRL(remaining)}`;
}

/**
 * A credit-card line is not a cash movement between the bank and a custody
 * account, and a value with no BRL equivalent cannot be split, so the action
 * stays out of the way unless a link already exists to be undone.
 */
export function allowsInvestmentLink(item: TransactionItem): boolean {
  if (investmentSplit(item) !== null) return true;
  return (
    item.card === null &&
    item.effective_money?.currency_code === "BRL" &&
    item.classification !== "unclassified"
  );
}

export function dateTime(value: string | null, language: Language = "pt-BR"): string {
  if (!value) return translate(language, "dateNotProvided");
  return dateFormatter(language, { dateStyle: "long", timeStyle: "short" }).format(new Date(value));
}

/** "13 set. 2026 · 11:42" — the compact form the identity header uses. */
export function shortDateTime(value: string | null, language: Language = "pt-BR"): string {
  if (!value) return translate(language, "dateNotProvided");
  const date = new Date(value);
  const day = dateFormatter(language, { day: "numeric", month: "short", year: "numeric" }).format(date);
  const time = formatTime(language, date);
  return `${day} · ${time}`;
}

export function recurringCommitmentPrefill(item: TransactionItem): Partial<RecurringCommitmentDraft> | null {
  if (!item.effective_money || item.effective_money.currency_code !== "BRL" || !item.occurred_at) {
    return null;
  }
  const amount = item.effective_money.value.replace(/^-/, "");
  if (!isPositiveDecimal(amount)) return null;
  return {
    name: item.description ?? "",
    kind: item.classification === "inflow" ? "income" : "expense",
    amount,
    categoryId: item.internal_category?.id ?? null,
    dayOfMonth: new Date(item.occurred_at).getDate(),
  };
}

export function installmentLabel(card: TransactionItem["card"], language: Language = "pt-BR"): string | null {
  if (!card || card.installment_number == null || card.total_installments == null) return null;
  return translate(language, "installmentOf", { number: card.installment_number, total: card.total_installments });
}

function matchesClassification(kind: Category["kind"], classification: TransactionItem["classification"]): boolean {
  if (classification === "inflow") return kind === "income" || kind === "transfer";
  if (classification === "outflow") return kind === "expense" || kind === "transfer";
  return true;
}

export type CategoryChoice = {
  value: string;
  /** "Despesa: Lazer" — for contexts that list several natures flat. */
  label: string;
  name: string;
  kind: Category["kind"];
  isActive: boolean;
  icon: string;
  color: string;
};

/**
 * Categories the person may pick for this line: active ones on the same
 * side, plus whatever is vigente even if it has since been deactivated.
 */
export function categoryChoices(categories: Category[], item: TransactionItem, language: Language = "pt-BR"): CategoryChoice[] {
  const byId = new Map(
    categories
      .filter((category) => category.is_active && matchesClassification(category.kind, item.classification))
      .map((category) => [category.id, category]),
  );
  if (item.internal_category) {
    byId.set(item.internal_category.id, {
      id: item.internal_category.id,
      name: item.internal_category.name,
      kind: item.internal_category.kind,
      is_active: item.internal_category.is_active,
      icon: item.internal_category.icon,
      color: item.internal_category.color,
      created_at: "",
      updated_at: "",
    });
  }
  const kindOrder = ["expense", "income", "transfer"] as const;
  return [...byId.values()]
    .sort((a, b) => {
      if (a.kind !== b.kind) return kindOrder.indexOf(a.kind) - kindOrder.indexOf(b.kind);
      return a.name.localeCompare(b.name, "pt-BR");
    })
    .map((category) => ({
      value: category.id,
      label: `${categoryKindName(category.kind, language)}: ${category.name}${category.is_active ? "" : translate(language, "inactiveCategory")}`,
      name: category.name,
      kind: category.kind,
      isActive: category.is_active,
      icon: category.icon,
      color: category.color,
    }));
}

/** Case- and accent-insensitive form for matching names typed by people or sent by providers. */
export function fold(value: string): string {
  return value.normalize("NFD").replace(/\p{M}/gu, "").trim().toLocaleLowerCase("pt-BR");
}

/**
 * The provider's category is free text, not one of ours. When its name
 * matches a category the person could pick, "Usar" can apply it in one tap;
 * otherwise the suggestion is only informative.
 */
export function suggestedCategory(item: TransactionItem, choices: CategoryChoice[]): CategoryChoice | null {
  if (!item.source_category) return null;
  const wanted = fold(item.source_category);
  return choices.find((choice) => fold(choice.name) === wanted) ?? null;
}
