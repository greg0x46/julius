import type { TimelineDayPoint } from "../api/contracts";
import { clampBRLAtZero, divideBRLFloor, minBRL } from "./money";

/**
 * The most that can be spent every day, from the first point on, without the
 * projected balance dipping below zero. Spending d a day leaves B_k - k*d on
 * day k, so the answer is the minimum over k of B_k / k, floored to the cent:
 * a low point early in the window weighs far more than the same low point at
 * the end. A day whose balance is negative leaves nothing to spend.
 *
 * `points` is one point per day, the first being today and the last the end of
 * the month. Spending is assumed to land at the end of each day.
 */
export function dailyAllowance(points: Pick<TimelineDayPoint, "balance">[]): string {
  if (points.length === 0) return "0.00";
  return minBRL(points.map((point, index) => divideBRLFloor(clampBRLAtZero(point.balance), index + 1)));
}
