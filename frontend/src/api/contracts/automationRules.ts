import { isCount, isRecord, isUuid, isValidDate, requiredRecord } from "./shared";

// RuleCondition is the shared field/operator/value shape both automation
// rules and recurring commitments compose — see internal/rules (backend)
// and docs/development/architecture.md (motor de regras). "amount"/
// "day_of_month" only make sense where there's an occurrence to compare
// against — on an AutomationRule that means a "reconcile" action is present
// (see ruleConditionFieldOperators and AutomationAction below); the backend
// rejects them otherwise.
export const ruleConditionFields = ["description", "card", "account", "amount", "day_of_month"] as const;

export type RuleConditionField = (typeof ruleConditionFields)[number];

export const ruleConditionOperators = ["contains", "equals", "within_percent", "day_range"] as const;

export type RuleConditionOperator = (typeof ruleConditionOperators)[number];

export const ruleLogicOperators = ["and", "or"] as const;

export type RuleLogicOperator = (typeof ruleLogicOperators)[number];

export interface RuleCondition {
  field: RuleConditionField;
  operator: RuleConditionOperator;
  value: string;
}

// ruleConditionFieldOperators pins which operator(s) a field accepts —
// mirrors internal/automation/model.go's validateCondition. amount/
// day_of_month are only ever valid on a rule with a reconcile action (see
// parseRuleCondition's allowReconcileFields parameter).
const ruleConditionFieldOperators: Record<RuleConditionField, readonly RuleConditionOperator[]> = {
  description: ["contains", "equals"],
  card: ["contains", "equals"],
  account: ["contains", "equals"],
  amount: ["within_percent"],
  day_of_month: ["day_range"],
};

export function parseRuleCondition(value: unknown, allowReconcileFields: boolean): RuleCondition {
  const condition = requiredRecord(value, ["field", "operator", "value"], "Condição inválida.");
  const field = condition.field as RuleConditionField;
  const operator = condition.operator as RuleConditionOperator;
  const isReconcileField = field === "amount" || field === "day_of_month";
  if (
    !ruleConditionFields.includes(field) ||
    !ruleConditionOperators.includes(operator) ||
    !ruleConditionFieldOperators[field].includes(operator) ||
    (isReconcileField && !allowReconcileFields) ||
    typeof condition.value !== "string" ||
    condition.value === ""
  ) {
    throw new TypeError("Condição inválida.");
  }
  return { field, operator, value: condition.value };
}

export type AutomationLogicOperator = RuleLogicOperator;

export const automationLogicOperators = ruleLogicOperators;

export const automationActionTypes = ["ignore", "reconcile", "set_category"] as const;

export type AutomationActionType = (typeof automationActionTypes)[number];

export interface AutomationAction {
  type: AutomationActionType;
  /** The recurring Scenario a reconcile action targets; null otherwise. */
  scenario_id: string | null;
  category_id?: string | null;
}

export interface AutomationRule {
  id: string;
  name: string;
  is_active: boolean;
  logic_operator: AutomationLogicOperator;
  conditions: RuleCondition[];
  actions: AutomationAction[];
  created_at: string;
  updated_at: string;
}

export interface AutomationRuleWrite {
  name: string;
  is_active: boolean;
  logic_operator: AutomationLogicOperator;
  conditions: RuleCondition[];
  actions: AutomationAction[];
  apply_retroactively: boolean;
}

export interface RetroactiveApplyResult {
  matched: number;
  ignored: number;
  categorized: number;
}

export interface AutomationRuleWriteResult {
  rule: AutomationRule;
  retroactive_apply: RetroactiveApplyResult | null;
}

export interface AutomationRuleConditionOptions {
  accounts: string[];
  cards: string[];
}

export function parseAutomationRuleConditionOptions(
  value: unknown,
): AutomationRuleConditionOptions {
  const options = requiredRecord(
    value,
    ["accounts", "cards"],
    "Opções de condições inválidas.",
  );
  if (
    !Array.isArray(options.accounts) ||
    !options.accounts.every((item) => typeof item === "string" && item !== "") ||
    !Array.isArray(options.cards) ||
    !options.cards.every((item) => typeof item === "string" && item !== "")
  ) {
    throw new TypeError("Opções de condições inválidas.");
  }
  return {
    accounts: options.accounts as string[],
    cards: options.cards as string[],
  };
}

function parseAutomationAction(value: unknown): AutomationAction {
  const record = isRecord(value) ? value : null;
  const hasCategoryID = record !== null && "category_id" in record;
  const action = requiredRecord(
    value,
    ["type", "scenario_id", ...(hasCategoryID ? ["category_id"] : [])],
    "Ação inválida.",
  );
  const type = action.type as AutomationActionType;
  const scenarioID = action.scenario_id;
  const categoryID = action.category_id;
  if (
    !automationActionTypes.includes(type) ||
    !(scenarioID === null || (typeof scenarioID === "string" && isUuid(scenarioID))) ||
    !(categoryID === undefined || categoryID === null || (typeof categoryID === "string" && isUuid(categoryID))) ||
    (type === "ignore" && (scenarioID !== null || (categoryID !== null && categoryID !== undefined))) ||
    (type === "reconcile" && (scenarioID === null || (categoryID !== null && categoryID !== undefined))) ||
    (type === "set_category" && (categoryID == null || scenarioID !== null))
  ) {
    throw new TypeError("Ação inválida.");
  }
  const result: AutomationAction = { type, scenario_id: scenarioID as string | null };
  if (hasCategoryID) result.category_id = categoryID as string | null;
  return result;
}

export function parseAutomationRule(value: unknown): AutomationRule {
  const rule = requiredRecord(
    value,
    ["id", "name", "is_active", "logic_operator", "conditions", "actions", "created_at", "updated_at"],
    "Regra de automação inválida.",
  );
  if (
    typeof rule.id !== "string" ||
    !isUuid(rule.id) ||
    typeof rule.name !== "string" ||
    rule.name === "" ||
    typeof rule.is_active !== "boolean" ||
    !automationLogicOperators.includes(rule.logic_operator as AutomationLogicOperator) ||
    !Array.isArray(rule.conditions) ||
    rule.conditions.length === 0 ||
    !Array.isArray(rule.actions) ||
    rule.actions.length === 0 ||
    !isValidDate(rule.created_at) ||
    !isValidDate(rule.updated_at)
  ) {
    throw new TypeError("Regra de automação inválida.");
  }
  const actions = rule.actions.map(parseAutomationAction);
  const hasReconcileAction = actions.some((action) => action.type === "reconcile");
  return {
    id: rule.id,
    name: rule.name,
    is_active: rule.is_active,
    logic_operator: rule.logic_operator as AutomationLogicOperator,
    conditions: rule.conditions.map((c) => parseRuleCondition(c, hasReconcileAction)),
    actions,
    created_at: rule.created_at,
    updated_at: rule.updated_at,
  };
}

export function parseAutomationRuleList(value: unknown): AutomationRule[] {
  if (!Array.isArray(value)) {
    throw new TypeError("Lista de regras de automação inválida.");
  }
  return value.map(parseAutomationRule);
}

function parseRetroactiveApplyResult(value: unknown): RetroactiveApplyResult {
  const hasCategorized = isRecord(value) && "categorized" in value;
  const result = requiredRecord(
    value,
    hasCategorized ? ["matched", "ignored", "categorized"] : ["matched", "ignored"],
    "Resultado retroativo inválido.",
  );
  if (!isCount(result.matched) || !isCount(result.ignored) || (hasCategorized && !isCount(result.categorized))) {
    throw new TypeError("Resultado retroativo inválido.");
  }
  const parsed = { matched: result.matched, ignored: result.ignored } as RetroactiveApplyResult;
  if (hasCategorized) parsed.categorized = result.categorized as number;
  return parsed;
}

export function parseAutomationRuleWriteResult(value: unknown): AutomationRuleWriteResult {
  const result = requiredRecord(
    value,
    ["rule", "retroactive_apply"],
    "Resposta de regra de automação inválida.",
  );
  return {
    rule: parseAutomationRule(result.rule),
    retroactive_apply:
      result.retroactive_apply === null ? null : parseRetroactiveApplyResult(result.retroactive_apply),
  };
}
