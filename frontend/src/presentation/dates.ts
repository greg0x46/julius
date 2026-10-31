import { dateConventions, dateFormatter, numericDate, numericDayMonth, shortMonthName } from "../i18n/dateFormats";
import { translate, type Language } from "../i18n/messages";

export function formatDate(value: string, language: Language = "pt-BR"): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? translate(language, "invalidDate") : dateFormatter(language, { dateStyle: "short", timeStyle: "medium" }).format(date);
}

export function formatOptionalDate(value: string | null, language: Language = "pt-BR"): string {
  return value === null ? translate(language, "notYetAvailable") : formatDate(value, language);
}

/** "22/09/2026 às 06:00" — the same instant as formatDate, without seconds
 *  and read as a sentence, for a place like "Atualizado em ..." rather than
 *  a data table. */
export function formatOptionalDateTime(value: string | null, language: Language = "pt-BR"): string {
  if (value === null) return translate(language, "notYetAvailable");
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return translate(language, "invalidDate");
  return translate(language, "dateAtTime", {
    date: dateFormatter(language, { dateStyle: "short" }).format(date),
    time: dateFormatter(language, { timeStyle: "short" }).format(date),
  });
}

// Bill and card-limit dates are calendar days anchored at midnight UTC, so
// rendering them with formatDate would tack on a meaningless "00:00:00" — and
// reading them in the local timezone would slide them a day backwards.
export function formatDay(value: string, language: Language = "pt-BR"): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? translate(language, "invalidDate") : dateFormatter(language, { dateStyle: "short", timeZone: "UTC" }).format(date);
}

export function formatOptionalDay(value: string | null, language: Language = "pt-BR"): string {
  return value === null ? "—" : formatDay(value, language);
}

// An instant — a transaction's occurred_at, a card's last use — is a point in
// time, not a calendar day, and belongs in the reader's own timezone: that's
// how the transactions screen renders it, and the two screens show the same
// rows. Pinning it to UTC like formatDay would push anything after 21:00 in
// Brazil onto the next day, so the same purchase would carry two dates.
export function formatLocalDay(value: string, language: Language = "pt-BR"): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? translate(language, "invalidDate") : dateFormatter(language, { dateStyle: "short" }).format(date);
}

export function formatOptionalLocalDay(value: string | null, language: Language = "pt-BR"): string {
  return value === null ? "—" : formatLocalDay(value, language);
}

// A plain "YYYY-MM-DD" from the timeline API is a calendar day with no
// instant behind it, so it needs neither Date parsing nor a timezone —
// reordering the parts is both correct and cheaper than the alternatives.
export function formatDateOnly(value: string, language: Language = "pt-BR"): string {
  const [year, month, day] = value.split("-");
  return year && month && day ? numericDate(language, { year, month, day }) : value;
}

/** A plain "YYYY-MM-DD" as "DD/MM", for places where the year is already obvious. */
export function formatDayMonth(value: string, language: Language = "pt-BR"): string {
  const [, month, day] = value.split("-");
  return month && day ? numericDayMonth(language, { month, day }) : value;
}

const dateOnlyPattern = /^\d{4}-\d{2}-\d{2}$/;

/**
 * The calendar parts of either kind of date the API sends: a plain
 * "YYYY-MM-DD" is taken as written, an instant is read in the local zone
 * (see formatLocalDay). Null when the value can't be parsed.
 */
export function calendarParts(value: string): { day: number; month: number; year: number } | null {
  if (dateOnlyPattern.test(value)) {
    const [year, month, day] = value.split("-").map(Number);
    return { day, month, year };
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return null;
  return { day: date.getDate(), month: date.getMonth() + 1, year: date.getFullYear() };
}

/**
 * "13 set." — the shortest date that still tells rows apart in a dense list;
 * the year is appended only when it isn't the current one.
 */
export function formatCompactDay(value: string, language: Language = "pt-BR"): string {
  const parts = calendarParts(value);
  if (parts === null) return translate(language, "invalidDate");
  const currentYear = parts.year === new Date().getFullYear();
  if (!dateFormatsHaveShortMonths(language)) {
    const date = new Date(Date.UTC(parts.year, parts.month - 1, parts.day));
    return dateFormatter(language, { day: "numeric", month: "short", ...(currentYear ? {} : { year: "numeric" }), timeZone: "UTC" }).format(date);
  }
  const label = `${String(parts.day).padStart(2, "0")} ${shortMonthName(language, parts.month - 1)}.`;
  return currentYear ? label : `${label} ${parts.year}`;
}

const dateFormatsHaveShortMonths = (language: Language) => dateConventions[language].shortMonths !== undefined;
