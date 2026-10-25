import { EllipsisOutlined } from "../ui/icons";
import { Button, Dropdown } from "../ui";
import type { ItemType } from "../ui/menu";
import { useState } from "react";

import { BottomSheet } from "./BottomSheet";
import { useCompactScreen } from "./useCompactScreen";

export interface RecordMenuItem {
  key: string;
  label: string;
  danger?: boolean;
  disabled?: boolean;
  onClick?: () => void;
  /** A submenu (e.g. the goals a position can move to). */
  children?: RecordMenuItem[];
}

interface RecordMenuProps {
  /** The trigger's accessible name, and the phone sheet's title ("Ações de Conta Corrente"). */
  label: string;
  items: RecordMenuItem[];
  /** Spinner in place of the dots while one of the actions is running. */
  loading?: boolean;
  /** Extra class for a page-specific tweak (hover reveal, alignment). */
  className?: string;
}

function toItem(item: RecordMenuItem): ItemType {
  return {
    key: item.key,
    label: item.label,
    danger: item.danger,
    disabled: item.disabled,
    onClick: item.onClick,
    children: item.children?.map(toItem),
  } as ItemType;
}

/**
 * The record's secondary actions behind one `···` — Editar, Excluir,
 * Corrigir — instead of a coloured link or a row of small buttons on every
 * line. From `md` up it is a dropdown next to the trigger; on a phone, where
 * a tiny popover is hard to hit, it is a bottom sheet with 48px rows (the
 * same split `CreateActionMenu` makes). The trigger is a sibling of the row's
 * content, never nested inside another control, and keeps a 44px hit area
 * around its 32px look. Pair a destructive item with `useConfirm`.
 */
export function RecordMenu({ label, items, loading, className }: RecordMenuProps) {
  const compact = useCompactScreen();
  const [open, setOpen] = useState(false);
  if (items.length === 0) return null;

  const classes = ["record-menu", className].filter(Boolean).join(" ");
  const trigger = (
    <Button
      type="text"
      size="small"
      className={classes}
      icon={<EllipsisOutlined aria-hidden="true" />}
      loading={loading}
      aria-label={label}
      onClick={compact ? () => setOpen(true) : undefined}
    />
  );

  if (!compact) {
    return (
      <Dropdown menu={{ items: items.map(toItem) }} trigger={["click"]} placement="bottomRight">
        {trigger}
      </Dropdown>
    );
  }

  const choose = (item: RecordMenuItem) => {
    setOpen(false);
    item.onClick?.();
  };
  const row = (item: RecordMenuItem) => (
    <li key={item.key}>
      <button
        type="button"
        className={item.danger ? "record-menu-row record-menu-row-danger" : "record-menu-row"}
        disabled={item.disabled}
        onClick={() => choose(item)}
      >
        {item.label}
      </button>
    </li>
  );

  return (
    <>
      {trigger}
      <BottomSheet open={open} onClose={() => setOpen(false)} title={label}>
        <ul className="bottom-sheet-list record-menu-list">
          {items.map((item) =>
            item.children !== undefined ? (
              <li key={item.key} className="record-menu-group">
                <span className="record-menu-group-label">{item.label}</span>
                <ul className="record-menu-list">{item.children.map(row)}</ul>
              </li>
            ) : (
              row(item)
            ),
          )}
        </ul>
      </BottomSheet>
    </>
  );
}
