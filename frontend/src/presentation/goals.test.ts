import { describe, expect, it } from "vitest";

import { goalPercent } from "./goals";

describe("goalPercent", () => {
  it.each([
    ["0", 0],
    ["0.452105", 45],
    ["0.455", 46],
    ["0.995", 99],
    ["0.9999999999999999", 99],
    ["1", 100],
    ["1.2345", 123],
  ])("reads the ratio %s as %i%%", (progress, percent) => {
    expect(goalPercent(progress)).toBe(percent);
  });

  it("never reports a negative percent", () => {
    expect(goalPercent("-0.2")).toBe(0);
  });
});
