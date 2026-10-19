import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import type { Goal } from "../../api/contracts";
import { useGoals } from "../../hooks/useGoals";
import { MonthlyContributionIndicator } from "./MonthlyContributionIndicator";

vi.mock("../../hooks/useGoals");

function state(remaining: string | null = "1350") {
  return {
    goals: [{ name: "Reserva", target_date: "2027-12-31", monthly_contribution: {
      month: "2026-10", target: "2000", net_contributed: "650", remaining, assumes_zero_return: true,
    } } as Goal], isLoading: false, error: null,
  } as ReturnType<typeof useGoals>;
}
function show() { return render(<MemoryRouter><MonthlyContributionIndicator /></MemoryRouter>); }

describe("MonthlyContributionIndicator", () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date(2026, 9, 19, 12));
    vi.mocked(useGoals).mockReturnValue(state());
  });
  afterEach(() => vi.useRealTimers());

  it("shows backend BRL remainder and opens Metas", () => {
    show();
    const link = screen.getByRole("link", { name: /Falta aportar este mês/ });
    expect(link).toHaveAttribute("href", "/metas");
    expect(link).toHaveTextContent("R$ 1.350,00");
    expect(link).toHaveTextContent(/Reserva · dez.*2027/);
    expect(screen.queryByText(/Próxima meta|atrasad/i)).not.toBeInTheDocument();
  });
  it("distinguishes monthly coverage from goal completion", () => {
    vi.mocked(useGoals).mockReturnValue(state("0"));
    show();
    expect(screen.getByText("Aporte do mês em dia")).toBeVisible();
    expect(screen.queryByText(/Meta atingida/i)).not.toBeInTheDocument();
  });
  it("discloses an unavailable estimate without a fake zero", () => {
    vi.mocked(useGoals).mockReturnValue(state(null));
    show();
    expect(screen.getByText("Estimativa indisponível")).toBeVisible();
    expect(screen.queryByText(/R\$/)).not.toBeInTheDocument();
  });
  it.each([
    { ...state(), goals: [] },
    { ...state(), isLoading: true },
    { ...state(), error: new Error("unavailable") },
    { ...state(), goals: [{ ...state().goals[0], monthly_contribution: null }] },
  ])("reserves no space without a current answer", (value) => {
    vi.mocked(useGoals).mockReturnValue(value);
    expect(show().container).toBeEmptyDOMElement();
  });
  it("does not display last month's cached remainder", () => {
    vi.setSystemTime(new Date(2026, 10, 1, 12));
    expect(show().container).toBeEmptyDOMElement();
  });
});
