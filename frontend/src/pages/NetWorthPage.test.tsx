import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import dayjs from "dayjs";
import { MemoryRouter } from "react-router-dom";
import { afterEach, expect, it, vi } from "vitest";
import { getInvestmentHistory } from "../api/investmentPortfolio";
import { investmentPeriodFixture } from "../test/investmentWorkspaceFixtures";
import { NetWorthPage } from "./NetWorthPage";

vi.mock("../api/investmentPortfolio", () => ({ getInvestmentHistory: vi.fn() }));
vi.mock("../hooks/useToday", () => ({ useToday: () => dayjs("2026-10-19") }));
vi.mock("../hooks/useNetWorth", () => ({ useNetWorth: () => ({
  series: [], latest: { valuation_status: "unavailable", captured_at: "2026-10-19", currency_code: "BRL" },
  isLoading: false, error: null,
}) }));
vi.mock("../components/netWorth/NetWorthChart", () => ({ NetWorthChart: ({ from, to, history }: { from: string; to: string; history?: unknown }) =>
  <div data-testid="chart">{from} a {to} · {history ? "loaded" : "loading"}</div> }));

afterEach(() => window.localStorage.clear());

it("shares the requested period and clamps history to today, clearing old data when the period changes", async () => {
  window.localStorage.setItem("julius.periodo", JSON.stringify({ from: "2026-10-01", to: "2026-10-31" }));
  const period = investmentPeriodFixture();
  vi.mocked(getInvestmentHistory).mockResolvedValueOnce({ baseline: period, periods: [period] });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><MemoryRouter><NetWorthPage /></MemoryRouter></QueryClientProvider>);
  await waitFor(() => expect(screen.getByTestId("chart")).toHaveTextContent("2026-10-01 a 2026-10-19 · loaded"));
  expect(getInvestmentHistory).toHaveBeenCalledWith("2026-10-01", "2026-10-19", expect.any(AbortSignal));
  vi.mocked(getInvestmentHistory).mockImplementation(() => new Promise(() => {}));
  fireEvent.click(screen.getByRole("button", { name: "Mês anterior" }));
  await waitFor(() => expect(getInvestmentHistory).toHaveBeenCalledWith("2026-09-01", "2026-09-30", expect.any(AbortSignal)));
  expect(screen.getByTestId("chart")).toHaveTextContent("2026-09-01 a 2026-09-30 · loading");
  client.clear();
});
