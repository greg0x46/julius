import { Grid } from "../ui";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";

import { useCompactScreen } from "../shared/useCompactScreen";
import { EmptyState } from "./EmptyState";

export interface ListRowProps {
  /** What the row is: the description, dominant (500). */
  title: ReactNode;
  /** One lighter line under the title: date, account, category. */
  meta?: ReactNode;
  /** The row's figure, right-aligned and tabular — usually a `Money`. */
  trailing?: ReactNode;
  /**
   * Only a non-default state ("Atrasada", "Quitada") — never a tag on every
   * row for the common case. Sits under the trailing figure.
   */
  status?: ReactNode;
  /** Makes the whole row a button. Mutually exclusive with `href`. */
  onClick?: () => void;
  /** Makes the whole row a link to an in-app route. */
  href?: string;
  /**
   * The interactive row's accessible name ("Abrir Conta Corrente"); give it
   * whenever there is an `onClick` or `href`. A static row is plain text and
   * needs none.
   */
  ariaLabel?: string;
  /** Keep title and meta to one line each, cut with an ellipsis (names and institutions that can run long). */
  truncate?: boolean;
  className?: string;
}

/**
 * One line of a stacked list: title and meta on the left, figure and status
 * on the right, the whole row one tap target (a button or a link — never
 * nested controls). Render it inside a `<ul className="list-rows">` or
 * through `ResponsiveList`. Record actions (Editar, Excluir) do not belong on
 * the row: they live in the record's own detail or panel.
 */
export function ListRow({
  title,
  meta,
  trailing,
  status,
  onClick,
  href,
  ariaLabel,
  truncate,
  className,
}: ListRowProps) {
  const body = (
    <>
      <span className="list-row-text">
        <span className="list-row-title">{title}</span>
        {meta !== undefined && <span className="list-row-meta">{meta}</span>}
      </span>
      {(trailing !== undefined || status !== undefined) && (
        <span className="list-row-side">
          {trailing !== undefined && <span className="list-row-trailing">{trailing}</span>}
          {status !== undefined && <span className="list-row-status">{status}</span>}
        </span>
      )}
    </>
  );
  const classes = ["list-row", truncate && "list-row-truncate", className].filter(Boolean).join(" ");

  return (
    <li className="list-row-item">
      {href !== undefined ? (
        <Link to={href} className={classes} aria-label={ariaLabel}>
          {body}
        </Link>
      ) : onClick !== undefined ? (
        <button type="button" className={classes} aria-label={ariaLabel} onClick={onClick}>
          {body}
        </button>
      ) : (
        <div className={classes}>{body}</div>
      )}
    </li>
  );
}

interface ResponsiveListProps<T> {
  /** The list's accessible name ("Contas bancárias"). */
  label: string;
  items: T[];
  getKey: (item: T) => string;
  /** How one item reads as a stacked row on a phone. */
  row: (item: T) => Omit<ListRowProps, "className">;
  /** What a wide screen shows instead: the table (or any richer layout) for the same items. */
  wide: ReactNode;
  isLoading?: boolean;
  /** Skeleton or spinner shown while loading, on both layouts. */
  loading?: ReactNode;
  /** Shown instead of either layout when there are no items. */
  empty?: ReactNode;
  /**
   * Below which breakpoint the list stacks. `"md"` (default, 768px) is the
   * phone. A table with many columns (Recorrências, Automações, Pendências)
   * is squeezed or clipped well above that, so those pages pass `"lg"`
   * (992px) or, with the 215px sidebar beside them, `"xl"` (1200px), and keep
   * the table only where it has room.
   */
  stackBelow?: "md" | "lg" | "xl";
}

/**
 * Sideways-scrolling tables do not work on a phone, so a collection that is
 * a table on a wide screen becomes a stack of `ListRow`s below `md`. The
 * wide layout is passed in as-is — this helper only decides which of the two
 * to mount and owns the loading and empty states, so a list never
 * implements the split by hand. Generalises what `CompactPayableList` did.
 */
export function ResponsiveList<T>({
  label,
  items,
  getKey,
  row,
  wide,
  isLoading,
  loading,
  empty,
  stackBelow = "md",
}: ResponsiveListProps<T>) {
  const compact = useCompactScreen();
  // `lg` is undefined before the first media-query tick: that counts as
  // stacked, like `useCompactScreen`, so a phone never flashes the table.
  const screens = Grid.useBreakpoint();
  const stacked =
    stackBelow === "xl" ? !screens.xl : stackBelow === "lg" ? !screens.lg : compact;
  if (isLoading) return <>{loading}</>;
  if (items.length === 0) return <>{empty ?? <EmptyState title="Nada por aqui ainda" />}</>;
  if (!stacked) return <>{wide}</>;
  return (
    <ul className="list-rows" aria-label={label}>
      {items.map((item) => (
        <ListRow key={getKey(item)} {...row(item)} />
      ))}
    </ul>
  );
}
