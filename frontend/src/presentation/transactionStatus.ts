import type { TransactionClassification, TransactionItem } from "../api/contracts";

export const classificationLabel: Record<TransactionClassification, string> = {
  inflow: "Entrada",
  outflow: "Saída",
  unclassified: "Não classificada",
};

export const classificationColor: Record<TransactionClassification, string> = {
  inflow: "green",
  outflow: "red",
  unclassified: "default",
};

export function inclusionOriginLabel(inclusion: TransactionItem["inclusion"]): string {
  const action = inclusion.state === "ignored" ? "Ignorada" : "Restaurada";
  if (inclusion.origin === "rule") {
    return inclusion.rule_name ? `${action} pela regra "${inclusion.rule_name}"` : `${action} por regra`;
  }
  return `${action} manualmente`;
}

export const exclusionReasonLabel = {
  ignored: "Fora dos totais: transação ignorada",
  linked_transfer: "Fora dos totais: transferência vinculada entre contas próprias",
  transfer_category: "Fora dos totais: transferência entre contas próprias",
  unclassified: "Fora dos totais: tipo não classificado",
  ineligible_status: "Fora dos totais: situação não elegível",
  missing_money_pair: "Fora dos totais: valor e moeda incompletos",
  zero_value: "Fora dos totais: valor zero",
  investment_transfer: "Fora dos totais: valor transferido para investimento",
} as const;
