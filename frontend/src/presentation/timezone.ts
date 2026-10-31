import { defaultTimezone } from "../api/contracts/preferences";
import { calendar, type CalendarDate } from "./calendar";

/**
 * The app's timezone: an IANA name from the owner's preferences. Every
 * "today", month boundary and day label is derived from it, never from the
 * machine the browser runs on.
 */

const formatters = new Map<string, Intl.DateTimeFormat>();

/** A cached pt-BR formatter pinned to `timezone`. Throws RangeError for an unknown zone. */
export function zonedFormatter(timezone: string, options: Intl.DateTimeFormatOptions, locale = "pt-BR") {
  const key = `${locale}|${timezone}|${JSON.stringify(options)}`;
  let formatter = formatters.get(key);
  if (!formatter) {
    formatter = new Intl.DateTimeFormat(locale, { ...options, timeZone: timezone });
    formatters.set(key, formatter);
  }
  return formatter;
}

export function isSupportedTimezone(timezone: string): boolean {
  try {
    zonedFormatter(timezone, {});
    return true;
  } catch {
    return false;
  }
}

/** The machine's own zone; a fallback for the moments the configured one is unknown. */
export function browserTimezone(): string {
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone?.trim();
  return zone && isSupportedTimezone(zone) ? zone : "UTC";
}

/** The zone to use for a configured name: the name itself when the browser knows it. */
export function resolveTimezone(configured: string | null | undefined): string {
  return configured && isSupportedTimezone(configured) ? configured : browserTimezone();
}

// For runtimes without Intl.supportedValuesOf.
const fallbackTimezones = [
  "America/Sao_Paulo",
  "America/Manaus",
  "America/Cuiaba",
  "America/Belem",
  "America/Fortaleza",
  "America/Recife",
  "America/Noronha",
  "America/Rio_Branco",
  "America/Argentina/Buenos_Aires",
  "America/Santiago",
  "America/Bogota",
  "America/Mexico_City",
  "America/New_York",
  "America/Chicago",
  "America/Denver",
  "America/Los_Angeles",
  "America/Toronto",
  "Europe/Lisbon",
  "Europe/London",
  "Europe/Paris",
  "Europe/Berlin",
  "Europe/Madrid",
  "Europe/Rome",
  "Europe/Moscow",
  "Africa/Johannesburg",
  "Asia/Dubai",
  "Asia/Kolkata",
  "Asia/Bangkok",
  "Asia/Shanghai",
  "Asia/Singapore",
  "Asia/Tokyo",
  "Australia/Sydney",
  "Pacific/Auckland",
  "UTC",
];

/** Every zone the person can pick, sorted, always including `current` and UTC. */
export function timezoneChoices(current?: string): string[] {
  let zones: string[];
  try {
    zones = Intl.supportedValuesOf("timeZone");
  } catch {
    zones = fallbackTimezones;
  }
  const all = new Set([...zones, "UTC", defaultTimezone]);
  if (current) all.add(current);
  return [...all].sort((a, b) => a.localeCompare(b, "en"));
}

/** The calendar day ("YYYY-MM-DD") an instant falls on in `timezone`; null if it cannot be parsed. */
export function zonedDay(instant: Date | string | number, timezone: string): string | null {
  const date = instant instanceof Date ? instant : new Date(instant);
  if (Number.isNaN(date.getTime())) return null;
  const parts = zonedFormatter(timezone, { year: "numeric", month: "2-digit", day: "2-digit" }, "en-US").formatToParts(date);
  const part = (type: string) => parts.find((item) => item.type === type)?.value ?? "";
  return `${part("year").padStart(4, "0")}-${part("month")}-${part("day")}`;
}

/** Today's calendar day in `timezone`, as "YYYY-MM-DD". */
export function zonedToday(timezone: string, now: Date = new Date()): string {
  return zonedDay(now, timezone) as string;
}

/** Today's calendar day in `timezone`. */
export function todayIn(timezone: string, now: Date = new Date()): CalendarDate {
  return calendar(zonedToday(timezone, now));
}

/** The calendar day of `input` in `timezone`: an instant is converted, a plain "YYYY-MM-DD" is taken as written. */
export function calendarIn(input: string, timezone: string): CalendarDate {
  return calendar(/^\d{4}-\d{2}-\d{2}$/.test(input) ? input : (zonedDay(input, timezone) ?? ""));
}

const HOUR = 3_600_000;

/**
 * Milliseconds until the next midnight in `timezone`, to the millisecond. Searches
 * for the first instant on another day rather than assuming 24-hour days, so
 * a daylight-saving change cannot make the answer an hour wrong.
 */
export function msUntilNextDay(timezone: string, now: Date = new Date()): number {
  const today = zonedDay(now, timezone);
  let low = 0;
  let high = 26 * HOUR;
  while (high - low > 1) {
    const middle = Math.floor((low + high) / 2);
    if (zonedDay(new Date(now.getTime() + middle), timezone) === today) low = middle;
    else high = middle;
  }
  return high;
}

const isoDay = (ms: number) => new Date(ms).toISOString().slice(0, 10);

/** First and last day of the month `offset` months from the one `today` ("YYYY-MM-DD") is in. */
export function monthRange(today: string, offset = 0): [string, string] {
  const [year, month] = today.split("-").map(Number);
  return [isoDay(Date.UTC(year, month - 1 + offset, 1)), isoDay(Date.UTC(year, month + offset, 0))];
}

/** The day `count` days from `day` ("YYYY-MM-DD"); negative counts go back. */
export function shiftDay(day: string, count: number): string {
  const [year, month, date] = day.split("-").map(Number);
  return isoDay(Date.UTC(year, month - 1, date + count));
}
