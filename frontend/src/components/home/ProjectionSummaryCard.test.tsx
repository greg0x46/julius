import { render, screen } from "@testing-library/react";
import dayjs from "dayjs";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { TimelineDayPoint, TimelineSeries } from "../../api/contracts";
import * as homePeriodBounds from "../../hooks/useHomePeriodBounds";
import * as timelineHook from "../../hooks/useTimeline";
import { ProjectionSummaryCard } from "./ProjectionSummaryCard";

vi.mock("../../hooks/useHomePeriodBounds");
vi.mock("../../hooks/useTimeline");
vi.mock("../../hooks/useToday", () => ({ useToday: () => dayjs("2026-10-22") }));
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
