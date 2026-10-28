import type { ReactNode } from "react";
import { useId } from "react";
import { Link } from "react-router-dom";

interface HomeTileProps {
  label: string;
  /** Makes the whole tile one link; omit it while the tile shows a loading/error state (a retry button cannot live inside a link). */
  to?: string;
  className?: string;
  children: ReactNode;
}

/**
 * A Home surface without a header row: a quiet label above its figure. Same
 * surface as `Section` (hairline, edge-to-edge on a phone). As a link, the
 * label is plain text inside it; otherwise it is the tile's heading.
 */
export function HomeTile({ label, to, className, children }: HomeTileProps) {
  const labelId = useId();
  const classes = ["section", "home-tile", to !== undefined ? "is-link" : "", className].filter(Boolean).join(" ");
  if (to !== undefined) {
    return (
      <Link to={to} className={classes}>
        <span className="home-tile-body">
          <span className="home-tile-label">{label}</span>
          {children}
        </span>
        {/* Only drawn where the cards are a list of rows (a phone). */}
        <span className="home-tile-chevron" aria-hidden="true">
          ›
        </span>
      </Link>
    );
  }
  return (
    <section className={classes} aria-labelledby={labelId}>
      <h2 id={labelId} className="home-tile-label">
        {label}
      </h2>
      {children}
    </section>
  );
}
