import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { TimelineResponse } from "../../api/contracts";
import { queryKeys } from "../../api/queryKeys";
import * as timelineApi from "../../api/timeline";
import { DailyAllowanceCard } from "./DailyAllowanceCard";

vi.mock("../../api/timeline");

// One point per day from 2026-10-22 (the faked today) to the end of the month.
const monthDays = Array.from({ length: 10 }, (_, index) => `2026-10-${22 + index}`);

function response(balances: string | string[], startingBalance = "5000.00"): TimelineResponse {
  const list = typeof balances === "string" ? monthDays.map(() => balances) : balances;
  const points = monthDays.map((date, index) => ({
    date,
    balance: list[index] ?? list[list.length - 1],
    inflow: "0.00",
    outflow: "0.00",
    lowest_tier: "projetado",
  }));
  return {
    base: {
      points,
      entries: [],
      starting_balance: startingBalance,
      lowest_balance: points[0],
      first_negative: null,
    },
    period_totals: { income: "0.00", expense: "0.00", result: "0.00" },
    simulation: null,
    scenario_impacts: [],
  } as unknown as TimelineResponse;
}

const params = { referenceDate: "2026-10-22", from: "2026-10-22", to: "2026-10-31" };

function renderCard() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <DailyAllowanceCard />
    </QueryClientProvider>,
  );
  return client;
}

// The figure is split across the amount and its unit, so match the line's whole text.
const figureText = () => document.querySelector(".daily-allowance-value")?.textContent?.replace(/\s/g, " ");

describe("DailyAllowanceCard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Only Date: react-query and testing-library still need real timers.
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date(2026, 9, 22, 10, 0, 0));
  });

  afterEach(() => vi.useRealTimers());

  it("splits the balance over the days left", async () => {
    vi.mocked(timelineApi.getTimeline).mockResolvedValue(response("855.00"));
    renderCard();

    await waitFor(() => expect(figureText()).toBe("R$ 85,50 / dia"));
    expect(screen.getByText("Disponível por dia")).toBeVisible();
    expect(screen.getByText("Disponível para gastar até o fim do mês")).toBeVisible();
    expect(timelineApi.getTimeline).toHaveBeenCalledWith(params, expect.anything());
  });

  it("weighs an early low point by the days it has to cover", async () => {
    // 100 on the second day can only pay for two days, however rich the rest of the month is.
    vi.mocked(timelineApi.getTimeline).mockResolvedValue(response(["5000.00", "100.00", "5000.00"]));
    renderCard();

    await waitFor(() => expect(figureText()).toBe("R$ 50,00 / dia"));
  });

  it("reads today's point, so what falls due today is already out of it", async () => {
    // 3000 in cash, 1500 of rent due today: the server's point for today is 1500.
    vi.mocked(timelineApi.getTimeline).mockResolvedValue(response("1500.00", "3000.00"));
    renderCard();

    await waitFor(() => expect(figureText()).toBe("R$ 150,00 / dia"));
  });

  it("shows zero when the month dips below zero", async () => {
    vi.mocked(timelineApi.getTimeline).mockResolvedValue(response(["5000.00", "-300.00"]));
    renderCard();

    await waitFor(() => expect(figureText()).toBe("R$ 0,00 / dia"));
  });

  it("shows a skeleton while loading", () => {
    vi.mocked(timelineApi.getTimeline).mockReturnValue(new Promise(() => {}));
    renderCard();

    expect(screen.getByRole("status", { name: "Carregando o gasto diário disponível" })).toBeInTheDocument();
  });

  it("offers a retry when the timeline fails", async () => {
    vi.mocked(timelineApi.getTimeline)
      .mockRejectedValueOnce(new Error("boom"))
      .mockResolvedValue(response("855.00"));
    renderCard();

    const retry = await screen.findByRole("button", { name: "Tentar novamente" });
    expect(screen.getByText("Não foi possível calcular o gasto diário disponível.")).toBeVisible();
    await userEvent.click(retry);
    await waitFor(() => expect(figureText()).toBe("R$ 85,50 / dia"));
  });

  it("keeps the last figure when a background refetch fails", async () => {
    vi.mocked(timelineApi.getTimeline)
      .mockResolvedValueOnce(response("855.00"))
      .mockRejectedValue(new Error("boom"));
    const client = renderCard();

    await waitFor(() => expect(figureText()).toBe("R$ 85,50 / dia"));
    await act(() => client.invalidateQueries({ queryKey: queryKeys.timeline }));
    await waitFor(() => expect(timelineApi.getTimeline).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(client.getQueryState(queryKeys.timelineFor(params))?.status).toBe("error"));

    expect(figureText()).toBe("R$ 85,50 / dia");
    expect(screen.queryByRole("button", { name: "Tentar novamente" })).not.toBeInTheDocument();
  });

  it("follows the timeline when a write invalidates it, storing nothing", async () => {
    vi.mocked(timelineApi.getTimeline).mockResolvedValueOnce(response("855.00")).mockResolvedValue(response("500.00"));
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    const client = renderCard();

    await waitFor(() => expect(figureText()).toBe("R$ 85,50 / dia"));
    await act(() => client.invalidateQueries({ queryKey: queryKeys.timeline }));
    await waitFor(() => expect(figureText()).toBe("R$ 50,00 / dia"));
    expect(setItem).not.toHaveBeenCalled();
    setItem.mockRestore();
  });
});
