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

const zone = "America/Sao_Paulo";

describe("date presentation", () => {
  it("formats valid dates in pt-BR in the given timezone", () => {
    expect(formatDate("2026-07-29T12:00:00Z", zone)).toMatch(/\d{2}\/\d{2}\/2026/);
  });

  it("handles invalid and absent dates explicitly", () => {
    expect(formatDate("invalid", zone)).toBe("Data inválida");
    expect(formatOptionalDate(null, zone)).toBe("Ainda não disponível");
    expect(formatDay("invalid")).toBe("Data inválida");
    expect(formatLocalDay("invalid", zone)).toBe("Data inválida");
    expect(formatOptionalDay(null)).toBe("—");
    expect(formatOptionalLocalDay(null, zone)).toBe("—");
  });

  // A calendar day carries no time of day, so it has to read the same wherever
  // the reader is — including just past midnight UTC, where a local reading
  // would slide it a day backwards.
  it("pins a calendar day to UTC", () => {
    expect(formatDay("2026-04-03T00:00:00Z")).toBe("03/04/2026");
    expect(formatDay("2026-04-02T00:30:00Z")).toBe("02/04/2026");
  });

  // An instant is a point in time and has to read as the day it happened in the
  // configured zone, whatever zone the machine runs in: 00:30Z is still the 1st
  // in São Paulo and already the 2nd in Tokyo.
  it("reads an instant in the given timezone, not the machine's", () => {
    const instant = "2026-04-02T00:30:00Z";
    expect(formatLocalDay(instant, "America/Sao_Paulo")).toBe("01/04/2026");
    expect(formatLocalDay(instant, "Asia/Tokyo")).toBe("02/04/2026");
    expect(formatOptionalLocalDay(instant, "UTC")).toBe("02/04/2026");
  });

  it("formats calendar dates and missing values in English", () => {
    expect(formatDay("2026-04-03T00:00:00Z", "en")).toBe("4/3/26");
    expect(formatDateOnly("2026-04-03", "en")).toBe("04/03/2026");
    expect(formatDayMonth("2026-04-03", "en")).toBe("04/03");
    expect(formatOptionalDateTime(null, zone, "en")).toBe("Not yet available");
    expect(formatDay("invalid", "en")).toBe("Invalid date");
  });

  it("formats an updated-at instant as a sentence, without seconds", () => {
    expect(formatOptionalDateTime("2026-09-22T09:00:00Z", zone)).toMatch(
      /^\d{2}\/\d{2}\/2026 às \d{2}:\d{2}$/,
    );
    expect(formatOptionalDateTime(null, zone)).toBe("Ainda não disponível");
    expect(formatOptionalDateTime("invalid", zone)).toBe("Data inválida");
  });
});

it("formats a calendar day as day and month only", () => {
  expect(formatDayMonth("2026-10-31")).toBe("31/10");
  expect(formatDayMonth("not a date")).toBe("not a date");
});
