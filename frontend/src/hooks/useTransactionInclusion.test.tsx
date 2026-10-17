import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { queryKeys } from "../api/queryKeys";
import * as transactionApi from "../api/transactions";
import { transactionId } from "../test/transactionFixtures";
import { useTransactionInclusion } from "./useTransactionInclusion";

vi.mock("../api/transactions");

function setup() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return { client, wrapper };
}

describe("useTransactionInclusion", () => {
  it("cancels before PUT, avoids optimistic writes, stales the prefix and refetches active keys", async () => {
    const { client, wrapper } = setup();
    const order: string[] = [];
    vi.spyOn(client, "cancelQueries").mockImplementation(async () => {
      order.push("cancel");
    });
    vi.spyOn(client, "invalidateQueries").mockImplementation(async () => {
      order.push("invalidate");
    });
    vi.spyOn(client, "refetchQueries").mockImplementation(async () => {
      order.push("refetch");
    });
    const setData = vi.spyOn(client, "setQueryData");
    vi.mocked(transactionApi.setTransactionInclusion).mockImplementation(async () => {
      order.push("put");
      return { transaction_id: transactionId, state: "ignored", changed_at: "2026-07-30T13:00:00Z" };
    });
    const { result } = renderHook(useTransactionInclusion, { wrapper });

    act(() => result.current.setInclusion({ transactionId, state: "ignored" }));
    await waitFor(() =>
      expect(result.current.announcement).toBe("Transação ignorada. Totais atualizados."),
    );

    expect(order).toEqual(["cancel", "put", "invalidate", "invalidate", "refetch", "refetch"]);
    expect(setData).not.toHaveBeenCalled();
    expect(client.invalidateQueries).toHaveBeenCalledWith({
      queryKey: ["transactions"],
      refetchType: "none",
    });
    expect(client.refetchQueries).toHaveBeenCalledWith(
      {
        queryKey: ["transactions"],
        type: "active",
      },
      { throwOnError: true },
    );
  });

  it("clears active participant details and stales other participants when a fee is ignored", async () => {
    const { client, wrapper } = setup();
    const relationship = { id: "transfer" };
    client.setQueryData(queryKeys.transactionTransfer("source"), relationship);
    client.setQueryData(queryKeys.transactionTransfer("destination"), relationship);
    client.setQueryData(queryKeys.transactionTransfer("fee"), relationship);
    const readTransfer = vi.fn(async () => null);
    vi.mocked(transactionApi.setTransactionInclusion).mockResolvedValue({
      transaction_id: "fee", state: "ignored", changed_at: "2026-07-30T13:00:00Z",
    });
    const { result } = renderHook(() => ({
      inclusion: useTransactionInclusion(),
      source: useQuery({ queryKey: queryKeys.transactionTransfer("source"), queryFn: readTransfer, staleTime: Infinity }),
      fee: useQuery({ queryKey: queryKeys.transactionTransfer("fee"), queryFn: readTransfer, staleTime: Infinity }),
    }), { wrapper });
    expect(result.current.source.data).toEqual(relationship);
    expect(result.current.fee.data).toEqual(relationship);
    expect(readTransfer).not.toHaveBeenCalled();

    act(() => result.current.inclusion.setInclusion({ transactionId: "fee", state: "ignored" }));
    await waitFor(() => expect(result.current.inclusion.isPending).toBe(false));
    await waitFor(() => expect(result.current.source.data).toBeNull());
    expect(result.current.fee.data).toBeNull();
    expect(client.getQueryState(queryKeys.transactionTransfer("destination"))?.isInvalidated).toBe(true);
  });

  it("retries failed transfer-detail refresh without repeating the inclusion write", async () => {
    const { client, wrapper } = setup();
    client.setQueryData(queryKeys.transactionTransfer("source"), { id: "transfer" });
    const readTransfer = vi.fn()
      .mockRejectedValueOnce(new Error("transfer refresh failed"))
      .mockResolvedValue(null);
    vi.mocked(transactionApi.setTransactionInclusion).mockResolvedValue({
      transaction_id: "fee", state: "ignored", changed_at: "2026-07-30T13:00:00Z",
    });
    const { result } = renderHook(() => ({
      inclusion: useTransactionInclusion(),
      source: useQuery({ queryKey: queryKeys.transactionTransfer("source"), queryFn: readTransfer, staleTime: Infinity }),
    }), { wrapper });

    act(() => result.current.inclusion.setInclusion({ transactionId: "fee", state: "ignored" }));
    await waitFor(() => expect(result.current.inclusion.refreshError).toBe("transfer refresh failed"));
    await act(() => result.current.inclusion.retryRefresh());
    expect(result.current.source.data).toBeNull();
    expect(result.current.inclusion.refreshError).toBeNull();
    expect(transactionApi.setTransactionInclusion).toHaveBeenCalledTimes(1);
  });

  it("distinguishes a failed PUT retry from refresh-only recovery", async () => {
    const { client, wrapper } = setup();
    vi.mocked(transactionApi.setTransactionInclusion)
      .mockRejectedValueOnce(new Error("write failed"))
      .mockResolvedValueOnce({
        transaction_id: transactionId,
        state: "ignored",
        changed_at: "2026-07-30T13:00:00Z",
      });
    vi.spyOn(client, "refetchQueries")
      .mockRejectedValueOnce(new Error("refresh failed"))
      .mockResolvedValueOnce(undefined);
    const { result } = renderHook(useTransactionInclusion, { wrapper });

    act(() => result.current.setInclusion({ transactionId, state: "ignored" }));
    await waitFor(() => expect(result.current.writeError).toBe("write failed"));
    act(() => result.current.retryWrite());
    await waitFor(() => expect(result.current.refreshError).toBe("refresh failed"));
    expect(transactionApi.setTransactionInclusion).toHaveBeenCalledTimes(2);
    await act(() => result.current.retryRefresh());
    expect(transactionApi.setTransactionInclusion).toHaveBeenCalledTimes(2);
    expect(result.current.announcement).toBe("Resultados atualizados.");
  });
});
