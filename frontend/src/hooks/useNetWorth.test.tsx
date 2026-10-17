import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import * as netWorthApi from "../api/netWorth";
import type { NetWorthSnapshot } from "../api/contracts";
import { useNetWorth } from "./useNetWorth";

vi.mock("../api/netWorth");

const snapshot: NetWorthSnapshot = {
  captured_at: "2026-08-18T00:00:00Z",
  is_backfilled: false,
  valuation_version: 0,
  valuation_status: "legacy",
  cash_balance: "1000.00",
  investment_balance: "500.00",
  credit_card_balance: "300.00",
  payables_debt: "200.00",
  total_assets: "1500.00",
  total_liabilities: "500.00",
  net_worth: "1150.00",
};

function setup() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return { client, wrapper };
}

describe("useNetWorth", () => {
  it("exposes the series and latest snapshot from the backend", async () => {
    vi.mocked(netWorthApi.getNetWorth).mockResolvedValue({ series: [snapshot], latest: snapshot });
    const { wrapper } = setup();
    const { result } = renderHook(useNetWorth, { wrapper });

    await waitFor(() => expect(result.current.latest).toEqual(snapshot));
    expect(result.current.series).toEqual([snapshot]);
    expect(result.current.isLoading).toBe(false);
  });

  it("starts with an empty series and null latest while loading", () => {
    vi.mocked(netWorthApi.getNetWorth).mockReturnValue(new Promise(() => {}));
    const { wrapper } = setup();
    const { result } = renderHook(useNetWorth, { wrapper });

    expect(result.current.series).toEqual([]);
    expect(result.current.latest).toBeNull();
    expect(result.current.isLoading).toBe(true);
  });

  it("surfaces an error from the backend", async () => {
    vi.mocked(netWorthApi.getNetWorth).mockRejectedValue(new Error("boom"));
    const { wrapper } = setup();
    const { result } = renderHook(useNetWorth, { wrapper });

    await waitFor(() => expect(result.current.error).toBeTruthy());
    expect(result.current.latest).toBeNull();
  });
});
