import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { applyStoredTheme, useTheme } from "./theme";

afterEach(() => {
  window.localStorage.clear();
  delete document.documentElement.dataset.theme;
});

describe("theme", () => {
  it("persists the choice and sets data-theme", () => {
    const { result } = renderHook(() => useTheme());
    act(() => result.current[1]("dark"));
    expect(result.current[0]).toBe("dark");
    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(window.localStorage.getItem("julius.theme")).toBe("dark");
  });

  it("restores the saved theme and ignores invalid values", () => {
    window.localStorage.setItem("julius.theme", "dark");
    applyStoredTheme();
    expect(document.documentElement.dataset.theme).toBe("dark");

    delete document.documentElement.dataset.theme;
    window.localStorage.setItem("julius.theme", "sepia");
    applyStoredTheme();
    expect(document.documentElement.dataset.theme).toBeUndefined();
  });
});
