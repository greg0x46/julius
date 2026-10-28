import type { TimelineDayPoint } from "../api/contracts";
import { clampBRLAtZero, divideBRLFloor, minBRL } from "./money";

/**
 * The most that can be spent every day, from the first point on, without the
 * projected balance dipping below zero. Spending d a day leaves B_k - k*d on
 * day k, so the answer is the minimum over k of B_k / k, floored to the cent:
 * a low point early in the window weighs far more than the same low point at
 * the end. A day whose balance is negative leaves nothing to spend.
 *
 * Money that has not arrived is not spendable: each day's balance is capped at
 * `startingBalance` (cash now), i.e. `startingBalance + min(0, net)` where net
 * is the cumulative future inflows minus outflows. A future inflow therefore
 * only offsets the outflows already due; any surplus never becomes allowance.
 *
 * `points` is one point per day, the first being today and the last the end of
 * the month. Spending is assumed to land at the end of each day.
 */
export function dailyAllowance(points: Pick<TimelineDayPoint, "balance">[], startingBalance: string): string {
  if (points.length === 0) return "0.00";
  return minBRL(
    points.map((point, index) =>
      divideBRLFloor(clampBRLAtZero(minBRL([point.balance, startingBalance])), index + 1),
    ),
  );
}
