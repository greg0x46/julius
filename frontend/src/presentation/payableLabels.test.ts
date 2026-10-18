import { describe, expect, it } from "vitest";

import { settledPercent } from "./payableLabels";

describe("settledPercent", () => {
  it("floors exactly, with no float drift", () => {
    expect(settledPercent("100", "29")).toBe(29);
    expect(settledPercent("100", "57")).toBe(57);
    expect(settledPercent("100", "58")).toBe(58);
    expect(settledPercent("1000", "290")).toBe(29);
    expect(settledPercent("1000.00", "999.99")).toBe(99);
  });

  it("is 0 for a zero total and clamps to 0-100", () => {
    expect(settledPercent("0.00", "0.00")).toBe(0);
    expect(settledPercent("100", "-5")).toBe(0);
    expect(settledPercent("100", "150")).toBe(100);
  });
});
