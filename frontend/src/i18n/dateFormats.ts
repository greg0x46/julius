import type { Language } from "./messages";

/** Per-locale date conventions: the one place that decides how a date reads. */
type DateConvention = {
  /** BCP 47 tag handed to `Intl`. */
  locale: string;
  /** Order of the numeric parts in a short date. */
  order: "dmy" | "mdy";
  /** Hand-written short month names, for locales where `Intl` is too long for dense lists. */
  shortMonths?: readonly string[];
};

export const dateConventions: Record<Language, DateConvention> = {
  "pt-BR": {
    locale: "pt-BR",
    order: "dmy",
    shortMonths: ["jan", "fev", "mar", "abr", "mai", "jun", "jul", "ago", "set", "out", "nov", "dez"],
  },
  en: { locale: "en", order: "mdy" },
};

export const intlLocale = (language: Language): string => dateConventions[language].locale;

const cache = new Map<string, Intl.DateTimeFormat>();

/** A cached `Intl.DateTimeFormat` for the language's locale. */
export function dateFormatter(language: Language, options: Intl.DateTimeFormatOptions): Intl.DateTimeFormat {
  const key = `${language}|${JSON.stringify(options)}`;
  let formatter = cache.get(key);
  if (!formatter) {
    formatter = new Intl.DateTimeFormat(intlLocale(language), options);
    cache.set(key, formatter);
  }
  return formatter;
}

type Parts = { year: number | string; month: number | string; day: number | string };
const pad = (value: number | string) => String(value).padStart(2, "0");

/** Numeric day with year in the locale's order: 31/10/2026 or 10/31/2026. */
export function numericDate(language: Language, { year, month, day }: Parts): string {
  const ordered = dateConventions[language].order === "dmy" ? [pad(day), pad(month), year] : [pad(month), pad(day), year];
  return ordered.join("/");
}

/** Numeric day and month without the year: 31/10 or 10/31. */
export function numericDayMonth(language: Language, { month, day }: Omit<Parts, "year">): string {
  return (dateConventions[language].order === "dmy" ? [pad(day), pad(month)] : [pad(month), pad(day)]).join("/");
}

/** Short month name ("set", "Sep") for a 0-based month. */
export function shortMonthName(language: Language, month: number): string {
  const names = dateConventions[language].shortMonths;
  return names ? names[month] : dateFormatter(language, { month: "short", timeZone: "UTC" }).format(new Date(Date.UTC(2000, month, 1)));
}

/** Full month name ("janeiro", "January") for a 0-based month. */
export function longMonthName(language: Language, month: number): string {
  return dateFormatter(language, { month: "long", timeZone: "UTC" }).format(new Date(Date.UTC(2000, month, 1)));
}

/** Hour and minute of an instant in the reader's zone. */
export const formatTime = (language: Language, date: Date): string =>
  dateFormatter(language, { hour: "2-digit", minute: "2-digit" }).format(date);

/** "outubro de 2026" / "October 2026" for a UTC calendar day. */
export const formatMonthYear = (language: Language, date: Date): string =>
  dateFormatter(language, { month: "long", year: "numeric", timeZone: "UTC" }).format(date);
