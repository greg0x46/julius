import type { TimelineDayPoint, TimelineEntry } from "../api/contracts";
import { formatDayMonth } from "./dates";
import { compareDecimals, sumDecimals } from "./decimal";

export type ChartEventKind = "bill" | "installment" | "recurring";

export interface ChartEvent {
  kind: ChartEventKind;
  date: string;
  /** Short text for the marker: what it is and the day, e.g. "Fatura 25/10". */
  label: string;
  /** Exact net amount of the event's entries on that day (BRL/reference currency, signed). */
  amount: string;
  /** The server's end-of-day balance on that day: where the marker sits on the line. */
  balance: string;
}

const maxDescription = 14;

function kindOf(entry: TimelineEntry): ChartEventKind | null {
  if (entry.tier !== "confirmado" && entry.tier !== "projetado") return null;
  // A card purchase is a real entry re-dated to its bill's due day, so the
  // entries sharing that day are one bill.
  if (entry.source === "real" && entry.tier === "confirmado") return "bill";
  if (entry.source === "plano_pagamento") return "installment";
  if (entry.source === "recorrente") return "recurring";
  return null;
}

function shorten(text: string): string {
  const trimmed = text.trim();
  return trimmed.length > maxDescription ? `${trimmed.slice(0, maxDescription - 1).trimEnd()}…` : trimmed;
}

function labelOf(kind: ChartEventKind, description: string): string {
  if (kind === "bill") return "Fatura";
  if (kind === "installment") return "Parcela";
  return shorten(description) || "Recorrência";
}

const magnitude = (value: string) => (value.startsWith("-") ? value.slice(1) : value);

/**
 * The few future events worth naming on the balance curve: card bills (the
 * purchases due the same day, summed), payment-plan installments and
 * recurrences — the Confirmado and Projetado tiers only, strictly after the
 * reference day. Ranked by the exact size of the money, at most one marker per
 * day, the biggest `limit` kept and returned in date order. Amounts are added
 * and compared as decimals, never as numbers. Days in `skipDates` (the first
 * negative day, which has a marker of its own) get no event marker.
 */
export function selectChartEvents(
  entries: TimelineEntry[],
  points: Pick<TimelineDayPoint, "date" | "balance">[],
  referenceDate: string,
  limit: number,
  skipDates: string[] = [],
): ChartEvent[] {
  const balanceByDate = new Map(points.map((point) => [point.date, point.balance]));
  const groups = new Map<string, { kind: ChartEventKind; date: string; description: string; amounts: string[] }>();
  for (const entry of entries) {
    const kind = kindOf(entry);
    if (kind === null || entry.date <= referenceDate || skipDates.includes(entry.date) || !balanceByDate.has(entry.date)) continue;
    // Bills and installments add up per day; each recurrence stays its own event.
    const key = kind === "recurring" ? `${kind}|${entry.date}|${entry.description}` : `${kind}|${entry.date}`;
    const group = groups.get(key);
    if (group) group.amounts.push(entry.amount);
    else groups.set(key, { kind, date: entry.date, description: entry.description, amounts: [entry.amount] });
  }

  const candidates = [...groups.values()]
    .map((group) => ({ ...group, amount: sumDecimals(group.amounts) }))
    .filter((group) => compareDecimals(group.amount, "0") !== 0)
    .map((group) => ({ ...group, label: labelOf(group.kind, group.description) }))
    .sort(
      (a, b) =>
        compareDecimals(magnitude(b.amount), magnitude(a.amount)) ||
        a.date.localeCompare(b.date) ||
        a.label.localeCompare(b.label),
    );

  const chosen: typeof candidates = [];
  const usedDates = new Set<string>();
  for (const candidate of candidates) {
    if (chosen.length >= limit) break;
    if (usedDates.has(candidate.date)) continue;
    usedDates.add(candidate.date);
    chosen.push(candidate);
  }

  return chosen
    .sort((a, b) => a.date.localeCompare(b.date))
    .map((event) => ({
      kind: event.kind,
      date: event.date,
      label: `${event.label} ${formatDayMonth(event.date)}`,
      amount: event.amount,
      balance: balanceByDate.get(event.date) as string,
    }));
}
