import { afterEach, describe, expect, it, vi } from "vitest";

import { investmentPeriodFixture, periodSynced, periodUnavailable } from "../test/investmentWorkspaceFixtures";
import { parseInvestmentPeriod } from "./contracts";
import { getInvestmentPeriod } from "./investmentPortfolio";
import { expireSession } from "./transport";

afterEach(() => {
  vi.unstubAllGlobals();
  expireSession();
});

// A mutable view of the JSON payload, so each case can break one field.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Draft = Record<string, any>;

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

describe("parseInvestmentPeriod", () => {
  it("keeps every field, null and decimal string the server sent", () => {
    for (const groupBy of ["account", "class", "none"] as const) {
      const payload = investmentPeriodFixture({ group_by: groupBy });
      expect(parseInvestmentPeriod(clone(payload))).toEqual(payload);
    }
  });

  it("keeps the held/closed parts, the position share and the clamp flag", () => {
    const payload = investmentPeriodFixture({ group_by: "none", to_clamped: true });
    const parsed = parseInvestmentPeriod(clone(payload));
    expect(parsed.to_clamped).toBe(true);
    expect(parsed.totals.held.yield).toBe("220.00");
    expect(parsed.totals.closed.yield_rate).toBe("0.04");
    expect(parsed.groups[0].positions.map((position) => position.share)).toEqual(["0.8", "0.2", "0", null]);
  });

  it("accepts a whole-period request echo and an origin", () => {
    const payload = investmentPeriodFixture({ from: null, source: "synced" });
    expect(parseInvestmentPeriod(clone(payload))).toMatchObject({ from: null, source: "synced" });
  });

  it("accepts the rates that took foreign positions to the reference currency", () => {
    const payload = investmentPeriodFixture({
      native_rates: { USD: { value: "5.4", inverted: false, effective_on: "2025-09-30", source: "synthetic", stale: false } },
    });
    expect(parseInvestmentPeriod(clone(payload)).native_rates.USD.value).toBe("5.4");
  });

  const mutations: [string, (payload: Draft) => void][] = [
    ["a missing held part", (p) => delete p.totals.held],
    ["a numeric closed amount", (p) => (p.totals.closed.yield = 20)],
    ["an unknown field in a part", (p) => (p.totals.held.extra = "1")],
    ["a group without a closed part", (p) => delete p.groups[0].totals.closed],
    ["a float position share", (p) => (p.groups[0].positions[0].share = 0.8)],
    ["a missing position share", (p) => delete p.groups[0].positions[0].share],
    ["a non-boolean to_clamped", (p) => (p.to_clamped = "no")],
    ["a numeric total", (p) => (p.totals.end_value = 1500)],
    ["a float ratio", (p) => (p.totals.yield_rate = 0.1)],
    ["a missing total", (p) => delete p.totals.costs],
    ["an unknown top-level field", (p) => (p.extra = true)],
    ["an unknown grouping", (p) => (p.group_by = "month")],
    ["an unknown origin", (p) => (p.source = "csv")],
    ["a malformed day", (p) => (p.to = "30/09/2025")],
    ["a negative count", (p) => (p.totals.positions_total = -1)],
    ["a non-boolean partial", (p) => (p.totals.partial = "yes")],
    ["groups that are not a list", (p) => (p.groups = {})],
    ["an unknown asset class", (p) => (p.groups[0].positions[0].asset_class = "stocks")],
    ["an unknown unavailable reason", (p) => (p.groups[1].positions[1].unavailable_reason = "quebrado")],
    ["a currency without a code", (p) => (p.currency_code = "real")],
    ["an invalid account id", (p) => (p.groups[0].account_id = "conta")],
    ["a position field typo", (p) => (p.groups[0].positions[0].yeild = "1")],
    ["a numeric position amount", (p) => (p.groups[0].positions[0].yield = 200)],
    [
      "an available position without figures",
      (p) => (p.groups[0].positions[0].end_value = null),
    ],
    [
      "an unavailable position that still has figures",
      (p) => (p.groups[1].positions[1].end_value = "0"),
    ],
    ["a zero rate", (p) => (p.native_rates = { USD: { value: "0", inverted: false, effective_on: "2025-09-30", source: "x", stale: false } })],
  ];

  it.each(mutations)("rejects %s", (_name, mutate) => {
    const payload = clone(investmentPeriodFixture({ group_by: "account" })) as unknown as Draft;
    mutate(payload);
    expect(() => parseInvestmentPeriod(payload)).toThrow(TypeError);
  });

  it("says why an unavailable position has no figures instead of zero", () => {
    const parsed = parseInvestmentPeriod(clone(investmentPeriodFixture({ group_by: "none" })));
    const unavailable = parsed.groups[0].positions.find((position) => position.id === periodUnavailable.id)!;
    expect(unavailable).toMatchObject({ unavailable_reason: "sem_historico", end_value: null, yield: null, native: null });
    const synced = parsed.groups[0].positions.find((position) => position.id === periodSynced.id)!;
    expect(synced).toMatchObject({ invested: null, invested_unavailable_reason: "historico_incompleto" });
  });
});

describe("getInvestmentPeriod", () => {
  function respond(body: unknown) {
    return vi.fn().mockResolvedValue(
      new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } }),
    );
  }

  it("sends the window, grouping and origin as query parameters", async () => {
    const fetchMock = respond(investmentPeriodFixture());
    vi.stubGlobal("fetch", fetchMock);
    await getInvestmentPeriod({ from: "2025-09-01", to: "2025-09-30", groupBy: "class", source: "manual" });
    const url = new URL(String(fetchMock.mock.calls[0][0]), "http://localhost");
    expect(url.pathname).toBe("/api/investment-portfolio");
    expect(Object.fromEntries(url.searchParams)).toEqual({
      from: "2025-09-01",
      to: "2025-09-30",
      group_by: "class",
      source: "manual",
    });
  });

  it("omits from for the whole period and source for every origin", async () => {
    const fetchMock = respond(investmentPeriodFixture({ from: null }));
    vi.stubGlobal("fetch", fetchMock);
    await getInvestmentPeriod({ from: null, to: "2025-09-30", groupBy: "none", source: null });
    const url = new URL(String(fetchMock.mock.calls[0][0]), "http://localhost");
    expect([...url.searchParams.keys()].sort()).toEqual(["group_by", "to"]);
  });

  it("rejects a response that breaks the contract", async () => {
    vi.stubGlobal("fetch", respond({ ...investmentPeriodFixture(), totals: {} }));
    await expect(
      getInvestmentPeriod({ from: null, to: "2025-09-30", groupBy: "none", source: null }),
    ).rejects.toMatchObject({ kind: "response" });
  });
});
