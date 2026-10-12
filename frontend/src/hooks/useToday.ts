import dayjs from "dayjs";
import { useEffect, useState } from "react";

// The dashboard is left open for hours, so "today" cannot be whatever it was
// when the tab was first painted: at midnight the balance anchor, and with it
// the whole window, must move to the new day.
export function useToday(): dayjs.Dayjs {
  const [today, setToday] = useState(() => dayjs());
  useEffect(() => {
    const msToMidnight = today.add(1, "day").startOf("day").diff(dayjs());
    const timer = window.setTimeout(() => setToday(dayjs()), Math.max(msToMidnight, 0) + 1_000);
    return () => window.clearTimeout(timer);
  }, [today]);
  return today;
}
