import { CloseOutlined } from "../ui/icons";
import { Button, Drawer } from "../ui";
import type { ReactNode } from "react";

/**
 * The phone's one bottom sheet for a short decision: a rounded top edge, a
 * discreet drag handle, a title with a close control, and the options below.
 * `CreateActionMenu` and `RecordMenu` both open it, so "pick one of these"
 * looks and behaves the same whichever page it comes from. (Long, scrolling
 * sheets such as the filter panel are deliberately handle-free and keep their
 * own drawer.)
 */
export function BottomSheet({
  open,
  onClose,
  title,
  children,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
}) {
  return (
    <Drawer
      open={open}
      onClose={onClose}
      placement="bottom"
      height="auto"
      closable={false}
      destroyOnHidden
      className="bottom-sheet"
      title={
        <>
          <div className="bottom-sheet-handle" aria-hidden="true" />
          <div className="bottom-sheet-header">
            <h2 className="bottom-sheet-title">{title}</h2>
            <Button
              type="text"
              className="bottom-sheet-close"
              icon={<CloseOutlined aria-hidden="true" />}
              aria-label={`Fechar ${title.toLocaleLowerCase("pt-BR")}`}
              onClick={onClose}
            />
          </div>
        </>
      }
    >
      {children}
    </Drawer>
  );
}
