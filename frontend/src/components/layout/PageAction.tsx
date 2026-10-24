import { PlusOutlined } from "../ui/icons";
import { Button } from "../ui";
import type { ButtonProps } from "../ui";
import type { ReactNode } from "react";
import { forwardRef } from "react";

import { useCompactScreen } from "../shared/useCompactScreen";

export interface PageActionProps extends Omit<ButtonProps, "type" | "size" | "children" | "icon"> {
  /** Defaults to a plus: the usual page action creates something. */
  icon?: ReactNode;
  /** The full name ("Nova transação"), shown from `md` up and kept as the accessible name. */
  label: string;
  /** What a phone shows instead ("Nova"); must be contained in `label`. Defaults to `label`. */
  shortLabel?: string;
}

/**
 * A page's primary action as it sits in the title row: a filled button from
 * `md` up, and on a phone a compact 40px text button — icon plus a short
 * label — so the title row stays one quiet line instead of growing a green
 * bar. Page-wide commands only; a record's own actions stay with the record.
 */
export const PageAction = forwardRef<HTMLButtonElement, PageActionProps>(function PageAction(
  { icon = <PlusOutlined aria-hidden="true" />, label, shortLabel, className, ...rest },
  ref,
) {
  const compact = useCompactScreen();
  const short = compact && shortLabel !== undefined && shortLabel !== label;
  return (
    <Button
      {...rest}
      ref={ref}
      type={compact ? "text" : "primary"}
      className={["page-action", className].filter(Boolean).join(" ")}
      icon={icon}
      aria-label={short ? label : rest["aria-label"]}
    >
      {short ? shortLabel : label}
    </Button>
  );
});
