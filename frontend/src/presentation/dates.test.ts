import { describe, expect, it } from "vitest";

import {
  formatDate,
  formatDateOnly,
  formatDay,
  formatDayMonth,
  formatLocalDay,
  formatOptionalDate,
  formatOptionalDateTime,
  formatOptionalDay,
  formatOptionalLocalDay,
} from "./dates";

describe("date presentation", () => {
  it("formats valid dates in pt-BR local time", () => {
    expect(formatDate("2026-07-29T12:00:00Z")).toMatch(/\d{2}\/\d{2}\/2026/);
  });

  it("handles invalid and absent dates explicitly", () => {
    expect(formatDate("invalid")).toBe("Data inválida");
    expect(formatOptionalDate(null)).toBe("Ainda não disponível");
    expect(formatDay("invalid")).toBe("Data inválida");
    expect(formatLocalDay("invalid")).toBe("Data inválida");
    expect(formatOptionalDay(null)).toBe("—");
    expect(formatOptionalLocalDay(null)).toBe("—");
  });

  // A calendar day carries no time of day, so it has to read the same wherever
  // the reader is — including just past midnight UTC, where a local reading
  // would slide it a day backwards.
  it("pins a calendar day to UTC", () => {
    expect(formatDay("2026-04-03T00:00:00Z")).toBe("03/04/2026");
    expect(formatDay("2026-04-02T00:30:00Z")).toBe("02/04/2026");
  });

  // An instant is a point in time and has to read as the day it happened
  // where the reader is: 00:30Z is still the 1st in Brazil. Comparing against
  // the platform's own local formatting is what pins the *absence* of a
  // timeZone override — the regression this guards is someone hard-coding UTC
  // here, which shows up on any runner that isn't itself on UTC.
  it("reads an instant in the reader's own timezone", () => {
    const instant = "2026-04-02T00:30:00Z";
    expect(formatLocalDay(instant)).toBe(new Date(instant).toLocaleDateString("pt-BR"));
  });

  it("formats calendar dates and missing values in English", () => {
    expect(formatDay("2026-04-03T00:00:00Z", "en")).toBe("4/3/26");
    expect(formatDateOnly("2026-04-03", "en")).toBe("04/03/2026");
    expect(formatDayMonth("2026-04-03", "en")).toBe("04/03");
    expect(formatOptionalDateTime(null, "en")).toBe("Not yet available");
    expect(formatDay("invalid", "en")).toBe("Invalid date");
  });

  it("formats an updated-at instant as a sentence, without seconds", () => {
    expect(formatOptionalDateTime("2026-09-22T09:00:00Z")).toMatch(
      /^\d{2}\/\d{2}\/2026 às \d{2}:\d{2}$/,
    );
    expect(formatOptionalDateTime(null)).toBe("Ainda não disponível");
    expect(formatOptionalDateTime("invalid")).toBe("Data inválida");
  });
});

it("formats a calendar day as day and month only", () => {
  expect(formatDayMonth("2026-10-31")).toBe("31/10");
  expect(formatDayMonth("not a date")).toBe("not a date");
});
