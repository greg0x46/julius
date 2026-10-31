import { translate, type Language } from "../i18n/messages";
import type { ScenarioKind, ScenarioTransactionStatus } from "../api/contracts";

export function scenarioKindName(kind: ScenarioKind, language: Language = "pt-BR"): string {
  const keys = { debt_plan: "scenarioKindDebtPlan", receivable_plan: "scenarioKindReceivablePlan", standalone: "scenarioKindStandalone", recurring: "scenarioKindRecurring" } as const;
  return translate(language, keys[kind]);
}

export const scenarioTransactionStatusColor: Record<ScenarioTransactionStatus, string> = {
  atrasada: "error",
  projetada: "default",
  paga_parcialmente: "warning",
  paga: "success",
  paga_a_mais: "processing",
};

// A "projetada" tag renders dashed, per the spec: it's the one status that
// hasn't happened yet (unlike atrasada, which is also unpaid but already
// past its date), so it needs to read visually different from the "paga*"
// tags that represent a real, already-happened allocation.
export const scenarioTransactionStatusDashed: Record<ScenarioTransactionStatus, boolean> = {
  atrasada: false,
  projetada: true,
  paga_parcialmente: false,
  paga: false,
  paga_a_mais: false,
};

export function scenarioTransactionStatus(status: ScenarioTransactionStatus, language: Language = "pt-BR"): string {
  const keys = { atrasada: "installmentStatusOverdue", projetada: "installmentStatusPlanned", paga_parcialmente: "installmentStatusPartial", paga: "installmentStatusPaid", paga_a_mais: "installmentStatusOverpaid" } as const;
  return translate(language, keys[status]);
}
