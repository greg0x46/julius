import { describe, expect, it } from "vitest";
import { formatDateOnly, formatCompactDay, formatDayMonth } from "../presentation/dates";
import { longMonthName, numericDate, numericDayMonth, shortMonthName } from "./dateFormats";

describe("date formats by language", () => {
  it("orders numeric dates by locale", () => {
    expect(numericDate("pt-BR", { year: 2026, month: 10, day: 31 })).toBe("31/10/2026");
    expect(numericDate("en", { year: 2026, month: 10, day: 31 })).toBe("10/31/2026");
    expect(numericDayMonth("en", { month: "09", day: "13" })).toBe("09/13");
  });
  it("applies them to calendar days", () => {
    expect(formatDateOnly("2026-09-13")).toBe("13/09/2026");
    expect(formatDateOnly("2026-09-13", "en")).toBe("09/13/2026");
    expect(formatDayMonth("2026-09-13", "pt-BR")).toBe("13/09");
    expect(formatCompactDay("2020-09-13")).toBe("13 set. 2020");
    expect(formatCompactDay("2020-09-13", "en")).toContain("Sep");
  });
  it("names months", () => {
    expect(shortMonthName("pt-BR", 8)).toBe("set");
    expect(longMonthName("en", 0)).toBe("January");
    expect(longMonthName("pt-BR", 2)).toBe("março");
  });
});
