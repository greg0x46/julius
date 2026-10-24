import { Card } from "../ui";
import type { ReactNode } from "react";

interface DataCardProps {
  /** The summary strip at the top of the card: counts and totals of what is listed. */
  summary?: ReactNode;
  className?: string;
  /** Drops the body padding so a table can run edge to edge. */
  flush?: boolean;
  children: ReactNode;
}

/**
 * The surface a collection lives on: a summary of the data on top, the
 * listing below, and nothing else — no controls, no creation buttons. What
 * shapes the data sits in the ListToolbar right above; what creates data
 * sits in the Page header.
 */
export function DataCard({ summary, className, flush, children }: DataCardProps) {
  return (
    <Card
      className={["data-card", flush ? "data-card-flush" : "", className].filter(Boolean).join(" ")}
      styles={{ body: { padding: 0 } }}
    >
      {summary}
      <div className="data-card-body">{children}</div>
    </Card>
  );
}

interface DataCardSummaryProps {
  label: string;
  busy?: boolean;
  /** Small print under the figures, e.g. what the totals do and do not include. */
  note?: ReactNode;
  children: ReactNode;
}

/**
 * The header of a DataCard. It stays stuck under the app bar while the list
 * scrolls, so the totals are always one glance away.
 */
export function DataCardSummary({ label, busy, note, children }: DataCardSummaryProps) {
  return (
    <section className="data-card-summary" aria-label={label} aria-busy={busy}>
      {children}
      {note !== undefined && <p className="data-card-summary-note">{note}</p>}
    </section>
  );
}
