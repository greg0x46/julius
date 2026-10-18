import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { investmentPeriodFixture, periodTotals } from "../../test/investmentWorkspaceFixtures";
import { InvestmentPeriodSummary } from "./InvestmentPeriodSummary";

function renderSummary(overrides: Parameters<typeof investmentPeriodFixture>[0] = {}) {
  const data = investmentPeriodFixture({ group_by: "none", ...overrides });
  render(<InvestmentPeriodSummary data={data} positions={data.groups.flatMap((group) => group.positions)} updating={false} />);
  return screen.getByRole("region", { name: /Patrimônio em/ });
}

describe("InvestmentPeriodSummary", () => {
  it("shows costs only when there are any", () => {
    expect(within(renderSummary()).queryByText("Custos")).toBeNull();
  });

  it("shows costs, the origin and the unpaid provisioned taxes when they apply", () => {
    const strip = renderSummary({ totals: { ...periodTotals, costs: "12.50" }, source: "synced" });
    expect(within(strip).getByText("Custos")).toBeVisible();
    expect(within(strip).getByText("R$ 12,50")).toBeVisible();
    expect(within(strip).getByText(/Somente posições sincronizadas/)).toBeVisible();
    expect(within(strip).getByText(/antes do IR\/IOF provisionado/)).toBeVisible();
    expect(within(strip).getByText(/O saldo para investir \(caixa\)/)).toBeVisible();
  });

  it("does not claim a rentabilidade when the backend has no capital base", () => {
    const strip = renderSummary({ totals: { ...periodTotals, yield_rate: null } });
    expect(within(strip).getByRole("button", { name: /Rentabilidade indisponível/ })).toBeVisible();
    expect(within(strip).queryByText(/%/)).toBeNull();
  });

  it("states a negative result with its sign", () => {
    const strip = renderSummary({ totals: { ...periodTotals, yield: "-80.00", yield_rate: "-0.04" } });
    expect(within(strip).getByText("-R$ 80,00")).toBeVisible();
    expect(within(strip).getByText("-4,00%")).toBeVisible();
  });

  it("names the positions that start late and says nothing about partial when complete", () => {
    const strip = renderSummary({
      totals: { ...periodTotals, partial: false, positions_unavailable: 0, positions_partial_start: 2 },
    });
    expect(within(strip).getByText(/2 posições começam depois do primeiro dia do período/)).toBeVisible();
    expect(within(strip).queryByText(/Parcial/)).toBeNull();
  });

  it("explains the start for a whole-period request", async () => {
    const strip = renderSummary({ from: null });
    await userEvent.click(within(strip).getByRole("button", { name: /Inicial \+ aportes/ }));
    expect(within(strip).getByText(/início de cada posição/)).toBeVisible();
  });
});
