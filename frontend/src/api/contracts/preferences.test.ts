import { describe, expect, it } from "vitest";

import { defaultTimezone, parsePreferences } from "./preferences";

const base = { transactions_period_basis: "occurred_at", reference_currency: "BRL" };

describe("parsePreferences timezone", () => {
  it("reads the configured zone", () => {
    expect(parsePreferences({ ...base, timezone: "Asia/Tokyo" }).timezone).toBe("Asia/Tokyo");
    expect(parsePreferences({ ...base, timezone: "America/Argentina/Buenos_Aires" }).timezone).toBe(
      "America/Argentina/Buenos_Aires",
    );
    expect(parsePreferences({ ...base, timezone: "UTC" }).timezone).toBe("UTC");
  });

  it("defaults to the server's default when it is omitted or null, like the currency", () => {
    expect(parsePreferences({ ...base, timezone: null }).timezone).toBe(defaultTimezone);
    expect(parsePreferences(base).timezone).toBe(defaultTimezone);
  });

  it.each([5, "", "not a zone", "../etc/passwd", "A".repeat(65)])("rejects %j", (timezone) => {
    expect(() => parsePreferences({ ...base, timezone })).toThrow("Preferências inválidas.");
  });
});
