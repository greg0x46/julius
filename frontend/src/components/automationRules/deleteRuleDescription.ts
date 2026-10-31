import { translate, type Language } from "../../i18n/messages";
import type { AutomationRule } from "../../api/contracts";

/** What deleting a rule leaves untouched — shown by every "Excluir automação" confirmation. */
export function deleteRuleDescription(rule: AutomationRule, language: Language = "pt-BR"): string {
  return translate(language, rule.actions.some((action) => action.type === "reconcile") ? "ruleDeleteWithReconcile" : "ruleDeleteDescription");
}
