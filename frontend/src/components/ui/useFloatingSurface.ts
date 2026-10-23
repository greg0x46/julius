import {
  useLayoutEffect,
  useState,
  type CSSProperties,
  type RefObject,
} from "react";

/** The browser's top layer keeps menus visible above scrolling panels. */
export function useFloatingSurface(
  open: boolean,
  anchor: RefObject<HTMLElement | null>,
  surface: RefObject<HTMLElement | null>,
  alignRight = false,
  matchWidth = false,
) {
  const [position, setPosition] = useState<CSSProperties>({});
  useLayoutEffect(() => {
    const element = surface.current;
    const trigger = anchor.current;
    if (!open || !element || !trigger) return;
    element.showPopover?.();
    const update = () => {
      const rect = trigger.getBoundingClientRect();
      const bounds = element.getBoundingClientRect();
      const width = Math.min(
        Math.max(matchWidth ? rect.width : 0, bounds.width),
        window.innerWidth - 16,
      );
      const availableBelow = window.innerHeight - rect.bottom - 12;
      const above =
        availableBelow < Math.min(bounds.height, 180) &&
        rect.top > availableBelow;
      setPosition({
        position: "fixed",
        margin: 0,
        right: "auto",
        width: matchWidth ? Math.max(rect.width, width) : undefined,
        minWidth: matchWidth ? rect.width : undefined,
        maxWidth: window.innerWidth - 16,
        maxHeight: Math.max(80, above ? rect.top - 12 : availableBelow),
        left: Math.max(
          8,
          Math.min(
            alignRight ? rect.right - width : rect.left,
            window.innerWidth - width - 8,
          ),
        ),
        top: above ? "auto" : rect.bottom + 4,
        bottom: above ? window.innerHeight - rect.top + 4 : "auto",
      });
    };
    update();
    window.addEventListener("resize", update);
    window.addEventListener("scroll", update, true);
    return () => {
      window.removeEventListener("resize", update);
      window.removeEventListener("scroll", update, true);
      element.hidePopover?.();
    };
  }, [open, anchor, surface, alignRight, matchWidth]);
  return position;
}
