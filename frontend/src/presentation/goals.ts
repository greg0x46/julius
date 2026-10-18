import type { Goal, GoalMetric } from "../api/contracts";
import { compareDecimals, multiplyDecimals } from "./decimal";

export const goalMetricLabel: Record<GoalMetric, string> = {
  invested_value: "Valor investido (valor de mercado)",
};

/** The exact ratio ("0.4521") as a whole percent; it passes 100 once the goal is reached. */
export function goalPercent(progress: string): number {
  const percent = Math.max(0, Math.round(Number(multiplyDecimals(progress, "100"))));
  return compareDecimals(progress, "1") < 0 ? Math.min(99, percent) : percent;
}

export function goalMonthsLabel(months: number): string {
  return months === 1 ? "1 mês" : `${months} meses`;
}

/** Open goals first, then reached ones; each group from the smallest target to the largest. */
export function sortGoals(goals: Goal[]): Goal[] {
  const reached = (goal: Goal) => compareDecimals(goal.progress, "1") >= 0;
  return [...goals].sort(
    (a, b) =>
      Number(reached(a)) - Number(reached(b)) ||
      compareDecimals(a.target_amount, b.target_amount) ||
      a.name.localeCompare(b.name, "pt-BR"),
  );
}
