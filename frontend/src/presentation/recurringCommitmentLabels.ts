import { intlLocale, longMonthName } from "../i18n/dateFormats";
import { translate, type Language } from "../i18n/messages";
import type {
  RecurringCommitment,
  RecurringCommitmentCadence,
  RecurringCommitmentKind,
} from "../api/contracts";

export function recurringCommitmentKindName(kind: RecurringCommitmentKind, language: Language = "pt-BR"): string {
  return translate(language, kind === "income" ? "recurringKindIncome" : "recurringKindExpense");
}

export function recurringCommitmentCadenceName(cadence: RecurringCommitmentCadence, language: Language = "pt-BR"): string {
  return translate(language, cadence === "monthly" ? "cadenceMonthly" : "cadenceAnnual");
}

export function monthOfYearLabel(monthOfYear: number, language: Language = "pt-BR"): string {
  const name = longMonthName(language, monthOfYear - 1);
  return name.charAt(0).toLocaleUpperCase(intlLocale(language)) + name.slice(1);
}

export function recurrenceScheduleLabel(
  cadence: RecurringCommitmentCadence,
  dayOfMonth: number,
  monthOfYear: number | null,
  language: Language = "pt-BR",
): string {
  if (cadence === "annual" && monthOfYear !== null) {
    return translate(language, "scheduleAnnualLabel", { day: dayOfMonth, month: monthOfYearLabel(monthOfYear, language) });
  }
  return translate(language, "scheduleMonthlyLabel", { day: dayOfMonth });
}

const pad = (value: number) => String(value).padStart(2, "0");

function isoDay(year: number, month: number, day: number): string {
  return `${year}-${pad(month)}-${pad(day)}`;
}

/** The calendar day of today in the reader's zone, as "YYYY-MM-DD". */
export function localToday(now: Date = new Date()): string {
  return isoDay(now.getFullYear(), now.getMonth() + 1, now.getDate());
}

/**
 * The next date the schedule expects, from `today` on ("YYYY-MM-DD"), or null
 * once it has ended. Mirrors the backend's calendar (internal/recurrences/
 * occurrence.go): day 31 lands on the last day of a shorter month, an annual
 * schedule only counts its month, nothing before the start or after the end.
 */
export function nextOccurrenceDate(
  commitment: Pick<
    RecurringCommitment,
    "cadence" | "day_of_month" | "month_of_year" | "start_date" | "end_date"
  >,
  today: string,
): string | null {
  const from = commitment.start_date > today ? commitment.start_date : today;
  let year = Number(from.slice(0, 4));
  let month = Number(from.slice(5, 7));
  // An annual schedule repeats within twelve months; two years is a safe bound for both cadences.
  for (let step = 0; step < 24; step += 1) {
    const annualMismatch = commitment.cadence === "annual" && month !== commitment.month_of_year;
    if (!annualMismatch) {
      const lastDay = new Date(year, month, 0).getDate();
      const date = isoDay(year, month, Math.min(commitment.day_of_month, lastDay));
      if (commitment.end_date !== null && date > commitment.end_date) return null;
      if (date >= from) return date;
    }
    month += 1;
    if (month > 12) {
      month = 1;
      year += 1;
    }
  }
  return null;
}

/** "10/10", with the year only when it is not the current one. */
export function formatNextDay(value: string, today: string): string {
  const [year, month, day] = value.split("-");
  return year === today.slice(0, 4) ? `${day}/${month}` : `${day}/${month}/${year}`;
}

/** Where a recurrence stands, when that is not the default of being active and running. */
export function recurrenceStanding(
  commitment: Pick<RecurringCommitment, "is_active" | "end_date">,
  today: string,
): "paused" | "ended" | null {
  if (!commitment.is_active) return "paused";
  if (commitment.end_date !== null && commitment.end_date < today) return "ended";
  return null;
}

/** "todo dia 10" / "dia 10 de março": the schedule as it reads inside a sentence. */
export function recurrenceScheduleSentence(
  cadence: RecurringCommitmentCadence,
  dayOfMonth: number,
  monthOfYear: number | null,
  language: Language = "pt-BR",
): string {
  if (cadence === "annual" && monthOfYear !== null) {
    return translate(language, "scheduleAnnualSentence", { day: dayOfMonth, month: monthOfYearLabel(monthOfYear, language).toLowerCase() });
  }
  return translate(language, "scheduleMonthlySentence", { day: dayOfMonth });
}
