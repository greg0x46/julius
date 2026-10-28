import { render, screen } from "@testing-library/react";
import { calendar } from "../../presentation/calendar";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { TimelineDayPoint, TimelineSeries } from "../../api/contracts";
import * as homePeriodBounds from "../../hooks/useHomePeriodBounds";
import * as timelineHook from "../../hooks/useTimeline";
import { ProjectionSummaryCard } from "./ProjectionSummaryCard";

vi.mock("../../hooks/useHomePeriodBounds");
vi.mock("../../hooks/useTimeline");
vi.mock("../../hooks/useToday", () => ({ useToday: () => calendar("2026-10-22") }));
vi.mock("../timeline/ProjectionTimeline", () => ({ ProjectionTimeline: () => <div data-testid="projection-chart" /> }));

const period = { from: "2026-09-01", to: "2026-09-30" };

function renderCard(balances: string[], startingBalance = balances[0]) {
  const points: TimelineDayPoint[] = balances.map((balance, index) => ({
    date: `2026-09-${String(index + 1).padStart(2, "0")}`,
    balance,
    inflow: "0.00",
    outflow: "0.00",
    lowest_tier: "realizado",
  }));
  const base: TimelineSeries = {
    points, entries: [], starting_balance: startingBalance,
    lowest_balance: points[0], first_negative: null,
  };
  vi.mocked(homePeriodBounds.useHomePeriodBounds).mockReturnValue({
    bounds: period, wholePeriod: false, isLoading: false, error: null, refetch: vi.fn(),
  });
  vi.mocked(timelineHook.useTimeline).mockReturnValue({
    valuation: { currency_code: "BRL" }, currencyCode: "BRL", base,
    periodTotals: null, simulation: null, scenarioImpacts: [],
    isLoading: false, error: null, refetch: vi.fn(),
  });
  return render(<MemoryRouter><ProjectionSummaryCard period={period} /></MemoryRouter>);
}

function renderFuture(balances: string[], firstNegative: string | null = null, lowestIndex = 0) {
  const points: TimelineDayPoint[] = balances.map((balance, index) => ({
    date: `2026-10-${String(22 + index).padStart(2, "0")}`,
    balance,
    inflow: "0.00",
    outflow: "0.00",
    lowest_tier: "projetado",
  }));
  const base: TimelineSeries = {
    points, entries: [], starting_balance: balances[0],
    lowest_balance: points[lowestIndex], first_negative: firstNegative,
  };
  const future = { from: "2026-10-01", to: points[points.length - 1].date };
  vi.mocked(homePeriodBounds.useHomePeriodBounds).mockReturnValue({
    bounds: future, wholePeriod: false, isLoading: false, error: null, refetch: vi.fn(),
  });
  vi.mocked(timelineHook.useTimeline).mockReturnValue({
    valuation: { currency_code: "BRL" }, currencyCode: "BRL", base,
    periodTotals: null, simulation: null, scenarioImpacts: [],
    isLoading: false, error: null, refetch: vi.fn(),
  });
  return render(<MemoryRouter><ProjectionSummaryCard period={future} /></MemoryRouter>);
}

describe("ProjectionSummaryCard figures shown once", () => {
  beforeEach(() => vi.clearAllMocks());

  const monthDays = (tail: string[]) => [...tail, ...Array(10 - tail.length).fill(tail[tail.length - 1])];

  it("drops a lowest balance that is the month-end balance already in the top band", () => {
    renderFuture(monthDays(["1000.00", "900.00", "800.00"]), null, 9);

    expect(screen.queryByText("Menor saldo previsto")).toBeNull();
    expect(screen.queryByText("Saldo no fim do período")).toBeNull();
  });

  it("keeps a lowest balance that is not repeated elsewhere", () => {
    renderFuture(monthDays(["1000.00", "300.00", "800.00"]).map((v, i) => (i >= 2 ? "800.00" : v)), null, 1);

    expect(screen.getByText("Menor saldo previsto").parentElement).toHaveTextContent("R$ 300,00");
  });

  it("leaves the lowest point to the alert when the forecast goes negative", () => {
    renderFuture(monthDays(["1000.00", "-300.00", "200.00"]), "2026-10-23", 1);

    expect(screen.queryByText("Menor saldo previsto")).toBeNull();
  });
});

describe("ProjectionSummaryCard monetary comparisons", () => {
  beforeEach(() => vi.clearAllMocks());

  it("shows the exact historical minimum and its date even when Number would erase the difference", () => {
    renderCard(["1000000000000000.01", "1000000000000000.00"]);

    const minimum = screen.getByText("Menor saldo no período").parentElement;
    expect(minimum).toHaveTextContent("R$ 1.000.000.000.000.000,00");
    expect(minimum).toHaveTextContent("02/09/2026");
  });

  it("keeps the first date when exact balances tie despite different scales", () => {
    renderCard(["1.000", "1.00"]);

    expect(screen.getByText("Menor saldo no período").parentElement).toHaveTextContent("01/09/2026");
  });

  it("does not hide a nonzero balance when numeric conversion would underflow", () => {
    renderCard([`0.${"0".repeat(324)}1`], "0");

    expect(screen.getByTestId("projection-chart")).toBeVisible();
    expect(screen.queryByText("Ainda não há saldo para mostrar")).toBeNull();
  });
});
