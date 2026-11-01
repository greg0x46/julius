import { useTranslation } from "../../i18n/context";
import { Skeleton } from "../ui";

import { useMonthTimeline } from "../../hooks/useMonthTimeline";
import { ReportCurrencyContext } from "../../hooks/useReportCurrency";
import { formatDayMonth } from "../../presentation/dates";
import { problemMessage, reportingError } from "../../presentation/reporting";
import { UnavailableState } from "../AsyncState";
import { Money } from "../shared/Money";
import { StatusTag } from "../shared/StatusTag";


/**
 * The two balances that frame the daily allowance: the cash now
 * (`starting_balance`) and the server's end-of-day balance on the last day of
 * the month. Both are read as the server sends them; the second is labelled a
 * projection because it still includes what has not happened yet. Same window
 * and request as the allowance, so the figures always agree; the stale-rate
 * notice is the allowance card's, not repeated here.
 */
export function MonthBalancesCard() {
  const { t, language } = useTranslation();
  const title = t("monthlyBalanceTitle");
  const timeline = useMonthTimeline();
  const base = timeline.base;

  if (!base || reportingError(timeline.error)) {
    return (
      <section className="section home-tile home-balances" aria-label={title}>
        {timeline.isLoading && (
          <div role="status" aria-label={t("loadingMonthlyBalances")}>
            <Skeleton.Input active block style={{ height: 40 }} />
          </div>
        )}
        {!timeline.isLoading && (
          <UnavailableState onRetry={() => timeline.refetch()}>
            {problemMessage(timeline.error) ?? t("monthlyBalancesError")}
          </UnavailableState>
        )}
      </section>
    );
  }

  const last = base.points.length > 0 ? base.points[base.points.length - 1] : null;

  return (
    <ReportCurrencyContext.Provider value={timeline.currencyCode}>
      <section className="section home-tile home-balances" aria-label={title}>
        <dl className="home-balances-list">
          <div className="home-balances-item">
            <dt>{t("balanceToday")}</dt>
            <dd>
              <Money value={base.starting_balance} tone="balance" />
            </dd>
          </div>
          <div className="home-balances-item">
            <dt>
              {t("onDate").replace("{date}", formatDayMonth(timeline.monthEnd, language))} <StatusTag tone="neutral">{t("projected")}</StatusTag>
            </dt>
            <dd>{last ? <Money value={last.balance} tone="balance" /> : "—"}</dd>
          </div>
        </dl>
      </section>
    </ReportCurrencyContext.Provider>
  );
}
