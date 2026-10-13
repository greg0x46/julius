import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import * as homePeriodBounds from "../../hooks/useHomePeriodBounds";
import * as timelineHook from "../../hooks/useTimeline";
import { PeriodBalanceCard } from "./PeriodBalanceCard";

vi.mock("../../hooks/useHomePeriodBounds");
vi.mock("../../hooks/useTimeline");

const period = { from: "2026-10-01", to: "2026-10-31" };

function mockTotals(totals: { income: string; expense: string; result: string }, currencyCode = "BRL") {
  vi.mocked(homePeriodBounds.useHomePeriodBounds).mockReturnValue({
    bounds: period,
    wholePeriod: false,
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  } as unknown as ReturnType<typeof homePeriodBounds.useHomePeriodBounds>);
  vi.mocked(timelineHook.useTimeline).mockReturnValue({
    valuation: { currency_code: currencyCode },
 currencyCode,
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
  it("labels converted totals and accessible links in the response currency", () => {
    mockTotals({ income: "200", expense: "100", result: "100" }, "USD");
    renderCard();
    expect(screen.getByRole("link", { name: /Entradas: USD\s200,00/ })).toHaveTextContent("+USD 200,00");
    expect(screen.getByText("+USD 100,00")).toBeVisible();
    expect(screen.queryByText(/R\$/)).toBeNull();
  });

});
