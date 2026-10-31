import { translate, type Language } from "../../i18n/messages";
import type { DateRangePreset } from "./PeriodNavigator";

function dateText(date: Date): string {
  return [
    String(date.getFullYear()).padStart(4, "0"),
    String(date.getMonth() + 1).padStart(2, "0"),
    String(date.getDate()).padStart(2, "0"),
  ].join("-");
}

function monthRange(offset: number): [string, string] {
  const today = new Date();
  const start = new Date(today.getFullYear(), today.getMonth() + offset, 1);
  const end = new Date(today.getFullYear(), today.getMonth() + offset + 1, 0);
  return [dateText(start), dateText(end)];
}

/**
 * The shortcuts every period selector offers — the transactions filter bar and
 * the Home balance widget alike, so the same window means the same thing
 * wherever it is picked. Each range is resolved when it is read, never at
 * module load: a tab left open past midnight must still answer with today's
 * month.
 *
 * "Todo o período" is the pair of nulls: transactions read it as "no date
 * filter" and the balance widget resolves it against the database's own span.
 */
export function periodPresets(language: Language = "pt-BR"): DateRangePreset[] {
  return [
    { value: "this-month", label: translate(language, "thisMonth"), range: () => monthRange(0) },
    { value: "last-month", label: translate(language, "lastMonth"), range: () => monthRange(-1) },
    {
      value: "last-30-days",
      label: translate(language, "last30Days"),
      range: () => {
        const end = new Date();
        const start = new Date(end.getFullYear(), end.getMonth(), end.getDate() - 29);
        return [dateText(start), dateText(end)];
      },
    },
    {
      value: "this-year",
      label: translate(language, "thisYear"),
      range: () => {
        const today = new Date();
        return [
          dateText(new Date(today.getFullYear(), 0, 1)),
          dateText(new Date(today.getFullYear(), 11, 31)),
        ];
      },
    },
    { value: "all", label: translate(language, "allTime"), range: () => [null, null] },
  ];
}
