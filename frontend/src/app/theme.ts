import { useCallback, useEffect, useState } from "react";

export type Theme = "light" | "dark";

const storageKey = "julius.theme";
const darkQuery = "(prefers-color-scheme: dark)";

function storedTheme(): Theme | null {
  try {
    const value = window.localStorage.getItem(storageKey);
    return value === "light" || value === "dark" ? value : null;
  } catch {
    return null;
  }
}

function systemTheme(): Theme {
  return window.matchMedia?.(darkQuery).matches ? "dark" : "light";
}

/** Applies the saved choice; without one the CSS follows the OS setting. */
export function applyStoredTheme() {
  const theme = storedTheme();
  if (theme) document.documentElement.dataset.theme = theme;
}

export function useTheme(): [Theme, (theme: Theme) => void] {
  const [theme, setThemeState] = useState<Theme>(() => storedTheme() ?? systemTheme());

  useEffect(() => {
    if (storedTheme()) return;
    const query = window.matchMedia?.(darkQuery);
    if (!query) return;
    const onChange = () => setThemeState(systemTheme());
    query.addEventListener("change", onChange);
    return () => query.removeEventListener("change", onChange);
  }, []);

  const setTheme = useCallback((next: Theme) => {
    document.documentElement.dataset.theme = next;
    try {
      window.localStorage.setItem(storageKey, next);
    } catch {
      // Storage unavailable: the choice still applies for this session.
    }
    setThemeState(next);
  }, []);

  return [theme, setTheme];
}
