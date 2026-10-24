import { Dropdown } from "../ui";
import type { ItemType } from "../ui/menu";
import type { ReactNode } from "react";
import { useState } from "react";

import { BottomSheet } from "../shared/BottomSheet";
import { PanelNavRow } from "../shared/PanelStack";
import { useCompactScreen } from "../shared/useCompactScreen";
import { PageAction } from "./PageAction";

export interface CreateActionOption {
  key: string;
  label: string;
  /** Quiet, one-line context under the label — what the option is for, not how to use it. */
  description?: string;
  /** A small mark for the option's kind, shown on the phone sheet only. */
  icon?: ReactNode;
  onClick: () => void;
  disabled?: boolean;
}

interface CreateActionMenuProps {
  /** The trigger's own label, e.g. "Nova pendência" — one thing that groups the options. */
  label: string;
  /** What a phone shows on the trigger instead ("Nova"); must be contained in `label`. */
  shortLabel?: string;
  options: CreateActionOption[];
}

/**
 * A single compact trigger for several equally-weighted creation options —
 * the alternative to stacking one big button per option (which manufactures
 * a hierarchy between choices that don't have one). From `md` up it opens a
 * dropdown menu next to the trigger; on a phone it opens a bottom sheet,
 * the same split `FilterPanel` uses for its own drawer-vs-sheet choice.
 *
 * The trigger is a `PageAction`, so it sits in the title row and turns
 * compact (icon plus `shortLabel`) on a phone like every other page action.
 */
export function CreateActionMenu({ label, shortLabel, options }: CreateActionMenuProps) {
  const compact = useCompactScreen();
  const [open, setOpen] = useState(false);

  const trigger = (
    <PageAction
      className="create-action-trigger"
      label={label}
      shortLabel={shortLabel}
      onClick={compact ? () => setOpen(true) : undefined}
    />
  );

  if (!compact) {
    const items: ItemType[] = options.map((option) => ({
      key: option.key,
      disabled: option.disabled,
      label: (
        <span className="create-action-menu-option">
          <span className="create-action-menu-option-label">{option.label}</span>
          {option.description && (
            <span className="create-action-menu-option-description">{option.description}</span>
          )}
        </span>
      ),
      onClick: option.onClick,
    }));
    return (
      <Dropdown menu={{ items }} trigger={["click"]} placement="bottomRight">
        {trigger}
      </Dropdown>
    );
  }

  return (
    <>
      {trigger}
      <BottomSheet open={open} onClose={() => setOpen(false)} title={label}>
        <ul className="bottom-sheet-list">
          {options.map((option) => (
            <li key={option.key}>
              <PanelNavRow
                icon={option.icon}
                label={option.label}
                hint={option.description}
                disabled={option.disabled}
                onClick={() => {
                  setOpen(false);
                  option.onClick();
                }}
              />
            </li>
          ))}
        </ul>
      </BottomSheet>
    </>
  );
}
