import type { TransactionGroup as Group } from "../../api/contracts";
import { Money } from "../shared/Money";

const shortDate = new Intl.DateTimeFormat("pt-BR", {
  day: "numeric",
  month: "short",
  timeZone: "UTC",
});

function dateOnly(value: string): Date {
  return new Date(`${value}T12:00:00Z`);
}

function groupLabel(group: Group): string {
  if (group.kind === "none") return "";
  if (group.kind === "undated") return "Sem data";
  if (group.kind === "day") return shortDate.format(dateOnly(group.start_date!));
  if (group.start_date && group.end_date) {
    return `${shortDate.format(dateOnly(group.start_date))} – ${shortDate.format(dateOnly(group.end_date))}`;
  }
  return group.key;
}

/**
 * A period's heading: the range on the left, its result on the right, a
 * hairline underneath. Quieter than the page total in TransactionSummaryBar
 * but still the first thing to find when skimming down a long list.
 */
export function TransactionGroupHeader({ group }: { group: Group }) {
  const total = group.report_totals ?? group.totals.find((total) => total.currency_code === "BRL");
  const balance = total?.balance ?? "0";
  const split = group.has_items_before || group.has_items_after;
  return (
    <header className="transaction-group-header">
      <div className="transaction-group-title">
        <h2 id={`group-${group.key}`}>{groupLabel(group)}</h2>
        {split && <small>Período continua em outra página</small>}
      </div>
      <dl className="transaction-group-result">
        <div>
          <dt>Resultado</dt>
          <dd>
            <Money value={balance} currencyCode={total?.currency_code ?? "BRL"} tone="result" />
          </dd>
        </div>
      </dl>
    </header>
  );
}
