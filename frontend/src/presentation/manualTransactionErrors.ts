import { translate, type Language } from "../i18n/messages";
import { ApiError } from "../api/problems";

/**
 * The API refuses to edit or delete a manual bank line that still has parcels
 * linked to aportes/resgates, because rewriting it would silently change what
 * the investment ledger was reconciled against. The generic problem detail
 * does not say where to undo those links, so this spells the next step out.
 */
const investmentLinkedProblem = "/problems/investment-linked-transaction";

export function isInvestmentLinkedProblem(error: unknown): boolean {
  return error instanceof ApiError && error.problem?.type === investmentLinkedProblem;
}

export function manualTransactionErrorMessage(error: unknown, action: "save" | "delete", language: Language = "pt-BR"): string {
  if (isInvestmentLinkedProblem(error)) {
    return action === "delete"
      ? translate(language, "investmentLinkedDeleteError")
      : translate(language, "investmentLinkedEditError");
  }
  if (error instanceof Error) return error.message;
  return action === "delete"
    ? translate(language, "manualTransactionDeleteError")
    : translate(language, "manualTransactionSaveError");
}
