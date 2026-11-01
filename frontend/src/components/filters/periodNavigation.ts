import { dateFormatter, formatMonthYear } from "../../i18n/dateFormats";
import { translate, type Language } from "../../i18n/messages";
const DAY = 86_400_000;

function parseDate(value: unknown): Date | null {
  if (typeof value !== "string" || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return null;
  const date = new Date(`${value}T00:00:00Z`);
  return Number.isFinite(date.getTime()) && date.toISOString().slice(0, 10) === value
    ? date : null;
}

export function periodNavigation(from: unknown, to: unknown, language: Language = "pt-BR") {
  const start = parseDate(from);
  const end = parseDate(to);
  if (!start || !end || start > end) return null;
  const afterEnd = new Date(end.getTime() + DAY);
  const wholeMonths = start.getUTCDate() === 1 && afterEnd.getUTCDate() === 1;
  const months = (afterEnd.getUTCFullYear() - start.getUTCFullYear()) * 12
    + afterEnd.getUTCMonth() - start.getUTCMonth();
  const isYear = wholeMonths && months === 12 && start.getUTCMonth() === 0;
  const unit = isYear ? "Year" : wholeMonths && months === 1 ? "Month" : "Period";
  const format = (date: Date) => dateFormatter(language, { timeZone: "UTC" }).format(date);
  const label = isYear ? String(start.getUTCFullYear())
    : wholeMonths && months === 1
      ? formatMonthYear(language, start)
      : `${format(start)} – ${format(end)}`;

  const shift = (direction: -1 | 1): [string, string] | null => {
    let nextStart: Date;
    let nextEnd: Date;
    if (wholeMonths) {
      nextStart = new Date(start);
      nextStart.setUTCMonth(start.getUTCMonth() + months * direction);
      nextEnd = new Date(nextStart);
      nextEnd.setUTCMonth(nextStart.getUTCMonth() + months);
      nextEnd.setUTCDate(0);
    } else {
      const offset = (end.getTime() - start.getTime() + DAY) * direction;
      nextStart = new Date(start.getTime() + offset);
      nextEnd = new Date(end.getTime() + offset);
    }
    if (nextStart.getUTCFullYear() < 1 || nextEnd.getUTCFullYear() > 9999) return null;
    return [nextStart.toISOString().slice(0, 10), nextEnd.toISOString().slice(0, 10)];
  };
  return { label, previousLabel: translate(language, `previous${unit}` as "previousYear" | "previousMonth" | "previousPeriod"),
    nextLabel: translate(language, `next${unit}` as "nextYear" | "nextMonth" | "nextPeriod"), previous: shift(-1), next: shift(1) };
}
