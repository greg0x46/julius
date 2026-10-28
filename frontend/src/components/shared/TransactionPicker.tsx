import { DownOutlined, SearchOutlined } from "../ui/icons";
import { Button, Drawer, Input, Select, Spin, Tag, Typography } from "../ui";
import { useEffect, useId, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from "react";

import { calendarParts, formatCompactDay } from "../../presentation/dates";
import { formatMoney, formatSignedBRL } from "../../presentation/money";
import { readableName } from "../../presentation/readableName";
import { useCompactScreen } from "./useCompactScreen";

/**
 * One row the picker can offer. Every flow that links a transaction — a bank
 * line to an investment movement, an occurrence to a transaction, a payment
 * to a payable — maps its own record onto this shape; the picker never knows
 * where it came from.
 */
export type PickerTransaction = {
  id: string;
  description: string;
  /** Unsigned decimal string; `direction` decides the sign shown. */
  amount: string;
  currency_code?: string;
  direction: "inflow" | "outflow" | null;
  /** "YYYY-MM-DD" or an instant; null when unknown. */
  date: string | null;
  account?: string | null;
  institution?: string | null;
  /** Category, kind or type — whatever tells this row apart from its neighbours. */
  category?: string | null;
  /** Trailing badge, e.g. "Sugestão". */
  tag?: string | null;
  /** Extra text the search should also match, e.g. a ticker or notes. */
  keywords?: string[];
};

type Props = {
  id: string;
  /** Accessible name; also the sheet's title on a compact screen. */
  label: string;
  value: string | null;
  transactions: PickerTransaction[];
  /** Business eligibility lives in the caller; this only narrows what is listed. */
  filter?: (transaction: PickerTransaction) => boolean;
  onSelect: (id: string | null, transaction: PickerTransaction | null) => void;
  placeholder?: string;
  searchPlaceholder?: string;
  loading?: boolean;
  disabled?: boolean;
  allowClear?: boolean;
  /** Shown when nothing is eligible at all (as opposed to nothing matching the search). */
  emptyText?: string;
  /**
   * Server-side search: the picker forwards what is typed instead of
   * filtering locally, so a narrowed result set is never filtered twice.
   */
  search?: { value: string; onChange: (value: string) => void };
};

const DEFAULT_EMPTY = "Nenhuma transação disponível para vincular.";
const NO_RESULTS = "Nenhuma transação encontrada.";
const NO_RESULTS_HINT = "Tente alterar sua busca.";
const LOADING = "Buscando transações...";

function normalize(text: string): string {
  return text
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .toLocaleLowerCase("pt-BR");
}

/** Every spelling of the amount and the date a person might type. */
function haystack(transaction: PickerTransaction): string {
  const parts: string[] = [
    transaction.description,
    readableName(transaction.description),
    transaction.account ?? "",
    transaction.institution ?? "",
    transaction.category ?? "",
    transaction.tag ?? "",
    ...(transaction.keywords ?? []),
  ];
  const formatted = formatMoney(transaction.amount, transaction.currency_code ?? "BRL").replace(/^-?R\$\s/, "");
  parts.push(formatted, formatted.replace(/\./g, ""), formatted.replace(/\./g, "").replace(",", "."), transaction.amount);
  if (transaction.date) {
    parts.push(formatCompactDay(transaction.date));
    const date = calendarParts(transaction.date);
    if (date) {
      const dd = String(date.day).padStart(2, "0");
      const mm = String(date.month).padStart(2, "0");
      parts.push(`${dd}/${mm}`, `${dd}/${mm}/${date.year}`, `${date.year}-${mm}-${dd}`);
    }
  }
  return normalize(parts.join(" "));
}

/** Every whitespace-separated token has to appear somewhere in the row. */
function matches(text: string, query: string): boolean {
  const tokens = normalize(query).split(/\s+/).filter(Boolean);
  return tokens.every((token) => text.includes(token));
}

function Amount({ transaction }: { transaction: PickerTransaction }) {
  return (
    <span className={`txn-picker-amount is-${transaction.direction ?? "neutral"}`}>
      {(transaction.currency_code && transaction.currency_code !== "BRL" ? `${transaction.direction === "outflow" ? "-" : transaction.direction === "inflow" ? "+" : ""}${formatMoney(transaction.amount, transaction.currency_code)}` : formatSignedBRL(transaction.amount, transaction.direction ?? "unclassified"))}
    </span>
  );
}

/**
 * The description as a person recognises it, with the provider's raw text
 * one hover away. The amount is what tells look-alike rows apart, so it is
 * never asked to make room for the description.
 */
function Description({ transaction }: { transaction: PickerTransaction }) {
  const readable = readableName(transaction.description);
  const title = readable === transaction.description ? undefined : transaction.description;
  return (
    <span className="txn-picker-description" title={title}>
      {readable}
    </span>
  );
}

/**
 * A list row on a two-column grid: the left column (description, then
 * date · account · category) grows and truncates; the right column holds the
 * amount at its natural width, so no description can push it out of view.
 */
function Row({ transaction }: { transaction: PickerTransaction }) {
  const meta = [
    transaction.date ? formatCompactDay(transaction.date) : null,
    transaction.account,
    transaction.institution,
    transaction.category,
  ].filter(Boolean);
  return (
    <span className="txn-picker-row">
      <Description transaction={transaction} />
      <Amount transaction={transaction} />
      {(meta.length > 0 || transaction.tag) && (
        <span className="txn-picker-meta">
          <span className="txn-picker-meta-text">{meta.join(" · ")}</span>
          {transaction.tag && <Tag color="blue">{transaction.tag}</Tag>}
        </span>
      )}
    </span>
  );
}

/** The chosen row inside the closed control: one line, the amount never clipped. */
function Value({ transaction }: { transaction: PickerTransaction }) {
  return (
    <span className="txn-picker-value">
      <Description transaction={transaction} />
      {transaction.date && <span className="txn-picker-value-date">{formatCompactDay(transaction.date)}</span>}
      <Amount transaction={transaction} />
    </span>
  );
}

function Empty({ loading, hasQuery, emptyText }: { loading: boolean; hasQuery: boolean; emptyText: string }) {
  if (loading) {
    return (
      <span className="txn-picker-state" role="status">
        <Spin size="small" /> {LOADING}
      </span>
    );
  }
  if (hasQuery) {
    return (
      <span className="txn-picker-state" role="status">
        <span>{NO_RESULTS}</span>
        <Typography.Text type="secondary">{NO_RESULTS_HINT}</Typography.Text>
      </span>
    );
  }
  return (
    <span className="txn-picker-state" role="status">
      {emptyText}
    </span>
  );
}

/**
 * The one way to pick a transaction anywhere in Julius. Each row shows what a
 * person needs to tell two look-alike lines apart — description and amount on
 * the first line, date · account · category on the second — and the search
 * matches any of them. From `md` up it is a searchable combobox; on a compact
 * screen a bottom sheet with its own search box, where a dropdown would fight
 * the keyboard and the panel's scroll.
 */
export function TransactionPicker({
  id,
  label,
  value,
  transactions,
  filter,
  onSelect,
  placeholder = "Selecione uma transação",
  searchPlaceholder = "Buscar por descrição, valor ou conta...",
  loading = false,
  disabled = false,
  allowClear = false,
  emptyText = DEFAULT_EMPTY,
  search,
}: Props) {
  const compact = useCompactScreen();
  const [localSearch, setLocalSearch] = useState("");
  const [sheetOpen, setSheetOpen] = useState(false);
  // The chosen row is kept even after a server search narrows it out of the
  // list, so the control keeps showing what was picked.
  const [remembered, setRemembered] = useState<PickerTransaction | null>(null);

  const eligible = useMemo(() => (filter ? transactions.filter(filter) : transactions), [transactions, filter]);
  const indexed = useMemo(() => eligible.map((transaction) => ({ transaction, text: haystack(transaction) })), [eligible]);
  const query = search ? search.value : localSearch;
  const setQuery = search ? search.onChange : setLocalSearch;
  const visible = useMemo(
    () =>
      search || query.trim() === ""
        ? eligible
        : indexed.filter((entry) => matches(entry.text, query)).map((entry) => entry.transaction),
    [search, query, eligible, indexed],
  );
  const selected =
    eligible.find((transaction) => transaction.id === value) ?? (remembered?.id === value ? remembered : null);

  const choose = (next: string | null) => {
    const transaction = next === null ? null : (eligible.find((item) => item.id === next) ?? null);
    setRemembered(transaction);
    onSelect(next, transaction);
  };
  const empty = <Empty loading={loading} hasQuery={query.trim() !== ""} emptyText={emptyText} />;

  if (!compact) {
    return (
      <Select
        id={id}
        aria-label={label}
        className="txn-picker"
        classNames={{ popup: { root: "txn-picker-popup" } }}
        style={{ width: "100%" }}
        value={value ?? undefined}
        placeholder={placeholder}
        loading={loading}
        disabled={disabled}
        allowClear={allowClear}
        showSearch
        searchValue={query}
        onSearch={setQuery}
        filterOption={false}
        suffixIcon={<SearchOutlined aria-hidden="true" />}
        listHeight={336}
        listItemHeight={56}
        options={visible.map((transaction) => ({ value: transaction.id, label: transaction.description, transaction }))}
        optionRender={(option) => <Row transaction={option.data.transaction} />}
        labelRender={() => (selected ? <Value transaction={selected} /> : placeholder)}
        notFoundContent={empty}
        onChange={(next: string | undefined) => choose(next ?? null)}
        onOpenChange={(open) => {
          if (!open) setQuery("");
        }}
      />
    );
  }

  const close = () => {
    setSheetOpen(false);
    setQuery("");
  };

  const clearable = allowClear && selected !== null && !disabled;

  return (
    <>
      <div style={clearable ? { display: "flex", alignItems: "center", gap: 8 } : undefined}>
      <button
        type="button"
        id={id}
        className={`txn-picker-trigger ${selected ? "" : "is-placeholder"}`}
        aria-label={label}
        aria-haspopup="dialog"
        aria-expanded={sheetOpen}
        disabled={disabled}
        onClick={() => setSheetOpen(true)}
      >
        <span className="txn-picker-trigger-value">{selected ? <Value transaction={selected} /> : placeholder}</span>
        {loading ? <Spin size="small" /> : <DownOutlined aria-hidden="true" />}
      </button>
      {clearable && (
        <Button type="link" size="small" aria-label={`Limpar ${label}`} onClick={() => choose(null)}>
          Limpar
        </Button>
      )}
      </div>
      <Sheet
        open={sheetOpen}
        label={label}
        value={value}
        visible={visible}
        query={query}
        onQuery={setQuery}
        searchPlaceholder={searchPlaceholder}
        empty={empty}
        onClose={close}
        onChoose={(next) => {
          choose(next);
          close();
        }}
      />
    </>
  );
}

/**
 * The compact-screen sheet. Focus stays in the search box the whole time —
 * arrows move `aria-activedescendant`, Enter picks, Esc closes — so the
 * keyboard never has to be dismissed to browse.
 */
function Sheet({
  open,
  label,
  value,
  visible,
  query,
  onQuery,
  searchPlaceholder,
  empty,
  onClose,
  onChoose,
}: {
  open: boolean;
  label: string;
  value: string | null;
  visible: PickerTransaction[];
  query: string;
  onQuery: (value: string) => void;
  searchPlaceholder: string;
  empty: ReactNode;
  onClose: () => void;
  onChoose: (id: string | null) => void;
}) {
  const listId = useId();
  const [active, setActive] = useState(0);
  const listRef = useRef<HTMLUListElement>(null);

  // A new result set (or a fresh open) starts the cursor on the chosen row,
  // or at the top.
  useEffect(() => {
    const index = visible.findIndex((transaction) => transaction.id === value);
    setActive(index === -1 ? 0 : index);
  }, [open, visible, value]);

  useEffect(() => {
    listRef.current?.querySelector<HTMLElement>(`[data-index="${active}"]`)?.scrollIntoView?.({ block: "nearest" });
  }, [active]);

  const optionId = (index: number) => `${listId}-${index}`;

  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActive((current) => Math.min(current + 1, visible.length - 1));
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setActive((current) => Math.max(current - 1, 0));
    } else if (event.key === "Enter") {
      event.preventDefault();
      const transaction = visible[active];
      if (transaction) onChoose(transaction.id);
    } else if (event.key === "Escape") {
      event.preventDefault();
      onClose();
    }
  };

  return (
    <Drawer
      open={open}
      onClose={onClose}
      placement="bottom"
      height="85dvh"
      title={label}
      className="txn-picker-sheet"
      destroyOnHidden
    >
      <Input
        allowClear
        autoFocus
        role="combobox"
        aria-expanded
        aria-controls={listId}
        aria-activedescendant={visible[active] ? optionId(active) : undefined}
        prefix={<SearchOutlined aria-hidden="true" />}
        placeholder={searchPlaceholder}
        aria-label={`Buscar em ${label}`}
        value={query}
        onChange={(event) => onQuery(event.target.value)}
        onKeyDown={onKeyDown}
      />
      <ul ref={listRef} id={listId} className="txn-picker-list" role="listbox" aria-label={label}>
        {visible.map((transaction, index) => (
          <li
            key={transaction.id}
            id={optionId(index)}
            data-index={index}
            role="option"
            aria-selected={transaction.id === value}
            className={`txn-picker-option ${transaction.id === value ? "is-selected" : ""} ${index === active ? "is-active" : ""}`}
            onMouseEnter={() => setActive(index)}
            onClick={() => onChoose(transaction.id)}
          >
            <Row transaction={transaction} />
          </li>
        ))}
      </ul>
      {visible.length === 0 && empty}
    </Drawer>
  );
}
