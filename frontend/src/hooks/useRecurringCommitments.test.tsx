import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import * as api from "../api/recurringCommitments";
import { useRecurringCommitments } from "./useRecurringCommitments";

vi.mock("../api/recurringCommitments");

describe("useRecurringCommitments", () => {
  it.each([
    ["toggle", (hook: ReturnType<typeof useRecurringCommitments>) => hook.toggleCommitment({ commitmentId: "c1", isActive: false }).then(() => undefined), "setRecurringCommitmentActive"],
    ["delete", (hook: ReturnType<typeof useRecurringCommitments>) => hook.deleteCommitment("c1"), "deleteRecurringCommitment"],
  ] as const)("refreshes the projection views after a %s", async (_name, run, apiName) => {
    vi.mocked(api.listRecurringCommitments).mockResolvedValue([]);
    vi.mocked(api[apiName] as never as () => Promise<unknown>).mockResolvedValue(undefined as never);
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidate = vi.spyOn(client, "invalidateQueries");
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useRecurringCommitments(), { wrapper });
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    await act(() => run(result.current));

    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["recurringCommitments"] });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["timeline"] });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["timeline-data-range"] });
  });
});
