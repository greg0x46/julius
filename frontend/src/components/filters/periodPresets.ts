import { monthRange, shiftDay, zonedToday } from "../../presentation/timezone";
import { translate, type Language } from "../../i18n/messages";
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
export function periodPresets(timezone: string, language: Language = "pt-BR"): DateRangePreset[] {
  return [
    { value: "this-month", label: translate(language, "thisMonth"), range: () => monthRange(zonedToday(timezone), 0) },
    { value: "last-month", label: translate(language, "lastMonth"), range: () => monthRange(zonedToday(timezone), -1) },
    {
      value: "last-30-days",
      label: translate(language, "last30Days"),
      range: () => {
        const end = zonedToday(timezone);
        return [shiftDay(end, -29), end];
      },
    },
    {
      value: "this-year",
      label: translate(language, "thisYear"),
      range: () => {
        const year = zonedToday(timezone).slice(0, 4);
        return [`${year}-01-01`, `${year}-12-31`];
      },
    },
    { value: "all", label: translate(language, "allTime"), range: () => [null, null] },
  ];
}
