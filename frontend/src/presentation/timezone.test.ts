import { describe, expect, it } from "vitest";

import {
  browserTimezone,
  calendarIn,
  isSupportedTimezone,
  monthRange,
  msUntilNextDay,
  resolveTimezone,
  shiftDay,
  timezoneChoices,
  zonedDay,
  zonedToday,
} from "./timezone";

const day = (value: { format: (template: string) => string }) => value.format("YYYY-MM-DD");

describe("calendar day in a zone", () => {
  // One instant, three different calendar days.
  const instant = new Date("2026-04-02T00:30:00Z");

  it("reads an instant in the given zone, not the machine's", () => {
    expect(zonedDay(instant, "America/Sao_Paulo")).toBe("2026-04-01");
    expect(zonedDay(instant, "UTC")).toBe("2026-04-02");
    expect(zonedDay(instant, "Asia/Tokyo")).toBe("2026-04-02");
  });

  it("accepts a string or a number and refuses what is not a date", () => {
    expect(zonedDay("2026-04-02T00:30:00Z", "America/Sao_Paulo")).toBe("2026-04-01");
    expect(zonedDay(instant.getTime(), "America/Sao_Paulo")).toBe("2026-04-01");
    expect(zonedDay("not a date", "UTC")).toBeNull();
  });

  it("knows today in the zone", () => {
    const now = new Date("2026-10-31T15:30:00Z");
    expect(zonedToday("America/Sao_Paulo", now)).toBe("2026-10-31");
    expect(zonedToday("Asia/Tokyo", now)).toBe("2026-11-01");
  });

  it("takes a plain day as written and converts an instant", () => {
    expect(day(calendarIn("2026-04-02", "America/Sao_Paulo"))).toBe("2026-04-02");
    expect(day(calendarIn("2026-04-02T00:30:00Z", "America/Sao_Paulo"))).toBe("2026-04-01");
  });
});

describe("day arithmetic", () => {
  it("gives the first and last day of a month, leap years included", () => {
    expect(monthRange("2026-02-15")).toEqual(["2026-02-01", "2026-02-28"]);
    expect(monthRange("2028-02-10")).toEqual(["2028-02-01", "2028-02-29"]);
    expect(monthRange("2026-01-10", -1)).toEqual(["2025-12-01", "2025-12-31"]);
    expect(monthRange("2026-12-05", 1)).toEqual(["2027-01-01", "2027-01-31"]);
  });

  it("shifts a day across month and year ends", () => {
    expect(shiftDay("2026-03-01", -1)).toBe("2026-02-28");
    expect(shiftDay("2026-12-31", 1)).toBe("2027-01-01");
    expect(shiftDay("2026-05-10", 0)).toBe("2026-05-10");
  });
});

describe("msUntilNextDay", () => {
  it("is exact to the millisecond", () => {
    expect(msUntilNextDay("America/Sao_Paulo", new Date("2026-10-23T02:59:30Z"))).toBe(30_000);
    expect(msUntilNextDay("Asia/Tokyo", new Date("2026-10-22T14:59:59.500Z"))).toBe(500);
    expect(msUntilNextDay("UTC", new Date("2026-10-22T00:00:00Z"))).toBe(24 * 3_600_000);
  });

  // A day is not always 24 hours: New York skipped 02:00-03:00 on 2026-03-08.
  it("follows a daylight-saving change instead of assuming 24-hour days", () => {
    expect(msUntilNextDay("America/New_York", new Date("2026-03-08T04:00:00Z"))).toBe(3_600_000);
    expect(msUntilNextDay("America/New_York", new Date("2026-03-08T05:00:00Z"))).toBe(23 * 3_600_000);
  });
});

describe("choosing a zone", () => {
  it("recognizes the names this browser knows", () => {
    expect(isSupportedTimezone("Asia/Tokyo")).toBe(true);
    expect(isSupportedTimezone("Nowhere/Land")).toBe(false);
  });

  it("uses the configured zone, falling back to the browser's only when it is missing or unknown", () => {
    expect(resolveTimezone("Asia/Tokyo")).toBe("Asia/Tokyo");
    expect(resolveTimezone(null)).toBe(browserTimezone());
    expect(resolveTimezone(undefined)).toBe(browserTimezone());
    expect(resolveTimezone("Nowhere/Land")).toBe(browserTimezone());
  });

  it("always offers UTC, the default and the current zone, sorted", () => {
    const choices = timezoneChoices("Pacific/Kiritimati");
    expect(choices).toEqual(expect.arrayContaining(["UTC", "America/Sao_Paulo", "Pacific/Kiritimati"]));
    expect(choices).toEqual([...choices].sort((a, b) => a.localeCompare(b, "en")));
    expect(new Set(choices).size).toBe(choices.length);
  });
});
