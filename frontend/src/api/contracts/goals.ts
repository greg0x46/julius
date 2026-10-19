import {
  dateOnlyPattern,
  decimal,
  isNullableString,
  isUuid,
  isValidDate,
  nullableDecimal,
  requiredRecord,
} from "./shared";

export const goalMetrics = ["invested_value"] as const;

export type GoalMetric = (typeof goalMetrics)[number];

export const goalProjectionStatuses = ["reached", "on_track", "behind"] as const;

export type GoalProjectionStatus = (typeof goalProjectionStatuses)[number];

/**
 * The goal at its target date, assuming no new contributions. Computed by the
 * backend from the investments' average monthly return; null when the goal has
 * no future date.
 */
export interface GoalProjection {
  status: GoalProjectionStatus;
  months_remaining: number;
  /** Compounded average monthly return (ratio, negative for a loss); null when there is too little history, and the projection then assumes no appreciation. */
  monthly_return: string | null;
  /** First day of the history the return was measured over. */
  return_from: string | null;
  /** Some position could not be measured and was left out of the return. */
  return_partial: boolean;
  projected_value: string;
  projected_progress: string;
  /** End-of-month deposit that reaches the target on the date; "0" when none is needed. */
  required_monthly_contribution: string;
}

export interface MonthlyContribution {
  month: string;
  target: string | null;
  net_contributed: string | null;
  remaining: string | null;
  assumes_zero_return: boolean;
}

export interface Goal {
  monthly_contribution: MonthlyContribution | null;
  id: string;
  name: string;
  metric: GoalMetric;
  target_amount: string;
  target_date: string | null;
  notes: string | null;
  current_value: string;
  /** Ratio of current_value to target_amount; above "1" once the goal is reached. */
  progress: string;
  projection: GoalProjection | null;
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
  "monthly_contribution",
  "id",
  "name",
  "metric",
  "target_amount",
  "target_date",
  "notes",
  "current_value",
  "progress",
  "projection",
  "currency_code",
  "created_at",
  "updated_at",
] as const;

const projectionKeys = [
  "status",
  "months_remaining",
  "monthly_return",
  "return_from",
  "return_partial",
  "projected_value",
  "projected_progress",
  "required_monthly_contribution",
] as const;

function parseProjection(value: unknown): GoalProjection | null {
  if (value === null) return null;
  const item = requiredRecord(value, projectionKeys, "Projeção inválida.");
  if (
    !goalProjectionStatuses.includes(item.status as GoalProjectionStatus) ||
    !Number.isInteger(item.months_remaining) ||
    (item.months_remaining as number) < 1 ||
    !(item.return_from === null || (typeof item.return_from === "string" && dateOnlyPattern.test(item.return_from))) ||
    typeof item.return_partial !== "boolean"
  ) {
    throw new TypeError("Projeção inválida.");
  }
  return {
    status: item.status as GoalProjectionStatus,
    months_remaining: item.months_remaining as number,
    monthly_return: nullableDecimal(item.monthly_return),
    return_from: item.return_from as string | null,
    return_partial: item.return_partial,
    projected_value: decimal(item.projected_value),
    projected_progress: decimal(item.projected_progress),
    required_monthly_contribution: decimal(item.required_monthly_contribution),
  };
}

function parseMonthlyContribution(value: unknown): MonthlyContribution | null {
  if (value === null) return null;
  const item = requiredRecord(value, ["month", "target", "net_contributed", "remaining", "assumes_zero_return"], "Aporte mensal inválido.");
  if (typeof item.month !== "string" || !/^\d{4}-(0[1-9]|1[0-2])$/.test(item.month) || typeof item.assumes_zero_return !== "boolean") {
    throw new TypeError("Aporte mensal inválido.");
  }
  return { month: item.month, target: nullableDecimal(item.target), net_contributed: nullableDecimal(item.net_contributed), remaining: nullableDecimal(item.remaining), assumes_zero_return: item.assumes_zero_return };
}

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
    monthly_contribution: parseMonthlyContribution(item.monthly_contribution),
    id: item.id,
    name: item.name,
    metric: item.metric as GoalMetric,
    target_amount: decimal(item.target_amount),
    target_date: item.target_date as string | null,
    notes: item.notes as string | null,
    current_value: decimal(item.current_value),
    progress: decimal(item.progress),
    projection: parseProjection(item.projection),
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
