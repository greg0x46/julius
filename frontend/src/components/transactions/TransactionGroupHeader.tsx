import { dateFormatter } from "../../i18n/dateFormats";
import { useTranslation } from "../../i18n/context";
import { translate, type Language } from "../../i18n/messages";
import { reportingLabel, reportingTotals } from "../../presentation/reporting";
import type { TransactionGroup as Group } from "../../api/contracts";
import { Money } from "../shared/Money";


function dateOnly(value: string): Date {
  return new Date(`${value}T12:00:00Z`);
}

function groupLabel(group: Group, language: Language): string {
  const shortDate = dateFormatter(language, { day: "numeric", month: "short", timeZone: "UTC" });
  if (group.kind === "none") return "";
  if (group.kind === "undated") return translate(language, "undated");
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
  const { t, language } = useTranslation();
  const total = reportingTotals(group.reporting, group.totals);
  const split = group.has_items_before || group.has_items_after;
  return (
    <header className="transaction-group-header">
      <div className="transaction-group-title">
        <h2 id={`group-${group.key}`}>{groupLabel(group, language)}</h2>
        {split && <small>{t("periodContinues")}</small>}
      </div>
      <dl className="transaction-group-result">
        <div>
          <dt>{t("result")}</dt>
          <dd>
            {total ? <Money value={total.balance} currencyCode={total.currency_code} report tone="result" /> : <span>{group.reporting ? reportingLabel(group.reporting, language) : t("totalUnavailable")}</span>}
          </dd>
        </div>
      </dl>
    </header>
  );
}
