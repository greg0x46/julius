/** Local calendar operations; date-only input never passes through UTC parsing. */
export class CalendarDate {
  private readonly value: Date;
  constructor(input: string | Date | CalendarDate = new Date()) {
    if (input instanceof CalendarDate) this.value = input.toDate();
    else if (typeof input === "string" && /^\d{4}-\d{2}-\d{2}$/.test(input)) {
      const [year, month, day] = input.split("-").map(Number);
      this.value = new Date(0);
      this.value.setFullYear(year, month - 1, day);
      this.value.setHours(0, 0, 0, 0);
      if (
        this.value.getFullYear() !== year ||
        this.value.getMonth() !== month - 1 ||
        this.value.getDate() !== day
      )
        this.value.setTime(NaN);
    } else
      this.value = new Date(input instanceof Date ? input.getTime() : input);
  }
  toDate() {
    return new Date(this.value);
  }
  year() {
    return this.value.getFullYear();
  }
  date() {
    return this.value.getDate();
  }
  isValid() {
    return Number.isFinite(this.value.getTime());
  }
  format(template: string) {
    const parts: Record<string, string> = {
      YYYY: String(this.year()).padStart(4, "0"),
      MM: String(this.value.getMonth() + 1).padStart(2, "0"),
      DD: String(this.date()).padStart(2, "0"),
    };
    return this.isValid()
      ? template.replace(/YYYY|MM|DD/g, (key) => parts[key])
      : "Invalid Date";
  }
  add(count: number, unit: "day") {
    void unit;
    const next = this.toDate();
    next.setDate(next.getDate() + count);
    return new CalendarDate(next);
  }
  startOf(unit: "day") {
    void unit;
    const next = this.toDate();
    next.setHours(0, 0, 0, 0);
    return new CalendarDate(next);
  }
  endOf(unit: "month") {
    void unit;
    const next = this.toDate();
    next.setMonth(next.getMonth() + 1, 0);
    next.setHours(23, 59, 59, 999);
    return new CalendarDate(next);
  }
  diff(other: CalendarDate, unit?: "day") {
    if (!unit) return this.value.getTime() - other.value.getTime();
    const ordinal = (date: Date) => {
      const result = new Date(0);
      result.setUTCFullYear(
        date.getFullYear(),
        date.getMonth(),
        date.getDate(),
      );
      result.setUTCHours(0, 0, 0, 0);
      return result.getTime();
    };
    return Math.trunc(
      (ordinal(this.value) - ordinal(other.value)) / 86_400_000,
    );
  }
  isBefore(other: CalendarDate, unit: "day") {
    return this.diff(other, unit) < 0;
  }
}
export function calendar(input?: string | Date | CalendarDate) {
  return new CalendarDate(input);
}
