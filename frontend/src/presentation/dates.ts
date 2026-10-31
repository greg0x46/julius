import { zonedDay, zonedFormatter, zonedToday } from "./timezone";

// An instant (a sync run, a token's creation) is read in the app's configured
// timezone, which every caller passes in (see useTimezone) — never in whatever
// zone the machine happens to be in.
export function formatDate(value: string, timezone: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? "Data inválida"
    : zonedFormatter(timezone, { dateStyle: "short", timeStyle: "medium" }).format(date);
}

export function formatOptionalDate(value: string | null, timezone: string): string {
  return value === null ? "Ainda não disponível" : formatDate(value, timezone);
}

/** "22/09/2026 às 06:00" — the same instant as formatDate, without seconds
 *  and read as a sentence, for a place like "Atualizado em ..." rather than
 *  a data table. */
export function formatOptionalDateTime(value: string | null, timezone: string): string {
  if (value === null) return "Ainda não disponível";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "Data inválida";
  const day = zonedFormatter(timezone, { dateStyle: "short" }).format(date);
  const time = zonedFormatter(timezone, { timeStyle: "short" }).format(date);
  return `${day} às ${time}`;
}

// Bill and card-limit dates are calendar days anchored at midnight UTC, so
// rendering them with formatDate would tack on a meaningless "00:00:00" — and
// reading them in any other timezone would slide them a day backwards.
const dayFormatter = new Intl.DateTimeFormat("pt-BR", { dateStyle: "short", timeZone: "UTC" });

export function formatDay(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "Data inválida" : dayFormatter.format(date);
}

export function formatOptionalDay(value: string | null): string {
  return value === null ? "—" : formatDay(value);
}

// An instant — a transaction's occurred_at, a card's last use — is a point in
// time, not a calendar day, and belongs in the app's configured timezone:
// that's how the transactions screen groups it, and the two screens show the
// same rows. Pinning it to UTC like formatDay would push anything after 21:00
// in Brazil onto the next day, so the same purchase would carry two dates.
export function formatLocalDay(value: string, timezone: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? "Data inválida"
    : zonedFormatter(timezone, { dateStyle: "short" }).format(date);
}

export function formatOptionalLocalDay(value: string | null, timezone: string): string {
  return value === null ? "—" : formatLocalDay(value, timezone);
}

// A plain "YYYY-MM-DD" from the timeline API is a calendar day with no
// instant behind it, so it needs neither Date parsing nor a timezone —
// reordering the parts is both correct and cheaper than the alternatives.
export function formatDateOnly(value: string): string {
  const [year, month, day] = value.split("-");
  return year && month && day ? `${day}/${month}/${year}` : value;
}

/** A plain "YYYY-MM-DD" as "DD/MM", for places where the year is already obvious. */
export function formatDayMonth(value: string): string {
  const [, month, day] = value.split("-");
  return month && day ? `${day}/${month}` : value;
}

const dateOnlyPattern = /^\d{4}-\d{2}-\d{2}$/;

/**
 * The calendar parts of either kind of date the API sends: a plain
 * "YYYY-MM-DD" is taken as written, an instant is read in `timezone`
 * (see formatLocalDay). Null when the value can't be parsed.
 */
export function calendarParts(
  value: string,
  timezone: string,
): { day: number; month: number; year: number } | null {
  const text = dateOnlyPattern.test(value) ? value : zonedDay(value, timezone);
  if (text === null) return null;
  const [year, month, day] = text.split("-").map(Number);
  return { day, month, year };
}

const shortMonths = ["jan", "fev", "mar", "abr", "mai", "jun", "jul", "ago", "set", "out", "nov", "dez"];

/**
 * "13 set." — the shortest date that still tells rows apart in a dense list;
 * the year is appended only when it isn't the current one in `timezone`.
 */
export function formatCompactDay(value: string, timezone: string, now: Date = new Date()): string {
  const parts = calendarParts(value, timezone);
  if (parts === null) return "Data inválida";
  const label = `${String(parts.day).padStart(2, "0")} ${shortMonths[parts.month - 1]}.`;
  return parts.year === Number(zonedToday(timezone, now).slice(0, 4)) ? label : `${label} ${parts.year}`;
}
