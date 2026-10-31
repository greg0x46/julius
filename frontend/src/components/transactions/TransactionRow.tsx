import type { TransactionInclusionState, TransactionItem } from "../../api/contracts";
import { useTimezone } from "../../hooks/useTimezone";
import { formatCompactDay } from "../../presentation/dates";
import { zonedFormatter } from "../../presentation/timezone";
import { inclusionOriginLabel } from "../../presentation/transactionStatus";
import { TransactionActions } from "./TransactionActions";
import { TransactionAmount } from "./TransactionAmount";
import { TransactionCategory } from "./TransactionCategory";
import type { RowLayout } from "./rowLayout";

function rowTime(value: string | null, timezone: string): string | undefined {
  if (!value) return undefined;
  const instant = new Date(value);
  if (Number.isNaN(instant.getTime())) return undefined;
  return zonedFormatter(timezone, { hour: "2-digit", minute: "2-digit" }).format(instant);
}

function rowDay(value: string | null, timezone: string): string {
  return value ? formatCompactDay(value, timezone) : "Sem data";
}

/**
 * The lighter line of a row, one line that never wraps. Each item carries
 * its own leading "·" (see transactions.css), so a separator can never be
 * left dangling at the start of a line.
 *
 * On a phone it reads `data · categoria · conta` — there is no date or
 * category column there; on a narrow desktop only the category folds in
 * (the date is still a column); on a wide screen both are columns and the
 * line keeps the account (on a phone the account yields to the category).
 * `showAccount` is off when the list is already
 * scoped to one account. Manual / Arquivo / "Fora dos totais" are flags in
 * plain small text, shown only when they apply.
 */
function TransactionMeta({
  item,
  showAccount = true,
  layout = "wide",
}: {
  item: TransactionItem;
  showAccount?: boolean;
  /** Which columns the row has; what has no column folds into this line. */
  layout?: RowLayout;
}) {
  const timezone = useTimezone();
  const ignored = item.inclusion.state === "ignored";
  const account = item.account.name ?? item.account.institution ?? "Conta não informada";
  // A phone line has room for one place name: the category wins, and the
  // account (always in the panel) only shows when there is no category.
  const accountShown = showAccount && !(layout === "phone" && item.internal_category !== null);
  return (
    <span className="transaction-meta">
      {layout === "phone" && <span className="transaction-meta-date">{rowDay(item.occurred_at, timezone)}</span>}
      {layout !== "wide" && <TransactionCategory item={item} />}
      {accountShown && <span className="transaction-meta-where">{account}</span>}
      {item.origin === "manual" && <span className="transaction-meta-flag">Manual</span>}
      {item.source_provider === "file" && <span className="transaction-meta-flag">Arquivo</span>}
      {!ignored && (item.totals_eligibility.reason === "transfer_category" || item.totals_eligibility.reason === "linked_transfer") && (
        <span className="transaction-meta-flag">Fora dos totais</span>
      )}
      {ignored && (
        <span
          className="transaction-meta-flag is-ignored"
          title={item.inclusion.changed_at ? inclusionOriginLabel(item.inclusion) : undefined}
        >
          {item.inclusion.origin === "rule" ? "Fora dos totais (regra)" : "Fora dos totais"}
        </span>
      )}
    </span>
  );
}

/**
 * One transaction in a list. On a phone it is two lines — description and
 * amount, then date · category · account — and the whole row opens the
 * panel (the "···" menu is wide-only: Ignorar lives in the panel). On a wide
 * screen it is one dense line: date | description + account | category |
 * amount | ···; below 1200px the category moves into the meta line so the
 * description is not squeezed. The row is flat — a hairline below, a hover wash — and the
 * description is the real button whose hit area is stretched over the row,
 * so nothing interactive nests inside anything else.
 */
export function TransactionRow({
  item,
  selected = false,
  showAccount = true,
  layout,
  onSelect,
  onInclusion,
  inclusionPending = false,
}: {
  item: TransactionItem;
  /** From `useRowLayout`, read once by the list and passed down. */
  layout: RowLayout;
  /** Whether this line's panel is open. */
  selected?: boolean;
  /** Leave the account out when the whole list is already that account. */
  showAccount?: boolean;
  onSelect: (id: string) => void;
  onInclusion?: (id: string, state: TransactionInclusionState) => void;
  inclusionPending?: boolean;
}) {
  const timezone = useTimezone();
  const compact = layout === "phone";
  const ignored = item.inclusion.state === "ignored";
  const name = item.description ?? "transação sem descrição";
  const states = [
    ignored ? "is-ignored" : "",
    selected ? "is-selected" : "",
    layout === "narrow" ? "is-folded" : "",
    inclusionPending ? "is-pending" : "",
  ].join(" ");

  return (
    <div className={`transaction-row ${states}`} aria-current={selected || undefined} aria-busy={inclusionPending || undefined}>
      <span className="transaction-row-date" title={rowTime(item.occurred_at, timezone)}>
        {rowDay(item.occurred_at, timezone)}
      </span>
      <span className="transaction-row-identity">
        <button
          type="button"
          className="transaction-row-open"
          data-transaction-id={item.id}
          aria-label={`Ver detalhes de ${name}`}
          title={item.description ?? undefined}
          onClick={() => onSelect(item.id)}
        >
          {item.description ?? "Descrição não informada"}
        </button>
        <TransactionMeta item={item} showAccount={showAccount} layout={layout} />
      </span>
      {layout === "wide" && <TransactionCategory item={item} />}
      <TransactionAmount item={item} className="transaction-row-amount" />
      {!compact && (
        <TransactionActions item={item} onSelect={onSelect} onInclusion={onInclusion} pending={inclusionPending} />
      )}
    </div>
  );
}
