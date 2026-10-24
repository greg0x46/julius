import { CloseOutlined } from "../ui/icons";
import { Button, Drawer } from "../ui";
import { useEffect, useRef, type ReactNode } from "react";

import { useCompactScreen } from "../shared/useCompactScreen";

/**
 * The shell of a set of filters: a header that says how many are active, a
 * body that scrolls, and a sticky bar with the two actions every filter set
 * has. From `md` up it is a side drawer, so the list it filters stays in
 * view; on a phone it is a tall bottom sheet rather than a squeezed drawer.
 *
 * `activeCount` is the caller's — the same number its toolbar button shows —
 * so the panel never counts on its own.
 */
export function FilterPanel({
  open,
  onClose,
  title = "Filtros",
  activeCount,
  onClear,
  onApply,
  clearDisabled = false,
  width = 380,
  children,
}: {
  open: boolean;
  onClose: () => void;
  title?: string;
  activeCount: number;
  onClear: () => void;
  onApply: () => void;
  clearDisabled?: boolean;
  width?: number;
  children: ReactNode;
}) {
  const compact = useCompactScreen();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const status =
    activeCount === 0 ? "Nenhum ativo" : activeCount === 1 ? "1 ativo" : `${activeCount} ativos`;

  // Opening replaces what the person was looking at; the title tells
  // assistive tech (and the keyboard) where they are.
  useEffect(() => {
    if (open) headingRef.current?.focus();
  }, [open]);

  return (
    <Drawer
      open={open}
      onClose={onClose}
      placement={compact ? "bottom" : "right"}
      width={compact ? undefined : width}
      height={compact ? "92dvh" : undefined}
      closable={false}
      destroyOnHidden
      className={`filter-panel ${compact ? "filter-panel-compact" : ""}`}
      title={
        <div className="filter-panel-header">
          <div className="filter-panel-heading">
            <h2 ref={headingRef} tabIndex={-1} className="filter-panel-title">
              {title}
            </h2>
            <span className={`filter-panel-status ${activeCount ? "is-active" : ""}`} aria-live="polite">
              {status}
            </span>
          </div>
          <Button
            type="text"
            className="filter-panel-close"
            icon={<CloseOutlined />}
            aria-label="Fechar filtros"
            onClick={onClose}
          />
        </div>
      }
      footer={
        <div className="filter-panel-footer">
          <Button type="link" className="filter-panel-clear" disabled={clearDisabled} onClick={onClear}>
            Limpar filtros
          </Button>
          <Button type="primary" onClick={onApply}>
            Aplicar
          </Button>
        </div>
      }
    >
      <div className="filter-panel-body">{children}</div>
    </Drawer>
  );
}

/**
 * A semantic group of the panel: a discreet uppercase eyebrow and the
 * controls that belong together. Groups are separated by space, not boxes.
 */
export function FilterSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="filter-section" aria-label={title}>
      <h3 className="filter-section-title">{title}</h3>
      <div className="filter-section-body">{children}</div>
    </section>
  );
}

/**
 * A labelled control of a section: the label above, the control below.
 * `hideLabel` keeps the name for assistive tech when the section title
 * already says it.
 */
export function FilterField({
  id,
  label,
  hideLabel = false,
  children,
}: {
  id: string;
  label: string;
  hideLabel?: boolean;
  children: ReactNode;
}) {
  return (
    <div className="filter-field">
      <label htmlFor={id} className={hideLabel ? "visually-hidden" : undefined}>
        {label}
      </label>
      {children}
    </div>
  );
}
