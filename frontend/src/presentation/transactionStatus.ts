import { translate, type Language } from "../i18n/messages";
import type { TransactionClassification, TransactionItem } from "../api/contracts";

export const classificationColor: Record<TransactionClassification, string> = {
  inflow: "green",
  outflow: "red",
  unclassified: "default",
};

export function inclusionOriginLabel(inclusion: TransactionItem["inclusion"], language: Language = "pt-BR"): string {
  const action = translate(language, inclusion.state === "ignored" ? "ignoredFeminine" : "restoredFeminine");
  if (inclusion.origin === "rule") {
    return inclusion.rule_name ? translate(language, "inclusionByRule", { action, name: inclusion.rule_name }) : translate(language, "inclusionByUnnamedRule", { action });
  }
  return translate(language, "inclusionManually", { action });
}

export type ExclusionReason =
  | "ignored" | "linked_transfer" | "transfer_category" | "unclassified" | "ineligible_status" | "missing_money_pair" | "zero_value" | "investment_transfer";

export function exclusionReason(reason: ExclusionReason, language: Language = "pt-BR"): string {
  const keys = { ignored: "reasonIgnored", linked_transfer: "reasonLinkedTransfer", transfer_category: "reasonTransferCategory", unclassified: "reasonUnclassified", ineligible_status: "reasonStatus", missing_money_pair: "reasonMissingMoney", zero_value: "reasonZero", investment_transfer: "reasonInvestment" } as const;
  return translate(language, keys[reason]);
}
