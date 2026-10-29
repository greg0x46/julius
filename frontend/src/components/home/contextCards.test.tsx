import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Goal, NetWorthPoint, PayableTotalOwed } from "../../api/contracts";
import { ApiError } from "../../api/problems";
import * as goalsHook from "../../hooks/useGoals";
import * as netWorthHook from "../../hooks/useNetWorth";
import * as owedHook from "../../hooks/usePayableTotalOwed";
import { NetWorthSummaryCard } from "./NetWorthSummaryCard";
import { PrimaryGoalCard } from "./PrimaryGoalCard";
import { TotalOwedCard } from "./TotalOwedCard";

vi.mock("../../hooks/useNetWorth");
vi.mock("../../hooks/useGoals");
vi.mock("../../hooks/usePayableTotalOwed");

const wrap = (node: React.ReactNode) => render(<MemoryRouter>{node}</MemoryRouter>);

function point(day: string, total: string, overrides: Partial<NetWorthPoint> = {}): NetWorthPoint {
  return {
    date: day,
    valuation_status: "complete",
    cash_balance: total,
    investment_balance: "0.00",
    total,
    ...overrides,
  };
}

function mockNetWorth(partial: Partial<ReturnType<typeof netWorthHook.useNetWorth>>) {
  vi.mocked(netWorthHook.useNetWorth).mockReturnValue({
    series: [],
    latest: null,
    isLoading: false,
    error: null,
    refetch: vi.fn(),
    ...partial,
  } as unknown as ReturnType<typeof netWorthHook.useNetWorth>);
}

describe("NetWorthSummaryCard", () => {
  beforeEach(() => vi.clearAllMocks());

  it("links to the wealth page with the figure and the exact change since the oldest valued day", () => {
    const series = [point("2026-09-25", "180000.10"), point("2026-10-22", "184300.00")];
    mockNetWorth({ series, latest: series[1] });
    wrap(<NetWorthSummaryCard />);

    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "/patrimonio");
    expect(link).toHaveTextContent("R$ 184.300,00");
    expect(link).toHaveTextContent("+R$ 4.299,90 desde 25/09");
    expect(screen.getByRole("img", { name: "Evolução do patrimônio desde 25/09" })).toBeInTheDocument();
  });

  it("says there is not enough history instead of inventing a change", () => {
    const series = [point("2026-10-22", "184300.00")];
    mockNetWorth({ series, latest: series[0] });
    wrap(<NetWorthSummaryCard />);

    expect(screen.getByText(/Ainda não há histórico suficiente/)).toBeVisible();
    expect(screen.queryByRole("img")).toBeNull();
  });

  it("uses the response currency, not BRL", () => {
    const latest = point("2026-10-22", "1000.00", { currency_code: "USD" });
    mockNetWorth({ series: [latest], latest });
    wrap(<NetWorthSummaryCard />);

    expect(screen.getByRole("link")).toHaveTextContent("USD 1.000,00");
    expect(screen.queryByText(/R\$/)).toBeNull();
  });

  it("states that a wealth figure that cannot be valued is unavailable, with no zero", () => {
    const latest = point("2026-10-22", "0", { valuation_status: "unavailable", cash_balance: undefined, investment_balance: undefined, total: undefined });
    mockNetWorth({ series: [latest], latest });
    wrap(<NetWorthSummaryCard />);

    expect(screen.getByRole("link")).toHaveTextContent("Indisponível: faltam cotações de câmbio");
    expect(screen.queryByText(/R\$ 0,00/)).toBeNull();
  });

  it("shows a skeleton while loading", () => {
    mockNetWorth({ isLoading: true });
    wrap(<NetWorthSummaryCard />);

    expect(screen.getByRole("status", { name: "Carregando o patrimônio" })).toBeInTheDocument();
  });

  it("offers a retry on error", async () => {
    const refetch = vi.fn();
    mockNetWorth({ error: new ApiError("response", "Patrimônio fora do ar."), refetch });
    wrap(<NetWorthSummaryCard />);

    expect(screen.getByText("Patrimônio fora do ar.")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Tentar novamente" }));
    expect(refetch).toHaveBeenCalled();
  });
});

function goal(overrides: Partial<Goal>): Goal {
  return {
    id: "g1",
    name: "Reserva",
    metric: "invested_value",
    target_amount: "100000.00",
    target_date: "2028-08-31",
    notes: null,
    current_value: "62000.00",
    progress: "0.62",
    projection: {
      status: "on_track",
      months_remaining: 22,
      monthly_return: "0.01",
      return_from: "2025-10-01",
      return_partial: false,
      projected_value: "100000.00",
      projected_progress: "1.00",
      required_monthly_contribution: "0",
    },
    currency_code: "BRL",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function mockGoals(partial: Partial<ReturnType<typeof goalsHook.useGoals>>) {
  vi.mocked(goalsHook.useGoals).mockReturnValue({
    goals: [],
    isLoading: false,
    error: null,
    refetch: vi.fn(),
    ...partial,
  } as unknown as ReturnType<typeof goalsHook.useGoals>);
}

describe("PrimaryGoalCard", () => {
  beforeEach(() => vi.clearAllMocks());

  it("shows progress, amounts and the on-track projection as a link to the goals", () => {
    mockGoals({ goals: [goal({})] });
    wrap(<PrimaryGoalCard />);

    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "/metas");
    expect(link).toHaveTextContent("Meta · Reserva");
    expect(link).toHaveTextContent("62%");
    expect(link).toHaveTextContent("R$ 62.000,00 de R$ 100.000,00");
    expect(link).toHaveTextContent("No ritmo para 31/08/2028");
    expect(screen.getByRole("progressbar", { name: "Progresso de Reserva" })).toHaveAttribute("aria-valuenow", "62");
  });

  it("states the monthly contribution when behind", () => {
    mockGoals({
      goals: [
        goal({
          progress: "0.2",
          current_value: "20000.00",
          projection: { ...goal({}).projection!, status: "behind", required_monthly_contribution: "2500.50" },
        }),
      ],
    });
    wrap(<PrimaryGoalCard />);

    expect(screen.getByRole("link")).toHaveTextContent("Atrás do ritmo para 31/08/2028: aporte de R$ 2.500,50 por mês");
  });

  it("picks the first open goal and counts the others", () => {
    mockGoals({
      goals: [
        goal({ id: "done", name: "Concluída", progress: "1.2", current_value: "120.00", target_amount: "100.00" }),
        goal({ id: "big", name: "Grande", target_amount: "500000.00", progress: "0.1" }),
        goal({ id: "small", name: "Pequena", target_amount: "1000.00", progress: "0.5" }),
      ],
    });
    wrap(<PrimaryGoalCard />);

    expect(screen.getByRole("link")).toHaveTextContent("Meta · Pequena");
    expect(screen.getByRole("link")).toHaveTextContent("mais 2 metas");
  });

  it("caps the meter at 100% but keeps the real percentage for a goal beyond its target", () => {
    mockGoals({ goals: [goal({ progress: "1.2", current_value: "120000.00", projection: null })] });
    wrap(<PrimaryGoalCard />);

    expect(screen.getByRole("link")).toHaveTextContent("120%");
    expect(screen.getByRole("link")).toHaveTextContent("Meta atingida");
    expect(screen.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "100");
  });

  it("invites creating a goal when there are none", () => {
    mockGoals({ goals: [] });
    wrap(<PrimaryGoalCard />);

    expect(screen.getByText("Você ainda não tem metas.")).toBeVisible();
    expect(screen.getByRole("link", { name: "Criar meta" })).toHaveAttribute("href", "/metas");
  });

  it("shows a skeleton while loading and a retry on error", async () => {
    mockGoals({ isLoading: true });
    const { unmount } = wrap(<PrimaryGoalCard />);
    expect(screen.getByRole("status", { name: "Carregando a meta" })).toBeInTheDocument();
    unmount();

    const refetch = vi.fn();
    mockGoals({ error: new ApiError("response", "Metas fora do ar."), refetch });
    wrap(<PrimaryGoalCard />);
    expect(screen.getByText("Metas fora do ar.")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Tentar novamente" }));
    expect(refetch).toHaveBeenCalled();
  });
});

function mockOwed(partial: Partial<ReturnType<typeof owedHook.usePayableTotalOwed>>) {
  vi.mocked(owedHook.usePayableTotalOwed).mockReturnValue({
    total: null,
    isLoading: false,
    error: null,
    refetch: vi.fn(),
    ...partial,
  } as unknown as ReturnType<typeof owedHook.usePayableTotalOwed>);
}

const owed: PayableTotalOwed = {
  remaining_debts_total: "16310.00",
  future_installments_total: "2140.00",
  total_owed: "18450.00",
  currency_code: "BRL",
};

describe("TotalOwedCard", () => {
  beforeEach(() => vi.clearAllMocks());

  it("shows the total with its two parts, as a link to Pendências", () => {
    mockOwed({ total: owed });
    wrap(<TotalOwedCard />);

    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "/pendencias");
    expect(link).toHaveTextContent("R$ 18.450,00");
    expect(link).toHaveTextContent("Pendências R$ 16.310,00 + faturas abertas R$ 2.140,00");
  });

  it("says plainly when nothing is owed", () => {
    mockOwed({ total: { ...owed, remaining_debts_total: "0.00", future_installments_total: "0.00", total_owed: "0.00" } });
    wrap(<TotalOwedCard />);

    expect(screen.getByRole("link")).toHaveTextContent("Nenhuma dívida em aberto");
  });

  it("shows a skeleton while loading and a retry on error", async () => {
    mockOwed({ isLoading: true });
    const { unmount } = wrap(<TotalOwedCard />);
    expect(screen.getByRole("status", { name: "Carregando a dívida total" })).toBeInTheDocument();
    unmount();

    const refetch = vi.fn();
    mockOwed({ error: new ApiError("response", "Dívidas fora do ar."), refetch });
    wrap(<TotalOwedCard />);
    expect(screen.getByText("Dívidas fora do ar.")).toBeVisible();
    expect(screen.queryByText(/R\$ 0,00/)).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Tentar novamente" }));
    expect(refetch).toHaveBeenCalled();
  });
});
