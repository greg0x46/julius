import { monthRange, shiftDay, zonedToday } from "../../presentation/timezone";
import type { DateRangePreset } from "./PeriodNavigator";

/**
 * The shortcuts every period selector offers — the transactions filter bar and
 * the Home balance widget alike, so the same window means the same thing
 * wherever it is picked. Each range is resolved when it is read, never at
 * module load: a tab left open past midnight must still answer with today's
 * month. "Today" is the day it is in `timezone`, not on the machine.
 *
 * "Todo o período" is the pair of nulls: transactions read it as "no date
 * filter" and the balance widget resolves it against the database's own span.
 */
export function periodPresets(timezone: string): DateRangePreset[] {
  return [
    { value: "this-month", label: "Este mês", range: () => monthRange(zonedToday(timezone), 0) },
    { value: "last-month", label: "Mês passado", range: () => monthRange(zonedToday(timezone), -1) },
    {
      value: "last-30-days",
      label: "Últimos 30 dias",
      range: () => {
        const end = zonedToday(timezone);
        return [shiftDay(end, -29), end];
      },
    },
    {
      value: "this-year",
      label: "Este ano",
      range: () => {
        const year = zonedToday(timezone).slice(0, 4);
        return [`${year}-01-01`, `${year}-12-31`];
      },
    },
    { value: "all", label: "Todo o período", range: () => [null, null] },
  ];
}
