import type { GoalMetric } from "../api/contracts";
import { multiplyDecimals } from "./decimal";

export const goalMetricLabel: Record<GoalMetric, string> = {
  invested_value: "Valor investido (valor de mercado)",
};

/** The exact ratio ("0.4521") as a whole percent; it passes 100 once the goal is reached. */
export function goalPercent(progress: string): number {
  return Math.max(0, Math.round(Number(multiplyDecimals(progress, "100"))));
}
