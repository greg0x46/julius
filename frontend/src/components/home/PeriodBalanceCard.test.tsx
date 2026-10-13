import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import * as homePeriodBounds from "../../hooks/useHomePeriodBounds";
import * as timelineHook from "../../hooks/useTimeline";
import { ApiError } from "../../api/problems";
import { PeriodBalanceCard } from "./PeriodBalanceCard";

vi.mock("../../hooks/useHomePeriodBounds");
vi.mock("../../hooks/useTimeline");

const period = { from: "2026-10-01", to: "2026-10-31" };

function mockTotals(totals: { income: string; expense: string; result: string }) {
  vi.mocked(homePeriodBounds.useHomePeriodBounds).mockReturnValue({
    bounds: period,
    wholePeriod: false,
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  } as unknown as ReturnType<typeof homePeriodBounds.useHomePeriodBounds>);
  vi.mocked(timelineHook.useTimeline).mockReturnValue({
    base: null,
    periodTotals: totals,
    simulation: null,
    scenarioImpacts: [],
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  } as unknown as ReturnType<typeof timelineHook.useTimeline>);
}

function renderCard() {
  return render(
    <MemoryRouter>
      <PeriodBalanceCard period={period} />
    </MemoryRouter>,
  );
}

describe("PeriodBalanceCard", () => {
  beforeEach(() => vi.clearAllMocks());

  it("leads with the period result and says scenarios are included", () => {
    mockTotals({ income: "14160.00", expense: "16961.07", result: "-2801.07" });
    renderCard();

    expect(screen.getByText("Resultado do período")).toBeVisible();
    expect(screen.getByText("-R$ 2.801,07")).toBeVisible();
    // The caveat matches the backend: active scenarios are part of the totals.
    expect(screen.getByText(/incluindo cenários ativos/)).toBeVisible();
    expect(screen.queryByText(/sem cenários hipotéticos/)).toBeNull();
  });

  it("keeps entradas and saídas as links to the matching transactions", () => {
    mockTotals({ income: "14160.00", expense: "16961.07", result: "-2801.07" });
    renderCard();

    const inflow = screen.getByRole("link", { name: /Entradas: R\$\s14\.160,00/ });
    const outflow = screen.getByRole("link", { name: /Saídas: R\$\s16\.961,07/ });
    expect(inflow).toHaveAttribute("href", expect.stringContaining("classification=inflow"));
    expect(outflow).toHaveAttribute("href", expect.stringContaining("classification=outflow"));
    expect(inflow).toHaveTextContent("+R$ 14.160,00");
    // An ordinary outflow carries its minus but is not painted as an error.
    expect(outflow).toHaveTextContent("-R$ 16.961,07");
  });

  it("does not sign a period with no movement", () => {
    mockTotals({ income: "0.00", expense: "0.00", result: "0.00" });
    renderCard();

    expect(screen.getByText("Nenhuma movimentação no período")).toBeVisible();
    expect(screen.queryByText("+R$ 0,00")).toBeNull();
    expect(screen.queryByText("-R$ 0,00")).toBeNull();
  });
});

it("states missing historical rates without showing a zero period total", () => {
  mockTotals({ income: "0.00", expense: "0.00", result: "0.00" });
  vi.mocked(timelineHook.useTimeline).mockReturnValue({ base: null, periodTotals: null, simulation: null, scenarioImpacts: [], isLoading: false, refetch: vi.fn(), error: new ApiError("response", "Faltam cotações históricas para o relatório.", { type: "/problems/reporting-fx-unavailable", title: "Total indisponível", status: 503 }) });
  renderCard();
  expect(screen.getByText("Faltam cotações históricas para o relatório.")).toBeVisible();
  expect(screen.queryByText("R$ 0,00")).not.toBeInTheDocument();
});
