import { translate, type Language } from "../i18n/messages";
import type {
  InvestmentAccountKind,
  InvestmentOperationKind,
} from "../api/contracts";

export function investmentAccountKindName(kind: InvestmentAccountKind, language: Language = "pt-BR"): string {
  return kind === "manual" ? "Manual" : translate(language, "integratedFeminine");
}
export function investmentOperationKindName(kind: InvestmentOperationKind, language: Language = "pt-BR"): string {
  const keys = { initial_balance: "operationInitialBalance", deposit: "operationDeposit", withdrawal: "operationWithdrawal", buy: "operationBuy", sell: "operationSell", income: "operationIncome", fee: "operationFee", tax: "operationTax", valuation: "operationValuation", transfer_out: "operationTransferOut", transfer_in: "operationTransferIn" } as const;
  return translate(language, keys[kind]);
}
