import { describe, expect, it } from "vitest";

import type { InvestmentPeriodPosition } from "../api/contracts";
import { periodTesouro } from "../test/investmentWorkspaceFixtures";
import {
  formatRatioPercent,
  matchesAssetSearch,
  partialReasons,
  periodRequest,
  ratioDirection,
} from "./investmentPeriod";

const periodPosition = (overrides: Partial<InvestmentPeriodPosition>): InvestmentPeriodPosition => ({
  ...periodTesouro,
  ...overrides,
});

describe("formatRatioPercent", () => {
  it.each([
    ["0.0123456789", "1,23%"],
    ["0.2", "20,00%"],
    ["1", "100,00%"],
    ["12.3456", "1.234,56%"],
    ["0", "0,00%"],
    ["0.00004", "0,00%"],
    ["-0.05", "-5,00%"],
    ["-0.00004", "0,00%"],
  ])("writes %s as %s", (ratio, text) => expect(formatRatioPercent(ratio)).toBe(text));

  it("signs a positive result only when asked", () => {
    expect(formatRatioPercent("0.05", { signed: true })).toBe("+5,00%");
    expect(formatRatioPercent("-0.05", { signed: true })).toBe("-5,00%");
    expect(formatRatioPercent("0", { signed: true })).toBe("0,00%");
  });

  it("does not lose precision on long ratios", () => {
    expect(formatRatioPercent("0.1234999999999999999")).toBe("12,35%");
    expect(formatRatioPercent("0.12344999999999999999")).toBe("12,34%");
  });
});

describe("ratioDirection", () => {
  it("reads a ratio that rounds to nothing as zero", () => {
    expect(ratioDirection("0.00001")).toBe("zero");
    expect(ratioDirection("0.01")).toBe("positive");
    expect(ratioDirection("-0.01")).toBe("negative");
  });
});

describe("periodRequest", () => {
  const today = "2026-10-17";

  it("omits from for the whole period and ends it today", () => {
    expect(periodRequest({ from: null, to: null }, today)).toEqual({ from: null, to: today });
  });

  it("keeps a period that already ended", () => {
    expect(periodRequest({ from: "2026-09-01", to: "2026-09-30" }, today)).toEqual({
      from: "2026-09-01",
      to: "2026-09-30",
    });
  });

  it("sends a running period as is: the server cuts the end at today", () => {
    expect(periodRequest({ from: "2026-10-01", to: "2026-10-31" }, today)).toEqual({
      from: "2026-10-01",
      to: "2026-10-31",
    });
  });

  it("sends nothing for a period that starts after today", () => {
    expect(periodRequest({ from: "2026-11-01", to: "2026-11-30" }, today)).toBeNull();
  });

  it("keeps a period that starts today", () => {
    expect(periodRequest({ from: today, to: "2026-10-31" }, today)).toEqual({ from: today, to: "2026-10-31" });
  });
});

describe("matchesAssetSearch", () => {
  const position = { name: "Tesouro Selic 2029", ticker: "TD-SELIC" };

  it.each([
    ["", true],
    ["  ", true],
    ["tesouro", true],
    ["TESOURO selic", true],
    ["td-selic", true],
    ["selic 2029", true],
    ["cdb", false],
  ])("search %j matches: %s", (search, expected) => {
    expect(matchesAssetSearch(position, search)).toBe(expected);
  });

  it("ignores accents on both sides", () => {
    expect(matchesAssetSearch({ name: "Fundo Imobiliário", ticker: null }, "imobiliario")).toBe(true);
    expect(matchesAssetSearch({ name: "Fundo Imobiliario", ticker: null }, "imobiliário")).toBe(true);
  });
});

describe("partialReasons", () => {
  it("counts unavailable positions by reason", () => {
    const positions = [
      periodPosition({ unavailable_reason: "sem_historico" }),
      periodPosition({ unavailable_reason: "sem_historico" }),
      periodPosition({ unavailable_reason: "cambio_indisponivel" }),
      periodPosition({}),
    ];
    expect(partialReasons(positions)).toEqual(["2× Sem histórico", "1× Sem câmbio"]);
  });
});
