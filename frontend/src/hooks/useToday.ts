import { calendar, type CalendarDate } from "../presentation/calendar";
import { msUntilNextDay, zonedToday } from "../presentation/timezone";
import { useEffect, useMemo, useState } from "react";
import { useTimezone } from "./useTimezone";

// The dashboard is left open for hours, so "today" cannot be whatever it was
// when the tab was first painted: at the configured zone's midnight the
// balance anchor, and with it the whole window, must move to the new day.
export function useToday(): CalendarDate {
  const timezone = useTimezone();
  const [day, setDay] = useState(() => zonedToday(timezone));
  useEffect(() => {
    let timer: number;
    const refresh = () => {
      setDay(zonedToday(timezone));
      timer = window.setTimeout(refresh, msUntilNextDay(timezone) + 1_000);
    };
    // The zone may have changed since `day` was read.
    setDay(zonedToday(timezone));
    timer = window.setTimeout(refresh, msUntilNextDay(timezone) + 1_000);
    return () => window.clearTimeout(timer);
  }, [timezone]);
  return useMemo(() => calendar(day), [day]);
}
