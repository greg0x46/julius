import { useMemo } from "react";

import { useTimeline } from "./useTimeline";
import { useToday } from "./useToday";

const dateFormat = "YYYY-MM-DD";

/**
 * The rest of this month, from today: one point per day, today first, so a
 * point's position is the number of days it has to cover. Independent of the
 * period Home shows. React Query keys by value, so every card that reads this
 * window shares one request.
 */
export function useMonthTimeline() {
  const today = useToday();
  const referenceDate = today.format(dateFormat);
  const monthEnd = today.endOf("month").format(dateFormat);
  const params = useMemo(
    () => ({ referenceDate, from: referenceDate, to: monthEnd }),
    [referenceDate, monthEnd],
  );
  return { ...useTimeline(params), referenceDate, monthEnd };
}
