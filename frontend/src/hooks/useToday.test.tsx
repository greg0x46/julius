import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useToday } from "./useToday";

const day = (today: { format: (template: string) => string }) => today.format("YYYY-MM-DD");

describe("useToday", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 9, 22, 23, 59, 30));
  });

  afterEach(() => vi.useRealTimers());

  it("starts on the current day", () => {
    const { result } = renderHook(() => useToday());

    expect(day(result.current)).toBe("2026-10-22");
  });

  it("moves to the next day just after midnight, and again the day after", () => {
    const { result } = renderHook(() => useToday());

    act(() => vi.advanceTimersByTime(29_000));
    expect(day(result.current)).toBe("2026-10-22");

    act(() => vi.advanceTimersByTime(2_000));
    expect(day(result.current)).toBe("2026-10-23");

    act(() => vi.advanceTimersByTime(24 * 60 * 60 * 1000));
    expect(day(result.current)).toBe("2026-10-24");
  });

  it("rolls over the end of the month", () => {
    vi.setSystemTime(new Date(2026, 9, 31, 23, 59, 59));
    const { result } = renderHook(() => useToday());

    act(() => vi.advanceTimersByTime(2_000));
    expect(day(result.current)).toBe("2026-11-01");
  });

  it("stops its timer on unmount", () => {
    const { unmount } = renderHook(() => useToday());
    expect(vi.getTimerCount()).toBe(1);

    unmount();
    expect(vi.getTimerCount()).toBe(0);
  });
});
