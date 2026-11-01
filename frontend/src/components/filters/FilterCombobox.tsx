import { useTranslation } from "../../i18n/context";
import { CheckOutlined, DownOutlined, SearchOutlined } from "../ui/icons";
import { Button, Drawer, Input, Popover, Spin, type InputRef } from "../ui";
import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type ReactNode,
} from "react";

import { useCompactScreen } from "../shared/useCompactScreen";

export interface ComboboxOption {
  value: string;
  label: string;
  disabled?: boolean;
  /** Purely additive — read only by a caller-supplied optionRender. */
  icon?: string;
  color?: string;
}

interface CommonProps {
  /** The trigger's element id; the caller's <label htmlFor> points at it. */
  id: string;
  /** Accessible name of the list ("Contas"), also the sheet's title on a phone. */
  label: string;
  /** Shown on the trigger while nothing is selected: "Todas as contas". */
  placeholder: string;
  options: ComboboxOption[];
  searchPlaceholder?: string;
  loading?: boolean;
  /** A failure loading `options`; shown in place of the list. */
  error?: string | null;
  onRetry?: () => void;
  emptyText?: string;
  disabled?: boolean;
  /** "3 contas selecionadas" — the trigger's summary past two picks and the sheet's counter. */
  countLabel?: (count: number) => string;
  optionRender?: (option: ComboboxOption) => ReactNode;
}

type MultipleProps = CommonProps & {
  multiple: true;
  value: string[];
  onChange: (value: string[]) => void;
};

type SingleProps = CommonProps & {
  multiple?: false;
  value: string | null;
  onChange: (value: string | null) => void;
};

export type FilterComboboxProps = MultipleProps | SingleProps;

function normalize(text: string): string {
  return text
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase();
}


/**
 * The trigger's text: nothing → placeholder; one pick → its name; more →
 * the first name and how many others ("Nubank +2"). Account and category
 * names run long, so two names side by side would already truncate. The
 * field never grows with its selection; the open list is where the whole
 * set is visible.
 */
function comboboxSummary(
  selected: ComboboxOption[],
  placeholder: string,
): string {
  if (selected.length === 0) return placeholder;
  if (selected.length === 1) return selected[0]!.label;
  return `${selected[0]!.label} +${selected.length - 1}`;
}

/**
 * A filter's choice of one or several options from a list that may be
 * long: a select-looking trigger that shows a compact summary, and a
 * searchable list behind it. From `md` up the list is a popover beside the
 * trigger and each pick applies at once; on a phone it is a bottom sheet
 * with its own Cancelar / Confirmar, because a list that reaches under the
 * thumb should not commit on every tap.
 *
 * In `multiple` mode picks are an "any of" set — the list stays open until
 * the person is done. In single mode a pick closes the list.
 */
export function FilterCombobox(props: FilterComboboxProps) {
  const { t } = useTranslation();
  const {
    id,
    label,
    placeholder,
    options: listedOptions,
    searchPlaceholder,
    loading = false,
    error = null,
    onRetry,
    emptyText,
    disabled = false,
    countLabel,
    optionRender,
  } = props;
  const multiple = props.multiple === true;
  const committed = useMemo<string[]>(
    () => (props.multiple ? props.value : props.value === null ? [] : [props.value]),
    [props.multiple, props.value],
  );

  const compact = useCompactScreen();
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  // On a phone the sheet edits a draft and commits on Confirmar; the popover
  // commits each pick, so its draft is always the committed value.
  const [draft, setDraft] = useState<string[]>(committed);
  const [activeIndex, setActiveIndex] = useState(0);
  const [triggerWidth, setTriggerWidth] = useState(0);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<InputRef>(null);
  const listId = useId();
  const summaryId = useId();
  const selectedValues = compact ? draft : committed;

  // A selected value the facets no longer list stays visible and removable.
  const options = useMemo<ComboboxOption[]>(() => {
    if (loading || error) return listedOptions;
    const listed = new Set(listedOptions.map((option) => option.value));
    const missing = [...new Set([...committed, ...draft])].filter((value) => !listed.has(value));
    return missing.length === 0
      ? listedOptions
      : [...listedOptions, ...missing.map((value) => ({ value, label: t("unavailableSelection") }))];
  }, [listedOptions, committed, draft, loading, error, t]);
  const byValue = useMemo(() => new Map(options.map((option) => [option.value, option])), [options]);
  const selectedOptions = committed.flatMap((value) => {
    const option = byValue.get(value);
    return option ? [option] : [];
  });
  const visible = useMemo(() => {
    const needle = normalize(search.trim());
    if (needle === "") return options;
    return options.filter((option) => normalize(option.label).includes(needle));
  }, [options, search]);

  useEffect(() => {
    setActiveIndex(0);
  }, [search, open]);

  // The list opens from a click that leaves focus on the trigger; the
  // search is where typing and the arrow keys should land. The popover
  // mounts its content a tick after `open` flips, hence the timer.
  useEffect(() => {
    if (!open) return;
    const timer = setTimeout(() => searchRef.current?.focus({ preventScroll: true }), 50);
    return () => clearTimeout(timer);
  }, [open]);

  useEffect(() => {
    const active = listRef.current?.querySelector<HTMLElement>(`[data-index="${activeIndex}"]`);
    active?.scrollIntoView?.({ block: "nearest" });
  }, [activeIndex, visible]);

  const commit = (values: string[]) => {
    if (props.multiple) props.onChange(values);
    else props.onChange(values[0] ?? null);
  };

  const openList = () => {
    if (disabled) return;
    setDraft(committed);
    setSearch("");
    setTriggerWidth(triggerRef.current?.offsetWidth ?? 0);
    setOpen(true);
  };
  const close = ({ restoreFocus = true } = {}) => {
    setOpen(false);
    setSearch("");
    if (restoreFocus) triggerRef.current?.focus();
  };

  const toggleValue = (value: string) => {
    if (multiple) {
      const next = selectedValues.includes(value)
        ? selectedValues.filter((item) => item !== value)
        : [...selectedValues, value];
      if (compact) setDraft(next);
      else commit(next);
      return;
    }
    const next = selectedValues.includes(value) ? [] : [value];
    if (compact) setDraft(next);
    commit(next);
    close();
  };
  const clearAll = () => {
    if (compact) setDraft([]);
    else commit([]);
  };

  const onSearchKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActiveIndex((index) => Math.min(index + 1, Math.max(visible.length - 1, 0)));
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setActiveIndex((index) => Math.max(index - 1, 0));
    } else if (event.key === "Home") {
      event.preventDefault();
      setActiveIndex(0);
    } else if (event.key === "End") {
      event.preventDefault();
      setActiveIndex(Math.max(visible.length - 1, 0));
    } else if (event.key === "Enter") {
      event.preventDefault();
      const option = visible[activeIndex];
      if (option && !option.disabled) toggleValue(option.value);
    } else if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      close();
    }
  };

  const activeOption = visible[activeIndex];
  const optionId = (value: string) => `${listId}-${value}`;

  const list = (
    <div
      ref={listRef}
      className="filter-combobox-list"
      role="listbox"
      id={listId}
      aria-label={label}
      aria-multiselectable={multiple || undefined}
    >
      {loading ? (
        <div className="filter-combobox-state" role="status">
          <Spin size="small" /> {t("loading")}
        </div>
      ) : error ? (
        <div className="filter-combobox-state" role="alert">
          <span>{error}</span>
          {onRetry && (
            <Button type="link" size="small" onClick={onRetry}>
              {t("retry")}
            </Button>
          )}
        </div>
      ) : visible.length === 0 ? (
        <div className="filter-combobox-state">
          {options.length === 0 ? (emptyText ?? t("noOptionsAvailable")) : t("noSearchResults")}
        </div>
      ) : (
        visible.map((option, index) => {
          const selected = selectedValues.includes(option.value);
          return (
            <div
              key={option.value}
              id={optionId(option.value)}
              data-index={index}
              role="option"
              aria-selected={selected}
              aria-disabled={option.disabled || undefined}
              className={`filter-combobox-option ${selected ? "is-selected" : ""} ${
                index === activeIndex ? "is-active" : ""
              }`}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => !option.disabled && toggleValue(option.value)}
            >
              {multiple ? (
                <span className={`filter-combobox-check ${selected ? "is-checked" : ""}`} aria-hidden="true">
                  {selected && <CheckOutlined />}
                </span>
              ) : null}
              <span className="filter-combobox-option-label">
                {optionRender ? optionRender(option) : option.label}
              </span>
              {!multiple && selected && <CheckOutlined className="filter-combobox-tick" aria-hidden="true" />}
            </div>
          );
        })
      )}
    </div>
  );

  const searchField = (
    <div className="filter-combobox-search">
      <Input
        ref={searchRef}
        allowClear
        role="combobox"
        aria-label={t("searchIn", { label })}
        aria-expanded="true"
        aria-controls={listId}
        aria-autocomplete="list"
        aria-activedescendant={activeOption ? optionId(activeOption.value) : undefined}
        prefix={<SearchOutlined aria-hidden="true" />}
        placeholder={searchPlaceholder ?? t("search")}
        value={search}
        onChange={(event) => setSearch(event.target.value)}
        onKeyDown={onSearchKeyDown}
      />
    </div>
  );

  const counter = multiple && (
    <span className="filter-combobox-count" aria-live="polite">
      {selectedValues.length > 0 ? (countLabel?.(selectedValues.length) ?? t(selectedValues.length === 1 ? "selectedOne" : "selectedMany", { count: selectedValues.length })) : t("noSelection")}
    </span>
  );

  const trigger = (
    <button
      ref={triggerRef}
      type="button"
      id={id}
      className={`filter-combobox-trigger ${selectedOptions.length === 0 ? "is-placeholder" : ""}`}
      aria-haspopup="listbox"
      aria-expanded={open}
      aria-describedby={summaryId}
      disabled={disabled}
      // From `md` up the Popover wires the trigger's click itself.
      onClick={compact ? () => (open ? close() : openList()) : undefined}
    >
      <span id={summaryId} className="filter-combobox-summary">
        {comboboxSummary(selectedOptions, placeholder)}
      </span>
      <DownOutlined aria-hidden="true" />
    </button>
  );

  if (compact) {
    return (
      <>
        {trigger}
        <Drawer
          open={open}
          onClose={() => close()}
          placement="bottom"
          height="85dvh"
          title={label}
          className="filter-combobox-sheet"
          destroyOnHidden
          footer={
            <div className="filter-combobox-sheet-footer">
              {multiple ? (
                <>
                  {counter}
                  <div className="filter-combobox-sheet-actions">
                    <Button onClick={() => close()}>{t("cancel")}</Button>
                    <Button
                      type="primary"
                      onClick={() => {
                        commit(draft);
                        close();
                      }}
                    >
                      {t("confirm")}
                    </Button>
                  </div>
                </>
              ) : (
                <Button
                  type="link"
                  size="small"
                  disabled={selectedValues.length === 0}
                  onClick={() => {
                    commit([]);
                    close();
                  }}
                >
                  {t("clearSelection")}
                </Button>
              )}
            </div>
          }
        >
          {searchField}
          <div className="filter-combobox-scroll">{list}</div>
        </Drawer>
      </>
    );
  }

  return (
    <Popover
      open={open}
      onOpenChange={(next) => (next ? openList() : close({ restoreFocus: false }))}
      trigger="click"
      placement="bottomLeft"
      arrow={false}
      destroyOnHidden
      classNames={{ root: "filter-combobox-popover" }}
      content={
        <div
          className="filter-combobox-popover-body"
          style={{ "--filter-combobox-width": `${triggerWidth}px` } as CSSProperties}
          onKeyDown={(event) => event.key === "Escape" && close()}
        >
          {searchField}
          <div className="filter-combobox-scroll">{list}</div>
          {multiple && (
            <div className="filter-combobox-popover-footer">
              {counter}
              <Button type="link" size="small" disabled={selectedValues.length === 0} onClick={clearAll}>
                {t("clear")}
              </Button>
            </div>
          )}
        </div>
      }
    >
      {trigger}
    </Popover>
  );
}
