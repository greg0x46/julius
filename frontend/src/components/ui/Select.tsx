import { useFloatingSurface } from "./useFloatingSurface";
import {
  useEffect,
  useId,
  useRef,
  useState,
  type AriaAttributes,
  type CSSProperties,
  type ReactNode,
} from "react";

export interface SelectOption {
  value: string | number;
  label?: ReactNode;
  disabled?: boolean;
  searchText?: string;
  title?: string;
  [key: string]: unknown;
}
interface SelectProps<T, O extends SelectOption> extends AriaAttributes {
  id?: string;
  value?: T | null;
  options?: O[];
  onChange?: (value: T, option?: O) => void;
  disabled?: boolean;
  loading?: boolean;
  placeholder?: string;
  allowClear?: boolean;
  showSearch?: boolean;
  optionFilterProp?: string;
  filterOption?: boolean | ((input: string, option?: O) => boolean);
  optionRender?: (
    option: O & { data: O },
    info: { index: number },
  ) => ReactNode;
  labelRender?: (option: O) => ReactNode;
  searchValue?: string;
  onSearch?: (text: string) => void;
  onClear?: () => void;
  onDropdownVisibleChange?: (open: boolean) => void;
  onOpenChange?: (open: boolean) => void;
  onPopupScroll?: (event: React.UIEvent<HTMLDivElement>) => void;
  mode?: "multiple" | "tags";
  maxTagCount?: number | "responsive";
  suffixIcon?: ReactNode;
  className?: string;
  style?: CSSProperties;
  autoFocus?: boolean;
  status?: string;
  notFoundContent?: ReactNode;
  popupMatchSelectWidth?: boolean | number;
  classNames?: { popup?: { root?: string } };
  size?: string;
  listHeight?: number;
  listItemHeight?: number;
  virtual?: boolean;
}
export function Select<T = string, O extends SelectOption = SelectOption>({
  id,
  value,
  options = [],
  onChange,
  disabled,
  loading,
  placeholder = "Selecione",
  allowClear,
  showSearch,
  optionFilterProp = "label",
  filterOption,
  optionRender,
  labelRender,
  searchValue,
  onSearch,
  onClear,
  onDropdownVisibleChange,
  onOpenChange,
  onPopupScroll,
  mode,
  maxTagCount,
  suffixIcon,
  className = "",
  style,
  autoFocus,
  status,
  notFoundContent = "Nenhuma opção",
  popupMatchSelectWidth,
  classNames,
  size,
  listHeight,
  listItemHeight,
  virtual,
  ...aria
}: SelectProps<T, O>) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const root = useRef<HTMLDivElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const listId = useId();
  const surface = useRef<HTMLDivElement>(null);
  const position = useFloatingSurface(
    open && !disabled,
    root,
    surface,
    false,
    true,
  );
  const search = searchValue ?? query;
  const chosen: unknown[] = Array.isArray(value)
    ? value
    : value == null
      ? []
      : [value];
  const filtered = options.filter(
    (option) =>
      !showSearch ||
      !search ||
      filterOption === false ||
      (typeof filterOption === "function"
        ? filterOption(search, option)
        : String(option[optionFilterProp] ?? option.label ?? option.value)
            .toLocaleLowerCase("pt-BR")
            .includes(search.toLocaleLowerCase("pt-BR"))),
  );
  const changeOpen = (next: boolean) => {
    setOpen(next);
    const selectedIndex = filtered.findIndex(
      (option) => !option.disabled && chosen.includes(option.value),
    );
    setActive(
      selectedIndex >= 0
        ? selectedIndex
        : Math.max(
            0,
            filtered.findIndex((option) => !option.disabled),
          ),
    );
    onDropdownVisibleChange?.(next);
    onOpenChange?.(next);
    if (!next) {
      setQuery("");
      onSearch?.("");
    }
  };
  useEffect(() => {
    if (!open) return;
    const outside = (event: PointerEvent) => {
      if (!root.current?.contains(event.target as Node)) {
        setOpen(false);
        setQuery("");
        onSearch?.("");
        onOpenChange?.(false);
        onDropdownVisibleChange?.(false);
      }
    };
    document.addEventListener("pointerdown", outside);
    return () => document.removeEventListener("pointerdown", outside);
  }, [open, onSearch, onOpenChange, onDropdownVisibleChange]);
  useEffect(() => {
    if (open)
      surface.current
        ?.querySelector(`[id="${listId}-${active}"]`)
        ?.scrollIntoView?.({ block: "nearest" });
  }, [open, active, listId]);
  const choose = (option: O) => {
    if (option.disabled) return;
    const next = mode
      ? chosen.includes(option.value)
        ? chosen.filter((item) => item !== option.value)
        : [...chosen, option.value]
      : option.value;
    onChange?.(next as T, option);
    if (!mode) changeOpen(false);
    input.current?.focus();
  };
  const selected = chosen.map(
    (key) =>
      options.find((option) => option.value === key) ?? {
        value: key as string,
        label: String(key),
      },
  );
  const label = selected.map((option) =>
    labelRender ? labelRender(option as O) : (option.label ?? option.value),
  );
  void listItemHeight;
  void listHeight;
  void virtual;
  void maxTagCount;
  void popupMatchSelectWidth;
  void size;
  return (
    <div
      className={`ui-select ${status ? `ui-select-${status}` : ""} ${className}`}
      style={style}
      ref={root}
    >
      <div className="ui-select-selector">
        <input
          {...aria}
          id={id}
          ref={input}
          className="ui-select-input"
          role="combobox"
          aria-expanded={open}
          aria-controls={open ? listId : undefined}
          aria-describedby={
            [aria["aria-describedby"], chosen.length ? `${listId}-value` : ""]
              .filter(Boolean)
              .join(" ") || undefined
          }
          aria-autocomplete={showSearch ? "list" : "none"}
          aria-activedescendant={
            open && filtered[active] ? `${listId}-${active}` : undefined
          }
          autoComplete="off"
          autoFocus={autoFocus}
          disabled={disabled}
          readOnly={!showSearch}
          value={open && showSearch ? search : ""}
          placeholder={selected.length ? "" : placeholder}
          onClick={() => {
            if (!open) changeOpen(true);
          }}
          onChange={(event) => {
            setQuery(event.target.value);
            onSearch?.(event.target.value);
            if (!open) changeOpen(true);
            setActive(0);
          }}
          onKeyDown={(event) => {
            if (event.key === "Escape" && open) {
              event.preventDefault();
              event.stopPropagation();
              changeOpen(false);
            }
            if (event.key === "Tab" && open) changeOpen(false);
            if (["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
              event.preventDefault();
              if (!open) changeOpen(true);
              else {
                const direction =
                  event.key === "ArrowUp" || event.key === "End" ? -1 : 1;
                const start =
                  event.key === "Home"
                    ? -1
                    : event.key === "End"
                      ? filtered.length
                      : active;
                for (let offset = 1; offset <= filtered.length; offset++) {
                  const next =
                    (start + offset * direction + filtered.length) %
                    filtered.length;
                  if (!filtered[next].disabled) {
                    setActive(next);
                    break;
                  }
                }
              }
            }
            if (event.key === "Enter" || (!showSearch && event.key === " ")) {
              event.preventDefault();
              if (!open) changeOpen(true);
              else if (filtered[active]) choose(filtered[active]);
            }
          }}
        />
        {(!open || !showSearch || !search) && selected.length > 0 && (
          <span id={`${listId}-value`} className="ui-select-selection-item">
            {mode
              ? label.map((item, i) => (
                  <span key={i} className="ui-tag">
                    {item}
                  </span>
                ))
              : label[0]}
          </span>
        )}
        {selected.length === 0 && !search && (
          <span className="ui-select-selection-placeholder" aria-hidden="true">
            {placeholder}
          </span>
        )}
        {allowClear && chosen.length > 0 && !disabled && (
          <button
            type="button"
            className="ui-select-clear"
            aria-label="Limpar seleção"
            onClick={() => {
              onChange?.((mode ? [] : undefined) as T);
              onClear?.();
              input.current?.focus();
            }}
          >
            ×
          </button>
        )}
        <span className="ui-select-arrow" aria-hidden="true">
          {loading ? "…" : (suffixIcon ?? "⌄")}
        </span>
      </div>
      {open && !disabled && (
        <div
          ref={surface}
          popover="manual"
          style={position}
          id={listId}
          role="listbox"
          aria-multiselectable={mode ? true : undefined}
          className={`ui-select-dropdown ${classNames?.popup?.root ?? ""}`}
          onScroll={onPopupScroll}
        >
          {filtered.length ? (
            filtered.map((option, index) => (
              <div
                id={`${listId}-${index}`}
                key={option.value}
                role="option"
                aria-selected={chosen.includes(option.value)}
                aria-disabled={option.disabled || undefined}
                className={`ui-select-item ${index === active ? "ui-select-item-active" : ""}`}
                onPointerMove={() => setActive(index)}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => choose(option)}
              >
                {optionRender
                  ? optionRender({ ...option, data: option }, { index })
                  : (option.label ?? option.value)}
              </div>
            ))
          ) : (
            <div className="ui-select-empty">
              {loading ? "Carregando…" : notFoundContent}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
