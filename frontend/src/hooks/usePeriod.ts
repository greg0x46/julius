import { useState } from "react";
import { periodPresets } from "../components/filters/periodPresets";
import { periodNavigation } from "../components/filters/periodNavigation";
import { useTimezone } from "./useTimezone";

/** A window, in the same shape the transactions filter uses: nulls mean "all of it". */
export type Period = { from: string; to: string } | { from: null; to: null };

const storageKey = "julius.periodo";
const defaultPreset = "this-month";

export function toPeriod(from: string | null, to: string | null): Period {
  return from !== null && to !== null ? { from, to } : { from: null, to: null };
}

function presetPeriod(value: string, timezone: string): Period | null {
  const preset = periodPresets(timezone).find((candidate) => candidate.value === value);
  if (!preset) return null;
  const [from, to] = preset.range();
  return toPeriod(from, to);
}

export function defaultPeriod(timezone: string): Period {
  return presetPeriod(defaultPreset, timezone) ?? { from: null, to: null };
}

/** A window is well-formed when it is either unbounded or a navigable from/to pair. */
export function isValidPeriod(period: Period): boolean {
  if (period.from === null) return true;
  return period.from >= "0001-01-01" && periodNavigation(period.from, period.to) !== null;
}

/**
 * The chosen window is the page context shared by every screen that reads
 * a period (Home, Transações): picking "últimos 30 dias" on one must still be
 * the window when the next one opens. It is a per-browser preference, not
 * part of any page's address, so it lives in localStorage. Reading and
 * writing it can throw outright — private browsing, or a browser set to block
 * site data — so neither side may be the thing that breaks a page.
 *
 * A window that came from a shortcut is stored as the shortcut and resolved
 * again on the way back in: "este ano" saved in December must still mean this
 * year in January, not the year it was picked.
 */
export function storedPeriod(timezone: string): Period {
  let raw: string | null;
  try {
    raw = window.localStorage.getItem(storageKey);
  } catch {
    return defaultPeriod(timezone);
  }
  if (raw === null) return defaultPeriod(timezone);
  try {
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== "object" || parsed === null) return defaultPeriod(timezone);
    const { preset, from, to } = parsed as Record<string, unknown>;
    if (typeof preset === "string") return presetPeriod(preset, timezone) ?? defaultPeriod(timezone);
    if (typeof from === "string" && typeof to === "string") {
      const period = { from, to };
      if (isValidPeriod(period)) return period;
    }
  } catch {
    // Ignore malformed browser preferences.
  }
  return defaultPeriod(timezone);
}

export function storePeriod(period: Period, timezone: string): void {
  const preset = periodPresets(timezone).find((candidate) => {
    const [from, to] = candidate.range();
    return period.from === from && period.to === to;
  });
  try {
    window.localStorage.setItem(
      storageKey,
      JSON.stringify(preset ? { preset: preset.value } : period),
    );
  } catch {
    // A window that can't be remembered is still a usable window.
  }
}

/**
 * The page's period context. `initial` lets a page that was opened through a
 * link carrying its own window (e.g. `/transacoes?period=all`) honour that
 * link and, from then on, make it the shared window too.
 */
export function usePeriod(initial?: () => Period | null) {
  const timezone = useTimezone();
  const [period, setPeriodState] = useState<Period>(() => {
    const fromCaller = initial?.() ?? null;
    if (fromCaller) {
      storePeriod(fromCaller, timezone);
      return fromCaller;
    }
    return storedPeriod(timezone);
  });
  const setPeriod = (from: string | null, to: string | null) => {
    const next = toPeriod(from, to);
    setPeriodState(next);
    storePeriod(next, timezone);
  };
  return { period, setPeriod, presets: periodPresets(timezone) };
}
