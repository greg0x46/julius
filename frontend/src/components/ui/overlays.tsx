import { FeedbackContext, type Confirm } from "./feedbackContext";
import {
  cloneElement,
  isValidElement,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type HTMLAttributes,
  type ReactElement,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { useFloatingSurface } from "./useFloatingSurface";
import { Button } from "./controls";
import { lockDialogScroll } from "./dialogLifecycle";
import type { ItemType } from "./menu";

export function Drawer({
  open,
  onClose,
  title,
  children,
  footer,
  extra,
  closable = true,
  placement = "right",
  width = 480,
  height,
  className = "",
  rootClassName = "",
  destroyOnHidden,
  push,
  styles,
}: {
  open?: boolean;
  onClose?: () => void;
  title?: ReactNode;
  children?: ReactNode;
  footer?: ReactNode;
  extra?: ReactNode;
  closable?: boolean;
  placement?: string;
  width?: string | number;
  height?: string | number;
  className?: string;
  rootClassName?: string;
  destroyOnHidden?: boolean;
  push?: boolean;
  styles?: { body?: CSSProperties };
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const heading = useId();
  const pressedBackdrop = useRef(false);
  useEffect(() => {
    const dialog = ref.current;
    if (!open || !dialog) return;
    const previous = document.activeElement as HTMLElement | null;
    dialog.showModal();
    const unlock = lockDialogScroll();
    return () => {
      dialog.close();
      unlock();
      if (previous?.isConnected) previous.focus();
    };
  }, [open]);
  void destroyOnHidden;
  void push;
  if (!open) return null;
  return createPortal(
    <dialog
      ref={ref}
      aria-labelledby={title ? heading : undefined}
      className={`ui-drawer ui-drawer-${placement} ${rootClassName} ${className}`}
      style={{
        width: placement === "bottom" ? "100%" : width,
        height: placement === "bottom" ? (height ?? "auto") : "100%",
      }}
      onCancel={(event) => {
        event.preventDefault();
        event.stopPropagation();
        onClose?.();
      }}
      onPointerDown={(event) => {
        pressedBackdrop.current = event.target === event.currentTarget;
      }}
      onClick={(event) => {
        // A drag that starts inside (text selection) must not dismiss.
        if (event.target !== event.currentTarget || !pressedBackdrop.current)
          return;
        const bounds = event.currentTarget.getBoundingClientRect();
        if (
          event.clientX < bounds.left ||
          event.clientX > bounds.right ||
          event.clientY < bounds.top ||
          event.clientY > bounds.bottom
        )
          onClose?.();
      }}
    >
      {(title || extra || closable) && (
        <header className="ui-drawer-header">
          <div id={heading} className="ui-drawer-title">
            {title}
          </div>
          {extra}
          {closable && (
            <Button type="text" aria-label="Fechar" onClick={onClose}>
              ×
            </Button>
          )}
        </header>
      )}
      <div className="ui-drawer-body" style={styles?.body}>
        {children}
      </div>
      {footer && <footer className="ui-drawer-footer">{footer}</footer>}
    </dialog>,
    document.body,
  );
}
interface PopoverProps {
  children: ReactNode;
  content?: ReactNode;
  title?: ReactNode;
  trigger?: string | string[];
  placement?: string;
  arrow?: boolean;
  destroyOnHidden?: boolean;
  classNames?: { root?: string };
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  overlayClassName?: string;
  haspopup?: "menu";
}
export function Popover({
  children,
  content,
  title,
  open: controlled,
  onOpenChange,
  overlayClassName,
  trigger,
  placement,
  classNames,
  haspopup,
}: PopoverProps) {
  const [local, setLocal] = useState(false);
  const open = controlled ?? local;
  const root = useRef<HTMLSpanElement>(null);
  const opener = useRef<HTMLElement | null>(null);
  const id = useId();
  const surface = useRef<HTMLDivElement>(null);
  const position = useFloatingSurface(
    open,
    root,
    surface,
    placement?.includes("Right"),
  );
  const change = (next: boolean) => {
    setLocal(next);
    onOpenChange?.(next);
  };
  useEffect(() => {
    if (!open) return;
    const outside = (event: PointerEvent) => {
      if (!root.current?.contains(event.target as Node)) {
        setLocal(false);
        onOpenChange?.(false);
      }
    };
    document.addEventListener("pointerdown", outside);
    return () => document.removeEventListener("pointerdown", outside);
  }, [open, onOpenChange]);
  const hover = Array.isArray(trigger)
    ? trigger.includes("hover")
    : trigger === "hover";
  const child = isValidElement<HTMLAttributes<HTMLElement>>(children)
    ? cloneElement(children, {
        "aria-expanded": open,
        "aria-haspopup": haspopup,
        "aria-controls": open ? id : undefined,
        onClick: (event) => {
          children.props.onClick?.(event);
          change(hover ? true : !open);
        },
      })
    : children;
  return (
    <span
      ref={root}
      className={`ui-popover-anchor ${placement?.includes("Right") ? "ui-popover-right" : ""}`}
      onMouseEnter={hover ? () => change(true) : undefined}
      onMouseLeave={hover ? () => change(false) : undefined}
      onFocus={hover ? () => change(true) : undefined}
      onBlur={
        hover
          ? (event) => {
              if (!event.currentTarget.contains(event.relatedTarget))
                change(false);
            }
          : undefined
      }
      onClickCapture={(event) => {
        if (!open) opener.current = event.target as HTMLElement;
      }}
      onKeyDown={(event) => {
        if (event.key === "Escape" && open) {
          event.preventDefault();
          event.stopPropagation();
          change(false);
          (
            opener.current?.closest<HTMLElement>("button, a, input, [tabindex]") ??
            root.current?.querySelector<HTMLElement>("button, input")
          )?.focus();
        }
      }}
    >
      {child}
      {open && (
        <div
          id={id}
          ref={surface}
          popover="manual"
          style={position}
          className={`ui-popover ${overlayClassName ?? ""} ${classNames?.root ?? ""}`}
        >
          {title && <strong>{title}</strong>}
          {content}
        </div>
      )}
    </span>
  );
}
export function Tooltip({
  title,
  children,
}: {
  title?: ReactNode;
  children: ReactNode;
  placement?: string;
}) {
  const [visible, setVisible] = useState(false);
  const id = useId();
  const anchor = useRef<HTMLSpanElement>(null);
  const surface = useRef<HTMLSpanElement>(null);
  const position = useFloatingSurface(visible && !!title, anchor, surface);
  return (
    <span
      ref={anchor}
      className="ui-tooltip-anchor"
      onMouseEnter={() => setVisible(true)}
      onMouseLeave={() => setVisible(false)}
      onFocus={() => setVisible(true)}
      onBlur={() => setVisible(false)}
      onKeyDown={(event) => {
        if (event.key === "Escape") setVisible(false);
      }}
      aria-describedby={visible ? id : undefined}
    >
      {children}
      {visible && title && (
        <span
          ref={surface}
          popover="manual"
          style={position}
          role="tooltip"
          id={id}
          className="ui-tooltip"
        >
          {title}
        </span>
      )}
    </span>
  );
}
function MenuItems({ items, close }: { items: ItemType[]; close: () => void }) {
  return (
    <>
      {items.map((item) =>
        item.children?.length ? (
          <div key={item.key}>
            <span className="ui-menu-heading">{item.label}</span>
            <MenuItems items={item.children} close={close} />
          </div>
        ) : (
          <button
            key={item.key}
            type="button"
            role="menuitem"
            className={item.danger ? "ui-menu-danger" : ""}
            disabled={item.disabled}
            onClick={() => {
              close();
              item.onClick?.();
            }}
          >
            {item.label}
          </button>
        ),
      )}
    </>
  );
}
export function Dropdown({
  children,
  menu,
}: {
  children: ReactNode;
  menu: { items: ItemType[] };
  trigger?: string[];
  placement?: string;
}) {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLSpanElement>(null);
  useEffect(() => {
    if (open)
      trigger.current
        ?.querySelector<HTMLElement>('[role="menuitem"]:not(:disabled)')
        ?.focus();
  }, [open]);
  return (
    <span
      ref={trigger}
      onKeyDown={(event) => {
        const items = Array.from(
          trigger.current?.querySelectorAll<HTMLButtonElement>(
            '[role="menuitem"]:not(:disabled)',
          ) ?? [],
        );
        if (["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
          event.preventDefault();
          if (!open) {
            setOpen(true);
            return;
          }
          const index = items.indexOf(
            document.activeElement as HTMLButtonElement,
          );
          items[
            event.key === "Home"
              ? 0
              : event.key === "End"
                ? items.length - 1
                : (index +
                    (event.key === "ArrowDown" ? 1 : -1) +
                    items.length) %
                  items.length
          ]?.focus();
        }
        if (event.key === "Tab") setOpen(false);
      }}
    >
      <Popover
        open={open}
        onOpenChange={setOpen}
        placement="bottomRight"
        haspopup="menu"
        content={
          <div role="menu" className="ui-menu">
            <MenuItems
              items={menu.items}
              close={() => {
                setOpen(false);
                trigger.current?.querySelector<HTMLElement>("button")?.focus();
              }}
            />
          </div>
        }
      >
        {children}
      </Popover>
    </span>
  );
}
function ConfirmDialog({
  options,
  close,
}: {
  options: Confirm;
  close: () => void;
}) {
  const [pending, setPending] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <Drawer
      open
      title={options.title}
      onClose={pending ? undefined : close}
      width={440}
      className="ui-confirm-dialog"
      footer={
        <>
          <Button disabled={pending} onClick={close}>
            {options.cancelText ?? "Cancelar"}
          </Button>
          <Button
            type="primary"
            danger={options.okButtonProps?.danger}
            loading={pending}
            onClick={async () => {
              setPending(true);
              setFailed(false);
              try {
                await options.onOk();
                close();
              } catch {
                setFailed(true);
                setPending(false);
              }
            }}
          >
            {options.okText ?? "Confirmar"}
          </Button>
        </>
      }
    >
      {options.content}
      {failed && (
        <p role="alert">Não foi possível concluir. Tente novamente.</p>
      )}
    </Drawer>
  );
}
export function FeedbackProvider({
  children,
}: {
  children: ReactNode;
  component?: false;
}) {
  const [confirmation, setConfirmation] = useState<Confirm | null>(null);
  const [messages, setMessages] = useState<
    { id: number; text: string; type: string }[]
  >([]);
  const serial = useRef(0);
  const toasts = useRef<HTMLDivElement>(null);
  // Top layer, so toasts stay visible above open modal drawers.
  useLayoutEffect(() => {
    const element = toasts.current;
    if (!element) return;
    element.hidePopover?.();
    if (messages.length) element.showPopover?.();
  }, [messages]);
  const timers = useRef(new Set<ReturnType<typeof setTimeout>>());
  useEffect(() => {
    const pending = timers.current;
    return () => pending.forEach(clearTimeout);
  }, []);
  const notify = (
    type: string,
    { content, duration }: { content: string; duration: number },
  ) => {
    const id = serial.current++;
    setMessages((items) => [...items, { id, text: content, type }]);
    const timer = setTimeout(() => {
      setMessages((items) => items.filter((item) => item.id !== id));
      timers.current.delete(timer);
    }, duration * 1000);
    timers.current.add(timer);
  };
  return (
    <FeedbackContext.Provider
      value={{
        modal: { confirm: setConfirmation },
        message: {
          success: (options) => notify("success", options),
          error: (options) => notify("error", options),
          info: (options) => notify("info", options),
        },
      }}
    >
      {children}
      {createPortal(
        <div ref={toasts} popover="manual" className="ui-messages">
          {messages.map((item) => (
            <div
              key={item.id}
              role={item.type === "error" ? "alert" : "status"}
              className={`ui-message ui-message-${item.type}`}
            >
              {item.text}
            </div>
          ))}
        </div>,
        document.body,
      )}
      {confirmation && (
        <ConfirmDialog
          options={confirmation}
          close={() => setConfirmation(null)}
        />
      )}
    </FeedbackContext.Provider>
  );
}
export function Popconfirm({
  title,
  description,
  onConfirm,
  okText,
  cancelText,
  children,
}: {
  title: ReactNode;
  description?: ReactNode;
  onConfirm: () => unknown | Promise<unknown>;
  okText?: string;
  cancelText?: string;
  children: ReactElement<HTMLAttributes<HTMLElement>>;
}) {
  const [open, setOpen] = useState(false);
  return (
    <>
      {cloneElement(children, {
        onClick: (event) => {
          children.props.onClick?.(event);
          setOpen(true);
        },
      })}
      {open && (
        <ConfirmDialog
          options={{
            title,
            content: description,
            onOk: onConfirm,
            okText,
            cancelText,
          }}
          close={() => setOpen(false)}
        />
      )}
    </>
  );
}
