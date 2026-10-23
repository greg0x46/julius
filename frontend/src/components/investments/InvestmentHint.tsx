import { Popover } from "../ui";
import type { ReactNode } from "react";

/**
 * A figure or tag whose explanation opens on hover, tap or from the keyboard
 * (a hover-only tooltip is unreachable on a phone). `label` is the accessible
 * name and carries the explanation too, so a screen reader gets it without
 * opening the popover.
 */
export function InvestmentHint({
  label,
  content,
  children,
}: {
  label: string;
  content: ReactNode;
  children: ReactNode;
}) {
  return (
    <Popover content={<div className="investment-hint-content">{content}</div>} trigger={["hover", "click"]} placement="bottomLeft">
      <button type="button" className="investment-hint" aria-label={label}>
        {children}
      </button>
    </Popover>
  );
}
