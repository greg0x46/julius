import type { GoalMetric } from "../api/contracts";
import { compareDecimals, multiplyDecimals } from "./decimal";

export const goalMetricLabel: Record<GoalMetric, string> = {
  invested_value: "Valor investido (valor de mercado)",
};

/** The exact ratio ("0.4521") as a whole percent; it passes 100 once the goal is reached. */
export function goalPercent(progress: string): number {
  const percent = Math.max(0, Math.round(Number(multiplyDecimals(progress, "100"))));
  return compareDecimals(progress, "1") < 0 ? Math.min(99, percent) : percent;
}
