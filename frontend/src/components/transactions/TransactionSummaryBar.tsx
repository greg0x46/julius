import { useTranslation } from "../../i18n/context";
import { DownOutlined } from "../ui/icons";
import { useId, useState } from "react";

import { ReportingDetails } from "../shared/ReportingDetails";
import { reportingLabel, reportingTotals } from "../../presentation/reporting";
import type { CurrencyTotals, ReportingSummary } from "../../api/contracts";
import { formatReportMoney } from "../../presentation/money";
import { DataCardSummary } from "../layout/DataCard";
import { Money } from "../shared/Money";
import { useCompactScreen } from "../shared/useCompactScreen";

/**
 * The summary strip of the transactions DataCard: how many rows and the
 * result of everything the filters matched, with the inflow / outflow
 * figures and the note about what the totals include.
 *
 * On a phone the strip stays one ~40px line — "19 transações · Resultado
 * -R$ 2.656,18" — with a chevron that unfolds Entradas, Saídas and the note;
 * the figures that explain the result are not worth a permanent half
 * screen of a sticky bar. On a wide screen everything shows, in one quiet
 * line without icons or big figures.
 */
export function TransactionSummaryBar({
  totals,
  reporting,
  totalItems,
  busy,
}: {
  totals: CurrencyTotals[];
  reporting?: ReportingSummary;
  totalItems: number;
  busy?: boolean;
}) {
  const { t, language } = useTranslation();
  const compact = useCompactScreen();
  const [expanded, setExpanded] = useState(false);
  const brl = reportingTotals(reporting, totals);
  const showDetails = !compact || expanded;
  const detailsId = useId();

  return (
    <DataCardSummary
      label={t("financialSummary")}
      busy={busy}
      note={showDetails ? t("financialSummaryHint") : undefined}
    >
      <div className={`transaction-summary-bar ${expanded ? "is-expanded" : ""}`}>
        <p className="transaction-summary-line">
          <span className="transaction-summary-count">
            {t(totalItems === 1 ? "transactionCountOne" : "transactionCountMany", { count: totalItems.toLocaleString(language) })}
          </span>
          <span className="transaction-summary-figure transaction-summary-result">
            <span className="transaction-summary-label">{t("result")}</span>
            {brl ? <Money value={brl.balance} currencyCode={brl.currency_code} report tone="result" className="transaction-summary-value" /> : <span role="status">{reporting ? reportingLabel(reporting, language) : t("totalUnavailable")}</span>}
          </span>
          {!compact && brl && (
            <>
              <span className="transaction-summary-figure">
                <span className="transaction-summary-label">{t("inflows")}</span>
                <span className="transaction-summary-value">{formatReportMoney(brl.inflow, brl.currency_code)}</span>
              </span>
              <span className="transaction-summary-figure">
                <span className="transaction-summary-label">{t("outflows")}</span>
                <span className="transaction-summary-value">{formatReportMoney(brl.outflow, brl.currency_code)}</span>
              </span>
            </>
          )}
        </p>

        {compact && (
          <button
            type="button"
            className="transaction-summary-toggle"
            aria-expanded={expanded}
            aria-controls={expanded ? detailsId : undefined}
            aria-label={expanded ? t("hideSummary") : t("showSummary")}
            onClick={() => setExpanded((value) => !value)}
          >
            <DownOutlined aria-hidden="true" />
          </button>
        )}

        {compact && expanded && brl && (
          <dl id={detailsId} className="transaction-summary-details">
            <div>
              <dt>{t("inflows")}</dt>
              <dd>{formatReportMoney(brl.inflow, brl.currency_code)}</dd>
            </div>
            <div>
              <dt>{t("outflows")}</dt>
              <dd>{formatReportMoney(brl.outflow, brl.currency_code)}</dd>
            </div>
          </dl>
        )}
      </div>
      <ReportingDetails report={reporting} totals={totals} />
    </DataCardSummary>
  );
}
