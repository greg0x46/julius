import type { InvestmentPeriod, NetWorthSnapshot } from "../../api/contracts";
import { hasNetWorthValue } from "../../api/contracts";
import { changePercent } from "../../presentation/decimal";
import type { EvolutionScale } from "./EvolutionControls";

export type EvolutionPoint = { date: string; values: Record<string, string | null>; notes?: string[] };
export type InvestmentHistory = { baseline: InvestmentPeriod; periods: InvestmentPeriod[] };

export function reliablePortfolio(period: InvestmentPeriod): boolean {
  return period.totals.positions_total > 0 && !period.totals.partial && period.totals.positions_partial_start === 0 &&
    period.groups.every((group) => group.positions.every((position) => position.history_complete && !position.unavailable_reason));
}

export function portfolioStale(period: InvestmentPeriod): boolean {
  return period.fx.stale || Object.values(period.native_rates).some((rate) => rate.stale);
}

export function wealthPoints(snapshots: NetWorthSnapshot[], from: string, to: string, history?: InvestmentHistory): EvolutionPoint[] {
  const points = new Map<string, EvolutionPoint>();
  // Explicit boundary gaps prevent a later observation becoming an invented baseline.
  points.set(from, { date: from, values: {} });
  points.set(to, { date: to, values: {} });
  for (const snapshot of snapshots) {
    const date = snapshot.captured_at.slice(0, 10);
    if (date < from || date > to) continue;
    const observed = hasNetWorthValue(snapshot) && !snapshot.is_backfilled &&
      (snapshot.valuation_status === "complete" || snapshot.valuation_status === "stale");
    const stale = snapshot.valuation_status === "stale" || snapshot.fx?.stale ||
      Object.values(snapshot.valuation?.rates ?? {}).some((rate) => rate.stale);
    points.set(date, { date, values: { netWorth: observed ? snapshot.net_worth! : null, cash: observed ? snapshot.cash_balance! : null },
      notes: [observed ? (stale ? "Patrimônio e saldo: câmbio desatualizado" : "") : "Patrimônio e saldo: avaliação ausente, antiga ou reconstruída"].filter(Boolean) });
  }
  if (history) for (const period of [history.baseline, ...history.periods]) {
    if (period.to < from || period.to > to) continue;
    const point = points.get(period.to) ?? { date: period.to, values: {}, notes: [] };
    point.values.investments = reliablePortfolio(period) ? period.totals.end_value : null;
    if (!reliablePortfolio(period)) point.notes = [...point.notes ?? [], "Investimentos: histórico insuficiente ou parcial"];
    else if (portfolioStale(period)) point.notes = [...point.notes ?? [], "Investimentos: câmbio desatualizado"];
    points.set(period.to, point);
  }
  // Include missing calendar days: never bridge disconnected observations.
  const start = new Date(`${from}T00:00:00Z`).getTime();
  const end = new Date(`${to}T00:00:00Z`).getTime();
  for (let time = start; time <= end; time += 86400000) {
    const date = new Date(time).toISOString().slice(0, 10);
    if (!points.has(date)) points.set(date, { date, values: {} });
  }
  return [...points.values()].sort((a, b) => a.date.localeCompare(b.date));
}

export function scalePoints(points: EvolutionPoint[], selected: string[], scale: EvolutionScale) {
  const baseline = points[0];
  const invalid = scale === "percent" ? selected.filter((key) => {
    const value = baseline?.values[key];
    return value == null || changePercent(value, value) === null;
  }) : [];
  return { invalid, data: points.map((point): Record<string, string | number | null> & { date: string } => ({ date: point.date, ...Object.fromEntries(selected.map((key) => {
    const value = point.values[key];
    return [key, value == null || invalid.includes(key) ? null : Number(scale === "absolute" ? value : changePercent(value, baseline.values[key]!))];
  })) })) };
}
