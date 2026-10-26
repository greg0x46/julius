import type { ReactNode } from "react";
import { useId } from "react";

export interface SummaryStripItem {
  label: string;
  value: ReactNode;
}

interface SummaryStripProps {
  /** What the hero figure is ("Saldo em contas"). */
  label: string;
  /** The page's one hero figure. It stays on one line (the font scales down on a very narrow phone) and is never truncated. */
  value: ReactNode;
  /** Small print after the figures, on a line of its own, e.g. what the totals do and do not include. Text only. */
  note?: ReactNode;
  /** What belongs with the hero but is not text — a progress meter, a status tag. Stacked under the hero, start-aligned. */
  children?: ReactNode;
  /** Secondary figures, shown as a definition strip beside the hero from 1200px, under it below that. */
  items?: SummaryStripItem[];
  busy?: boolean;
  className?: string;
}

/**
 * The summary at the top of a page: one hero figure and, optionally, a few
 * secondary figures. It is the shared shape behind what `.accounts-summary`,
 * `.net-worth-summary` and `.investments-summary` each draw by hand. Pass
 * money through the `Money` component (or any node) as `value`; the strip
 * only lays the figures out.
 */
export function SummaryStrip({ label, value, note, children, items, busy, className }: SummaryStripProps) {
  const labelId = useId();
  return (
    <section
      className={["summary-strip", className].filter(Boolean).join(" ")}
      aria-labelledby={labelId}
      aria-busy={busy}
    >
      <div className="summary-strip-hero">
        <span id={labelId} className="summary-strip-label">
          {label}
        </span>
        <div className="summary-strip-value">{value}</div>
        {children !== undefined && <div className="summary-strip-extra">{children}</div>}
      </div>
      {items !== undefined && items.length > 0 && (
        <dl className="summary-strip-items">
          {items.map((item) => (
            <div key={item.label} className="summary-strip-item">
              <dt>{item.label}</dt>
              <dd>{item.value}</dd>
            </div>
          ))}
        </dl>
      )}
      {note !== undefined && <p className="summary-strip-note">{note}</p>}
    </section>
  );
}
