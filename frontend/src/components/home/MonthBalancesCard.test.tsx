import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { TimelineResponse } from "../../api/contracts";
import { ApiError } from "../../api/problems";
import * as timelineApi from "../../api/timeline";
import { MonthBalancesCard } from "./MonthBalancesCard";

vi.mock("../../api/timeline");

const monthDays = Array.from({ length: 10 }, (_, index) => `2026-10-${22 + index}`);

function response(startingBalance: string, lastBalance: string, currencyCode = "BRL"): TimelineResponse {
  const points = monthDays.map((date, index) => ({
    date,
    balance: index === monthDays.length - 1 ? lastBalance : startingBalance,
    inflow: "0.00",
    outflow: "0.00",
    lowest_tier: "projetado",
  }));
  return {
    currency_code: currencyCode,
    base: { points, entries: [], starting_balance: startingBalance, lowest_balance: points[0], first_negative: null },
    period_totals: { income: "0.00", expense: "0.00", result: "0.00" },
    simulation: null,
    scenario_impacts: [],
  } as unknown as TimelineResponse;
}

function renderCard() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <MonthBalancesCard />
    </QueryClientProvider>,
  );
}

describe("MonthBalancesCard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date(2026, 9, 22, 10, 0, 0));
  });

  afterEach(() => vi.useRealTimers());

  it("shows the cash now and the month-end balance, the latter marked as a projection", async () => {
    vi.mocked(timelineApi.getTimeline).mockResolvedValue(response("8420.00", "5270.10"));
    renderCard();

    const now = (await screen.findByText("Saldo hoje")).closest("div");
    expect(now).toHaveTextContent("R$ 8.420,00");
    const end = screen.getByText(/Em 31\/10/).closest("div");
    expect(end).toHaveTextContent("R$ 5.270,10");
    expect(end).toHaveTextContent("projetado");
  });

  it("keeps a negative projected balance negative", async () => {
    vi.mocked(timelineApi.getTimeline).mockResolvedValue(response("100.00", "-900.00"));
    renderCard();

    expect(await screen.findByText("-R$ 900,00")).toBeVisible();
  });

  it("reads the response currency", async () => {
    vi.mocked(timelineApi.getTimeline).mockResolvedValue(response("200", "100", "USD"));
    renderCard();

    expect(await screen.findByText("USD 200,00")).toBeVisible();
    expect(screen.queryByText(/R\$/)).toBeNull();
  });

  it("shows a skeleton while loading", () => {
    vi.mocked(timelineApi.getTimeline).mockReturnValue(new Promise(() => {}));
    renderCard();

    expect(screen.getByRole("status", { name: "Carregando os saldos do mês" })).toBeInTheDocument();
  });

  it("offers a retry when the timeline fails, and never a zero balance", async () => {
    vi.mocked(timelineApi.getTimeline)
      .mockRejectedValueOnce(new ApiError("response", "Falta cotação de BRL para USD."))
      .mockResolvedValue(response("8420.00", "5270.10"));
    renderCard();

    expect(await screen.findByText("Falta cotação de BRL para USD.")).toBeVisible();
    expect(screen.queryByText(/R\$ 0,00/)).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Tentar novamente" }));
    await waitFor(() => expect(screen.getByText("Saldo hoje")).toBeVisible());
  });
});
