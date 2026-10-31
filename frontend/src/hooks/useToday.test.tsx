import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { TimezoneContext } from "./useTimezone";
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

  // The day turns over at midnight in the configured zone, not the machine's.
  it.each([
    ["America/Sao_Paulo", "2026-10-23T02:59:30Z", "2026-10-22", "2026-10-23"],
    ["Asia/Tokyo", "2026-10-22T14:59:30Z", "2026-10-22", "2026-10-23"],
    ["UTC", "2026-10-22T23:59:30Z", "2026-10-22", "2026-10-23"],
  ])("rolls over at midnight in %s", (zone, start, before, after) => {
    vi.setSystemTime(new Date(start));
    const { result } = renderHook(() => useToday(), {
      wrapper: ({ children }) => <TimezoneContext.Provider value={zone}>{children}</TimezoneContext.Provider>,
    });
    expect(day(result.current)).toBe(before);

    act(() => vi.advanceTimersByTime(29_000));
    expect(day(result.current)).toBe(before);

    act(() => vi.advanceTimersByTime(2_000));
    expect(day(result.current)).toBe(after);
  });

  it("moves to the new day when the configured zone changes", () => {
    vi.setSystemTime(new Date("2026-10-22T20:00:00Z"));
    const zone = { current: "America/Sao_Paulo" };
    const { result, rerender } = renderHook(() => useToday(), {
      wrapper: ({ children }) => <TimezoneContext.Provider value={zone.current}>{children}</TimezoneContext.Provider>,
    });
    expect(day(result.current)).toBe("2026-10-22");

    zone.current = "Asia/Tokyo";
    rerender();
    expect(day(result.current)).toBe("2026-10-23");
  });

  it("stops its timer on unmount", () => {
    const { unmount } = renderHook(() => useToday());
    expect(vi.getTimerCount()).toBe(1);

    unmount();
    expect(vi.getTimerCount()).toBe(0);
  });
});
