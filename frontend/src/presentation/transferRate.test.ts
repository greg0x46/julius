import { describe, expect, it } from "vitest";
import { transferRate } from "./transferRate";

describe("transferRate", () => {
  it("uses actual native amounts and explicit direction units", () => {
    expect(transferRate("-540", "100")).toBe("5.4");
    expect(transferRate("10.0000", "10")).toBe("1");
    expect(transferRate("1", "3")).toBe("0.3333333333333333");
    expect(transferRate("2", "3")).toBe("0.6666666666666667");
    expect(transferRate("9007199254740993.01", "1")).toBe("9007199254740993.01");
    expect(transferRate("10", "0")).toBe("");
  });
});
