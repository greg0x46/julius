import { Button, Checkbox, Tag } from "../ui";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";

import { FilterButton } from "../layout/FilterButton";
import { ListToolbar } from "../layout/ListToolbar";
import { SearchField } from "../layout/SearchField";
import { FilterCombobox } from "./FilterCombobox";
import { FilterField, FilterPanel } from "./FilterPanel";
import { MoneyRangeFilter } from "./MoneyRangeFilter";
import { SegmentedControl } from "./SegmentedControl";

export type FilterFieldType =
  | "text"
  | "select"
  | "multiselect"
  | "value-range"
  | "segmented"
  | "boolean";

export interface FilterOption {
  value: string;
  label: string;
  disabled?: boolean;
  /** Purely additive — only consumers that opt in via FilterConfig.optionRender read these. */
  icon?: string;
  color?: string;
}

export interface FilterConfig<Values extends object> {
  key: Extract<keyof Values, string>;
  secondaryKey?: Extract<keyof Values, string>;
  label: string;
  type: FilterFieldType;
  placement: "main" | "advanced";
  placeholder?: string;
  debounceMs?: number;
  options?: FilterOption[] | ((values: Values) => FilterOption[]);
  hideChip?: boolean;
  /**
   * The chip's text. `selected` holds the option(s) the value names — one
   * for a select, several for a multiselect, none for the other types.
   */
  formatActive?: (value: unknown, values: Values, selected: FilterOption[]) => string;
  /** Optional richer rendering (e.g. icon + color) for each dropdown option. */
  optionRender?: (option: FilterOption) => ReactNode;
}

/** What a custom advanced layout gets to work with. */
export interface AdvancedFilterProps<Values extends object> {
  draft: Values;
  /** Sets one field of the draft; the panel applies the whole draft at once. */
  set: <Key extends Extract<keyof Values, string>>(key: Key, value: Values[Key]) => void;
  /** The range validation message, if the draft currently fails it. */
  error: string | null;
}

type Props<Values extends object> = {
  /** Names the toolbar for assistive tech: "Filtros de transações". */
  label: string;
  values: Values;
  emptyValues: Values;
  config: FilterConfig<Values>[];
  onApply: (values: Values) => void;
  onClear: () => void;
  /** Discreet one-tap toggles under the toolbar row (see QuickFilters), owned by the caller. */
  shortcuts?: ReactNode;
  /** The shaping side of the toolbar (group by, sort), owned by the caller. */
  end?: ReactNode;
  /**
   * The advanced panel's layout. Without it every advanced field is
   * rendered in config order; with it the caller groups them as it likes,
   * while the counting, chips, draft and validation stay here.
   */
  renderAdvanced?: (props: AdvancedFilterProps<Values>) => ReactNode;
};

const read = <Values extends object>(
  values: Values,
  key: Extract<keyof Values, string>,
): unknown => (values as Record<string, unknown>)[key];

const write = <Values extends object>(
  values: Values,
  key: Extract<keyof Values, string>,
  value: unknown,
): Values => ({ ...values, [key]: value });

function fieldOptions<Values extends object>(
  field: FilterConfig<Values>,
  values: Values,
): FilterOption[] {
  if (typeof field.options === "function") return field.options(values);
  return field.options ?? [];
}

function isPresent(value: unknown): boolean {
  if (Array.isArray(value)) return value.length > 0;
  if (typeof value === "boolean") return value;
  return value !== null && value !== undefined && value !== "";
}

/**
 * One field is one filter, however many values it holds: three accounts
 * picked in the account filter count once, and a range counts once even
 * when both ends are set.
 */
function fieldIsActive<Values extends object>(
  field: FilterConfig<Values>,
  values: Values,
): boolean {
  return (
    isPresent(read(values, field.key)) ||
    (field.secondaryKey !== undefined && isPresent(read(values, field.secondaryKey)))
  );
}

function replaceField<Values extends object>(
  values: Values,
  field: FilterConfig<Values>,
  emptyValues: Values,
): Values {
  let next = write(values, field.key, read(emptyValues, field.key));
  if (field.secondaryKey) {
    next = write(next, field.secondaryKey, read(emptyValues, field.secondaryKey));
  }
  return next;
}

function compareDecimalStrings(left: string, right: string): number {
  const [leftInteger = "0", leftFraction = ""] = left.split(".");
  const [rightInteger = "0", rightFraction = ""] = right.split(".");
  const normalizedLeft = leftInteger.replace(/^0+(?=\d)/, "");
  const normalizedRight = rightInteger.replace(/^0+(?=\d)/, "");
  if (normalizedLeft.length !== normalizedRight.length) {
    return normalizedLeft.length - normalizedRight.length;
  }
  if (normalizedLeft !== normalizedRight) {
    return normalizedLeft.localeCompare(normalizedRight);
  }
  const scale = Math.max(leftFraction.length, rightFraction.length);
  return leftFraction.padEnd(scale, "0").localeCompare(rightFraction.padEnd(scale, "0"));
}

export function ConfigurableFilters<Values extends object>({
  label,
  values,
  emptyValues,
  config,
  onApply,
  onClear,
  end,
  shortcuts,
  renderAdvanced,
}: Props<Values>) {
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [quickValues, setQuickValues] = useState(values);
  const [draft, setDraft] = useState(values);
  const [error, setError] = useState<string | null>(null);
  const quickTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    if (quickTimer.current) clearTimeout(quickTimer.current);
    setQuickValues(values);
    setDraft(values);
  }, [values]);
  useEffect(
    () => () => {
      if (quickTimer.current) clearTimeout(quickTimer.current);
    },
    [],
  );

  const mainFields = config.filter((field) => field.placement === "main");
  const advancedFields = config.filter((field) => field.placement === "advanced");
  // One source for the toolbar button and the panel header alike.
  const advancedCount = advancedFields.filter((field) => fieldIsActive(field, values)).length;
  const draftHasAdvanced = advancedFields.some((field) => fieldIsActive(field, draft));
  const chips = config.filter(
    (field) => !field.hideChip && fieldIsActive(field, values),
  );

  const optionsByKey = useMemo(
    () =>
      Object.fromEntries(
        config.map((field) => [field.key, fieldOptions(field, values)]),
      ) as Record<string, FilterOption[]>,
    [config, values],
  );

  const update = (
    source: Values,
    field: FilterConfig<Values>,
    value: unknown,
    secondary?: unknown,
  ) => {
    let next = write(source, field.key, value);
    if (field.secondaryKey && secondary !== undefined) {
      next = write(next, field.secondaryKey, secondary);
    }
    return next;
  };

  const rangeError = (candidate: Values): string | null => {
    for (const field of config) {
      if (!field.secondaryKey) continue;
      const first = read(candidate, field.key);
      const second = read(candidate, field.secondaryKey);
      if (
        typeof first === "string" &&
        first !== "" &&
        typeof second === "string" &&
        second !== "" &&
        compareDecimalStrings(first, second) > 0
      ) {
        return "O valor mínimo não pode ser maior que o valor máximo.";
      }
    }
    return null;
  };
  const validate = (candidate: Values): boolean => {
    const message = rangeError(candidate);
    setError(message);
    return message === null;
  };

  const applyQuick = (
    field: FilterConfig<Values>,
    value: unknown,
    secondary?: unknown,
  ) => {
    const next = update(quickValues, field, value, secondary);
    setQuickValues(next);
    if (!validate(next)) return;
    if (quickTimer.current) clearTimeout(quickTimer.current);
    if (field.debounceMs) {
      quickTimer.current = setTimeout(() => onApply(next), field.debounceMs);
    } else {
      onApply(next);
    }
  };

  const setDraftField: AdvancedFilterProps<Values>["set"] = (key, value) => {
    setDraft((current) => {
      const next = write(current, key, value);
      // A range that stops being inconsistent clears its message as you type;
      // becoming inconsistent only reports on Aplicar.
      if (error && rangeError(next) === null) setError(null);
      return next;
    });
  };

  const closeAdvanced = () => {
    setDraft(values);
    setError(null);
    setAdvancedOpen(false);
  };
  const clearAdvanced = () => {
    let next = draft;
    for (const field of advancedFields) {
      next = replaceField(next, field, emptyValues);
    }
    setDraft(next);
    setError(null);
  };
  const applyAdvanced = () => {
    if (!validate(draft)) return;
    onApply(draft);
    setAdvancedOpen(false);
  };

  const renderField = (
    field: FilterConfig<Values>,
    source: Values,
    quick: boolean,
  ) => {
    const value = read(source, field.key);
    const secondary = field.secondaryKey ? read(source, field.secondaryKey) : null;
    const commit = (nextValue: unknown, nextSecondary?: unknown) => {
      if (quick) applyQuick(field, nextValue, nextSecondary);
      else setDraft((current) => update(current, field, nextValue, nextSecondary));
    };
    const options = fieldOptions(field, source);
    const id = `filter-${field.key}`;

    if (field.type === "value-range") {
      return (
        <FilterField key={field.key} id={`${id}-min`} label={field.label}>
          <MoneyRangeFilter
            id={id}
            min={typeof value === "string" ? value : null}
            max={typeof secondary === "string" ? secondary : null}
            onChange={(min, max) => commit(min, max)}
            error={error}
          />
        </FilterField>
      );
    }

    if (field.type === "boolean") {
      return (
        <Checkbox
          key={field.key}
          className="filter-checkbox"
          checked={value === true}
          onChange={(event) => commit(event.target.checked)}
        >
          {field.label}
        </Checkbox>
      );
    }

    if (field.type === "segmented") {
      return (
        <FilterField key={field.key} id={id} label={field.label}>
          <SegmentedControl
            id={id}
            label={field.label}
            value={typeof value === "string" && value ? value : options[0]?.value ?? ""}
            options={options}
            onChange={commit}
          />
        </FilterField>
      );
    }

    if (field.type === "multiselect") {
      return (
        <FilterField key={field.key} id={id} label={field.label}>
          <FilterCombobox
            multiple
            id={id}
            label={field.label}
            placeholder={field.placeholder ?? field.label}
            options={options}
            optionRender={field.optionRender}
            value={Array.isArray(value) ? (value as string[]) : []}
            onChange={commit}
          />
        </FilterField>
      );
    }

    if (field.type === "select") {
      return (
        <FilterField key={field.key} id={id} label={field.label}>
          <FilterCombobox
            id={id}
            label={field.label}
            placeholder={field.placeholder ?? field.label}
            options={options}
            optionRender={field.optionRender}
            value={typeof value === "string" ? value : null}
            onChange={commit}
          />
        </FilterField>
      );
    }

    return (
      <SearchField
        key={field.key}
        id={id}
        label={field.label}
        placeholder={field.placeholder}
        value={typeof value === "string" ? value : ""}
        onChange={(next) => commit(next || null)}
      />
    );
  };

  const chipList = chips.length > 0 && (
    <div className="list-toolbar-chips" aria-label="Filtros ativos">
      {chips.map((field) => {
        const value = read(values, field.key);
        const wanted = Array.isArray(value) ? (value as unknown[]) : [value];
        const selected = optionsByKey[field.key]?.filter((item) => wanted.includes(item.value)) ?? [];
        const shown = field.formatActive
          ? field.formatActive(value, values, selected)
          : selected.map((item) => item.label).join(", ") || `${field.label}: ${String(value)}`;
        return (
          <Tag
            key={field.key}
            closable
            onClose={(event) => {
              event.preventDefault();
              onApply(replaceField(values, field, emptyValues));
            }}
            closeIcon={<span aria-label={`Remover ${shown}`}>×</span>}
          >
            {shown}
          </Tag>
        );
      })}
      <Button type="link" size="small" onClick={onClear}>
        Limpar filtros
      </Button>
    </div>
  );

  return (
    <>
      <ListToolbar
        label={label}
        start={
          <>
            {mainFields.map((field) => renderField(field, quickValues, true))}
            <FilterButton activeCount={advancedCount} onClick={() => setAdvancedOpen(true)} />
          </>
        }
        end={end}
        chips={
          shortcuts || chipList ? (
            // One row: it scrolls sideways on a phone instead of stacking
            // shortcuts and active chips into a second and third line.
            <div className="filter-chips-row">
              {shortcuts}
              {chipList}
            </div>
          ) : undefined
        }
      />
      <FilterPanel
        open={advancedOpen}
        onClose={closeAdvanced}
        activeCount={advancedCount}
        onClear={clearAdvanced}
        clearDisabled={!draftHasAdvanced}
        onApply={applyAdvanced}
      >
        {renderAdvanced ? (
          renderAdvanced({ draft, set: setDraftField, error })
        ) : (
          <div className="filter-section-body">
            {advancedFields.map((field) => renderField(field, draft, false))}
          </div>
        )}
      </FilterPanel>
    </>
  );
}
