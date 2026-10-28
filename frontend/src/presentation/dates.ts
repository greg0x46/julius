const formatter = new Intl.DateTimeFormat("pt-BR", {
  dateStyle: "short",
  timeStyle: "medium",
});

export function formatDate(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "Data inválida" : formatter.format(date);
}

export function formatOptionalDate(value: string | null): string {
  return value === null ? "Ainda não disponível" : formatDate(value);
}

const shortDateFormatter = new Intl.DateTimeFormat("pt-BR", { dateStyle: "short" });
const shortTimeFormatter = new Intl.DateTimeFormat("pt-BR", { timeStyle: "short" });

/** "22/09/2026 às 06:00" — the same instant as formatDate, without seconds
 *  and read as a sentence, for a place like "Atualizado em ..." rather than
 *  a data table. */
export function formatOptionalDateTime(value: string | null): string {
  if (value === null) return "Ainda não disponível";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "Data inválida";
  return `${shortDateFormatter.format(date)} às ${shortTimeFormatter.format(date)}`;
}

// Bill and card-limit dates are calendar days anchored at midnight UTC, so
// rendering them with formatDate would tack on a meaningless "00:00:00" — and
// reading them in the local timezone would slide them a day backwards.
const dayFormatter = new Intl.DateTimeFormat("pt-BR", { dateStyle: "short", timeZone: "UTC" });

export function formatDay(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "Data inválida" : dayFormatter.format(date);
}

export function formatOptionalDay(value: string | null): string {
  return value === null ? "—" : formatDay(value);
}

// An instant — a transaction's occurred_at, a card's last use — is a point in
// time, not a calendar day, and belongs in the reader's own timezone: that's
// how the transactions screen renders it, and the two screens show the same
// rows. Pinning it to UTC like formatDay would push anything after 21:00 in
// Brazil onto the next day, so the same purchase would carry two dates.
const localDayFormatter = new Intl.DateTimeFormat("pt-BR", { dateStyle: "short" });

export function formatLocalDay(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "Data inválida" : localDayFormatter.format(date);
}

export function formatOptionalLocalDay(value: string | null): string {
  return value === null ? "—" : formatLocalDay(value);
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

const shortMonths = ["jan", "fev", "mar", "abr", "mai", "jun", "jul", "ago", "set", "out", "nov", "dez"];

/**
 * "13 set." — the shortest date that still tells rows apart in a dense list;
 * the year is appended only when it isn't the current one.
 */
export function formatCompactDay(value: string): string {
  const parts = calendarParts(value);
  if (parts === null) return "Data inválida";
  const label = `${String(parts.day).padStart(2, "0")} ${shortMonths[parts.month - 1]}.`;
  return parts.year === new Date().getFullYear() ? label : `${label} ${parts.year}`;
}
