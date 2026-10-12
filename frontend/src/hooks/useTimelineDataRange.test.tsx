import { renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import * as timelineApi from "../api/timeline";
import { QueryTestProvider } from "../test/QueryTestProvider";
import { useTimelineDataRange } from "./useTimelineDataRange";
import { browserTimezone } from "./useTransactions";

vi.mock("../api/timeline");
vi.mock("./useTransactions", () => ({ browserTimezone: vi.fn() }));

describe("useTimelineDataRange", () => {
  it("requests the browser timezone and fetches new bounds when it changes", async () => {
    vi.mocked(browserTimezone).mockReturnValue("America/Sao_Paulo");
    vi.mocked(timelineApi.getTimelineDataRange)
      .mockResolvedValueOnce({ from: "2026-03-15", to: "2026-03-31" })
      .mockResolvedValueOnce({ from: "2026-03-16", to: "2026-03-31" });
    const { result, rerender } = renderHook(() => useTimelineDataRange(true), {
      wrapper: QueryTestProvider,
    });
    await waitFor(() => expect(result.current.range?.from).toBe("2026-03-15"));
    expect(timelineApi.getTimelineDataRange).toHaveBeenCalledWith("America/Sao_Paulo", expect.any(AbortSignal));
    vi.mocked(browserTimezone).mockReturnValue("UTC");
    rerender();
    await waitFor(() => expect(result.current.range?.from).toBe("2026-03-16"));
    expect(timelineApi.getTimelineDataRange).toHaveBeenCalledWith("UTC", expect.any(AbortSignal));
  });
});
