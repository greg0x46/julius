import { hasNetWorthValue, type NetWorthPoint } from "../api/contracts";
import { calendar } from "./calendar";
import { subtractDecimals } from "./decimal";

export interface NetWorthTrend {
  /** Calendar day ("YYYY-MM-DD") of the point the change is measured from. */
  since: string;
  /** Exact `latest - since` in the reference currency, signed. */
  change: string;
  /** Wealth of each compared day, oldest first, for drawing only. */
  values: number[];
}

/**
 * How wealth moved over the last `days` days. Every day is derived on read, so
 * the window is a plain date range; a day that could not be valued is skipped
 * rather than read as zero. Null when the latest day has no figures or no
 * earlier valued day exists in the window: no trend is better than a wrong one.
 */
export function netWorthTrend(series: NetWorthPoint[], latest: NetWorthPoint, days = 30): NetWorthTrend | null {
  if (!hasNetWorthValue(latest)) return null;
  const start = calendar(latest.date).add(-days, "day").format("YYYY-MM-DD");
  const window = series
    .filter(hasNetWorthValue)
    .filter((point) => point.date >= start && point.date <= latest.date)
    .sort((a, b) => a.date.localeCompare(b.date));
  const first = window[0];
  if (!first || first.date === latest.date) return null;
  return {
    since: first.date,
    change: subtractDecimals(latest.total, first.total),
    values: window.map((point) => Number(point.total)),
  };
}

/** SVG `points` for a line through `values`, scaled into a `width`×`height` box with `pad` on every side. */
export function sparklinePoints(values: number[], width: number, height: number, pad = 2): string {
  if (values.length === 0) return "";
  const min = Math.min(...values);
  const max = Math.max(...values);
  const span = max - min;
  const step = values.length > 1 ? (width - 2 * pad) / (values.length - 1) : 0;
  return values
    .map((value, index) => {
      const x = pad + index * step;
      // A flat series sits in the middle rather than on an edge.
      const y = span === 0 ? height / 2 : pad + (height - 2 * pad) * (1 - (value - min) / span);
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(" ");
}
