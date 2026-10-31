import { useTranslation } from "../../i18n/context";
import { CloseOutlined, LeftOutlined, RightOutlined } from "../ui/icons";
import { Button, Collapse, Drawer, Typography } from "../ui";
import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

import { useCompactScreen } from "./useCompactScreen";

const FooterSlotContext = createContext<HTMLElement | null>(null);

/** One visible screen per panel, with a header and a shared footer slot. */
export function PanelStack({
  open,
  onClose,
  title,
  onBack,
  extra,
  width = 480,
  children,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  /** Present on every screen but the root: the ← control pops one level. */
  onBack?: () => void;
  /** Right-hand slot of the header, e.g. an overflow menu. */
  extra?: ReactNode;
  width?: number;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const compact = useCompactScreen();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const [footerSlot, setFooterSlot] = useState<HTMLElement | null>(null);
  const isRoot = onBack === undefined;

  // Moving between screens replaces the whole body; sending focus to the
  // new title tells assistive tech (and the keyboard) where they are now.
  useEffect(() => {
    if (open) headingRef.current?.focus();
  }, [open, title]);

  // The root of a full-screen page reads as "back to the list", so it gets the
  // same chevron as `Page`'s back link (and as every Voltar inside the stack);
  // a side panel's root is dismissed, so it gets the usual ×.
  const dismissIcon = compact ? <LeftOutlined aria-hidden="true" /> : <CloseOutlined aria-hidden="true" />;

  return (
    <Drawer
      open={open}
      onClose={onClose}
      placement="right"
      width={compact ? "100%" : width}
      closable={false}
      destroyOnHidden
      push={false}
      className={`panel-stack ${compact ? "panel-stack-compact" : ""}`}
      title={
        <div className="panel-stack-header">
          <Button
            type="text"
            className="panel-stack-back"
            icon={isRoot ? dismissIcon : <LeftOutlined aria-hidden="true" />}
            aria-label={isRoot ? t("close") : t("back")}
            onClick={isRoot ? onClose : onBack}
          />
          <h2 ref={headingRef} tabIndex={-1} className="panel-stack-title">
            {title}
          </h2>
          <div className="panel-stack-extra">{extra}</div>
        </div>
      }
      // The slot is always rendered; a screen fills it through PanelFooter
      // and CSS hides it while empty, so the body never jumps between screens.
      footer={<div className="panel-stack-footer" ref={setFooterSlot} />}
    >
      <FooterSlotContext.Provider value={footerSlot}>
        <div className="panel-stack-body">{children}</div>
      </FooterSlotContext.Provider>
    </Drawer>
  );
}

/**
 * The sticky action bar of the current screen. Rendered from inside the
 * screen (where the action's state lives) into the shell's footer slot.
 */
export function PanelFooter({ children }: { children: ReactNode }) {
  const slot = useContext(FooterSlotContext);
  // Outside a PanelStack (a screen rendered on its own) the bar simply
  // follows the content.
  return slot ? createPortal(children, slot) : <div className="panel-stack-footer">{children}</div>;
}

/** A titled block of the body; the title is the small uppercase eyebrow used across the app's detail views. */
export function PanelSection({
  title,
  children,
  className = "",
}: {
  title?: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={`panel-section ${className}`}>
      {title && <h3 className="panel-section-title">{title}</h3>}
      {children}
    </section>
  );
}

/**
 * A full-width row that leads to another screen of the stack. The chevron
 * says "this navigates" — anything that acts in place is a button, not a row.
 */
export function PanelNavRow({
  icon,
  label,
  hint,
  onClick,
  disabled = false,
}: {
  icon?: ReactNode;
  label: string;
  /** Secondary line: current state ("R$ 600,00 vinculados") or why it is disabled. */
  hint?: ReactNode;
  onClick: () => void;
  disabled?: boolean;
}) {
  return (
    <button type="button" className="panel-nav-row" onClick={onClick} disabled={disabled}>
      {icon && (
        <span className="panel-nav-row-icon" aria-hidden="true">
          {icon}
        </span>
      )}
      <span className="panel-nav-row-text">
        <span className="panel-nav-row-label">{label}</span>
        {hint && <span className="panel-nav-row-hint">{hint}</span>}
      </span>
      <RightOutlined className="panel-nav-row-chevron" aria-hidden="true" />
    </button>
  );
}

/**
 * Progressive disclosure for explanatory text: a one-line summary that
 * stays, and the full explanation behind "Saiba mais".
 */
export function PanelDisclosure({
  summary,
  children,
  label,
}: {
  summary: ReactNode;
  children: ReactNode;
  label?: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="panel-disclosure">
      <Typography.Text type="secondary">{summary}</Typography.Text>
      <Collapse
        ghost
        size="small"
        className="panel-disclosure-more"
        items={[
          {
            key: "more",
            label: label ?? t("learnMore"),
            children: <Typography.Paragraph type="secondary">{children}</Typography.Paragraph>,
          },
        ]}
      />
    </div>
  );
}
