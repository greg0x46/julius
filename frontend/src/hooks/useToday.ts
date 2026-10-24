import { calendar, type CalendarDate } from "../presentation/calendar";
import { useEffect, useState } from "react";

// The dashboard is left open for hours, so "today" cannot be whatever it was
// when the tab was first painted: at midnight the balance anchor, and with it
// the whole window, must move to the new day.
export function useToday(): CalendarDate {
  const [today, setToday] = useState(() => calendar());
  useEffect(() => {
    const msToMidnight = today.add(1, "day").startOf("day").diff(calendar());
    const timer = window.setTimeout(() => setToday(calendar()), Math.max(msToMidnight, 0) + 1_000);
    return () => window.clearTimeout(timer);
  }, [today]);
  return today;
}
