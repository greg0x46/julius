import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import * as payablesApi from "../api/payables";
import type { Payable } from "../api/contracts";
import { usePayables } from "./usePayables";

vi.mock("../api/payables");

const payableId = "44444444-4444-4444-8444-444444444444";

const debt: Payable = {
  id: payableId,
  kind: "debt",
  name: "Financiamento do carro",
  total_amount: "1000",
  starting_settled_amount: "0",
  settled_amount: "200",
  remaining_amount: "800",
  status: "open",
  link_count: 1,
  created_at: "2026-07-30T12:00:00Z",
  updated_at: "2026-07-30T12:00:00Z",
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

describe("usePayables", () => {
  it("lists payables from the backend", async () => {
    vi.mocked(payablesApi.listPayables).mockResolvedValue([debt]);
    const { wrapper } = setup();
    const { result } = renderHook(() => usePayables(), { wrapper });
    await waitFor(() => expect(result.current.payables).toEqual([debt]));
    expect(payablesApi.listPayables).toHaveBeenCalledWith(null, expect.anything());
  });

  it("filters by kind when given one", async () => {
    vi.mocked(payablesApi.listPayables).mockResolvedValue([debt]);
    const { wrapper } = setup();
    renderHook(() => usePayables("debt"), { wrapper });
    await waitFor(() => expect(payablesApi.listPayables).toHaveBeenCalledWith("debt", expect.anything()));
  });

  it("invalidates the payables list after creating", async () => {
    vi.mocked(payablesApi.listPayables).mockResolvedValue([]);
    vi.mocked(payablesApi.createPayable).mockResolvedValue(debt);
    const { client, wrapper } = setup();
    const invalidateSpy = vi.spyOn(client, "invalidateQueries");
    const { result } = renderHook(() => usePayables(), { wrapper });
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    await act(() =>
      result.current.createPayable({
        kind: "debt",
        name: debt.name,
        total_amount: "1000",
        initial_remaining_amount: null,
      }),
    );

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["payables"] });
  });

  it("updates a payable and invalidates the list", async () => {
    vi.mocked(payablesApi.listPayables).mockResolvedValue([debt]);
    vi.mocked(payablesApi.updatePayable).mockResolvedValue({ ...debt, name: "Renomeada" });
    const { wrapper } = setup();
    const { result } = renderHook(() => usePayables(), { wrapper });
    await waitFor(() => expect(result.current.payables).toEqual([debt]));

    await act(() =>
      result.current.updatePayable({ payableId, write: { name: "Renomeada", total_amount: "1000" } }),
    );

    expect(payablesApi.updatePayable).toHaveBeenCalledWith(payableId, {
      name: "Renomeada",
      total_amount: "1000",
    });
  });

  it("deletes a payable and invalidates the list", async () => {
    vi.mocked(payablesApi.listPayables).mockResolvedValue([debt]);
    vi.mocked(payablesApi.deletePayable).mockResolvedValue(undefined);
    const { client, wrapper } = setup();
    const invalidateSpy = vi.spyOn(client, "invalidateQueries");
    const { result } = renderHook(() => usePayables(), { wrapper });
    await waitFor(() => expect(result.current.payables).toEqual([debt]));

    await act(() => result.current.deletePayable(payableId));

    expect(payablesApi.deletePayable).toHaveBeenCalledWith(payableId);
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["payables"] });
  });
});
