import { translate, type Language } from "../i18n/messages";
import type { RuleCondition, RuleConditionField, RuleConditionOperator, RuleLogicOperator } from "../api/contracts";

export function ruleConditionFieldName(field: RuleConditionField, language: Language = "pt-BR"): string {
  const keys = { description: "description", card: "card", account: "account", amount: "amount", day_of_month: "ruleFieldDayOfMonth" } as const;
  return translate(language, keys[field]);
}

export function ruleConditionOperatorName(operator: RuleConditionOperator, language: Language = "pt-BR"): string {
  const keys = { contains: "ruleOpContains", equals: "ruleOpEquals", within_percent: "ruleOpWithinPercent", day_range: "ruleOpDayRange" } as const;
  return translate(language, keys[operator]);
}

export function ruleLogicOperatorName(operator: RuleLogicOperator, language: Language = "pt-BR"): string {
  return translate(language, operator === "and" ? "ruleLogicAnd" : "ruleLogicOr");
}

function formatConditionValue(condition: RuleCondition, language: Language): string {
  if (condition.operator === "day_range") {
    const [min, max] = condition.value.split(":");
    // A half-filled range reads as what is there, never "e undefined".
    if (!min || !max) return min || max || "—";
    return translate(language, "ruleValueAnd", { min, max });
  }
  if (condition.operator === "within_percent") {
    const [reference, tolerance] = condition.value.split(":");
    // No tolerance yet: just the reference, never "(±undefined%)".
    return tolerance ? `${reference} (±${tolerance}%)` : (reference ?? "");
  }
  return `"${condition.value}"`;
}

export function summarizeConditions(conditions: RuleCondition[], logicOperator: RuleLogicOperator, language: Language = "pt-BR"): string {
  return conditions
    .map(
      (condition) =>
        `${ruleConditionFieldName(condition.field, language)} ${ruleConditionOperatorName(condition.operator, language)} ${formatConditionValue(condition, language)}`,
    )
    .join(` ${ruleLogicOperatorName(logicOperator, language)} `);
}
