import {
  dateOnlyPattern,
  decimal,
  isNullableString,
  isUuid,
  isValidDate,
  requiredRecord,
} from "./shared";

export const goalMetrics = ["invested_value"] as const;

export type GoalMetric = (typeof goalMetrics)[number];

export interface Goal {
  id: string;
  name: string;
  metric: GoalMetric;
  target_amount: string;
  target_date: string | null;
  notes: string | null;
  current_value: string;
  /** Ratio of current_value to target_amount; above "1" once the goal is reached. */
  progress: string;
  currency_code: string;
  created_at: string;
  updated_at: string;
}

export interface GoalWrite {
  name: string;
  metric: GoalMetric;
  target_amount: string;
  target_date: string | null;
  notes: string | null;
}

const goalKeys = [
  "id",
  "name",
  "metric",
  "target_amount",
  "target_date",
  "notes",
  "current_value",
  "progress",
  "currency_code",
  "created_at",
  "updated_at",
] as const;

export function parseGoal(value: unknown): Goal {
  const item = requiredRecord(value, goalKeys, "Meta inválida.");
  if (
    typeof item.id !== "string" ||
    !isUuid(item.id) ||
    typeof item.name !== "string" ||
    item.name.trim() === "" ||
    !goalMetrics.includes(item.metric as GoalMetric) ||
    !(item.target_date === null || (typeof item.target_date === "string" && dateOnlyPattern.test(item.target_date))) ||
    !isNullableString(item.notes) ||
    typeof item.currency_code !== "string" ||
    item.currency_code.length !== 3 ||
    !isValidDate(item.created_at) ||
    !isValidDate(item.updated_at)
  ) {
    throw new TypeError("Meta inválida.");
  }
  return {
    id: item.id,
    name: item.name,
    metric: item.metric as GoalMetric,
    target_amount: decimal(item.target_amount),
    target_date: item.target_date as string | null,
    notes: item.notes as string | null,
    current_value: decimal(item.current_value),
    progress: decimal(item.progress),
    currency_code: item.currency_code,
    created_at: item.created_at,
    updated_at: item.updated_at,
  };
}

export function parseGoalList(value: unknown): Goal[] {
  const payload = requiredRecord(value, ["items"], "Lista de metas inválida.");
  if (!Array.isArray(payload.items)) throw new TypeError("Lista de metas inválida.");
  return payload.items.map(parseGoal);
}
