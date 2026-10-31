import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import * as transactionApi from "../api/transactions";
import { transactionResult } from "../test/transactionFixtures";
import { QueryTestProvider } from "../test/QueryTestProvider";
import { TimezoneContext } from "./useTimezone";
import { currentMonthFilters, useTransactions } from "./useTransactions";

vi.mock("../api/transactions");

const zone = "America/Sao_Paulo";

describe("useTransactions", () => {
  it("disables automatic retry, forwards cancellation, and manually retries", async () => {
    vi.mocked(transactionApi.queryTransactions)
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce(transactionResult);
    const { result } = renderHook(
      () => useTransactions(currentMonthFilters(zone, new Date("2026-07-30T15:00:00Z")), "month", 1),
      { wrapper: QueryTestProvider },
    );
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(transactionApi.queryTransactions).toHaveBeenCalledTimes(1);
    await act(() => result.current.refetch());
    await waitFor(() => expect(result.current.data).toEqual(transactionResult));
  });

  it("preserves a confirmed same-key snapshot after a refetch error", async () => {
    vi.mocked(transactionApi.queryTransactions)
      .mockResolvedValueOnce(transactionResult)
      .mockRejectedValueOnce(new Error("offline"));
    const { result } = renderHook(
      () => useTransactions(currentMonthFilters(zone, new Date("2026-07-30T15:00:00Z")), "month", 1),
      { wrapper: QueryTestProvider },
    );
    await waitFor(() => expect(result.current.data).toEqual(transactionResult));
    await act(() => result.current.refetch());
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toEqual(transactionResult);
  });

  it("does not carry data across query identities", async () => {
    vi.mocked(transactionApi.queryTransactions)
      .mockResolvedValueOnce(transactionResult)
      .mockImplementationOnce(() => new Promise(() => undefined));
    const filters = currentMonthFilters(zone, new Date("2026-07-30T15:00:00Z"));
    const { result, rerender } = renderHook(
      ({ page }) => useTransactions(filters, "month", page),
      { wrapper: QueryTestProvider, initialProps: { page: 1 } },
    );
    await waitFor(() => expect(result.current.data).toEqual(transactionResult));
    rerender({ page: 2 });
    expect(result.current.data).toBeUndefined();
  });

  it("sends the configured timezone, not the machine's", async () => {
    vi.mocked(transactionApi.queryTransactions).mockResolvedValue(transactionResult);
    const { result } = renderHook(() => useTransactions(currentMonthFilters("Asia/Tokyo"), "month", 1), {
      wrapper: ({ children }) => (
        <QueryTestProvider>
          <TimezoneContext.Provider value="Asia/Tokyo">{children}</TimezoneContext.Provider>
        </QueryTestProvider>
      ),
    });
    await waitFor(() => expect(result.current.data).toEqual(transactionResult));
    expect(result.current.query.timezone).toBe("Asia/Tokyo");
    expect(transactionApi.queryTransactions).toHaveBeenCalledWith(
      expect.objectContaining({ timezone: "Asia/Tokyo" }),
      expect.any(AbortSignal),
    );
  });
});

describe("currentMonthFilters", () => {
  // 16:00Z on July 31st is already August 1st in Tokyo, still July 31st in São Paulo.
  const instant = new Date("2026-07-31T16:00:00Z");

  it("takes the month from the day in the given zone", () => {
    expect(currentMonthFilters("America/Sao_Paulo", instant)).toMatchObject({
      date_from: "2026-07-01",
      date_to: "2026-07-31",
    });
    expect(currentMonthFilters("Asia/Tokyo", instant)).toMatchObject({
      date_from: "2026-08-01",
      date_to: "2026-08-31",
    });
  });
});
