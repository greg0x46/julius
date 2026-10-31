import { useTranslation } from "../../i18n/context";
import { Skeleton } from "../ui";

import { hasNetWorthValue } from "../../api/contracts";
import { ReportCurrencyContext } from "../../hooks/useReportCurrency";
import { useNetWorth } from "../../hooks/useNetWorth";
import { formatDayMonth } from "../../presentation/dates";
import { netWorthTrend, sparklinePoints } from "../../presentation/netWorthTrend";
import { problemMessage } from "../../presentation/reporting";
import { balanceChartColor } from "../../presentation/chartColors";
import { UnavailableState } from "../AsyncState";
import { Money } from "../shared/Money";
import { HomeTile } from "./HomeTile";

const route = "/patrimonio";
const sparkWidth = 200;
const sparkHeight = 36;

/**
 * Wealth (cash plus investments) today with its recent evolution, as a link to
 * its page. Reads the same `/api/net-worth` series as that page; the change is
 * the exact difference between two valued days (see `netWorthTrend`), in the
 * reference currency the response declares. A day that cannot be valued is
 * said so, never zero.
 */
export function NetWorthSummaryCard() {
  const { t, language } = useTranslation();
  const title = t("netWorth");
  const netWorth = useNetWorth();

  if (netWorth.isLoading) {
    return (
      <HomeTile label={title}>
        <div role="status" aria-label={t("loadingWealth")}>
          <Skeleton active title={false} paragraph={{ rows: 2 }} />
        </div>
      </HomeTile>
    );
  }
  if (netWorth.error || !netWorth.latest) {
    return (
      <HomeTile label={title}>
        <UnavailableState onRetry={() => netWorth.refetch()}>
          {problemMessage(netWorth.error) ?? t("wealthLoadError")}
        </UnavailableState>
      </HomeTile>
    );
  }

  const latest = netWorth.latest;
  if (!hasNetWorthValue(latest)) {
    return (
      <HomeTile label={title} to={route}>
        <span className="home-tile-note">{t("wealthRatesUnavailable")}</span>
      </HomeTile>
    );
  }

  const trend = netWorthTrend(netWorth.series, latest);
  const since = trend ? formatDayMonth(trend.since, language) : null;

  return (
    <ReportCurrencyContext.Provider value={latest.currency_code ?? "BRL"}>
      <HomeTile label={title} to={route}>
        <span className="home-tile-line">
          <span className="home-tile-figure">
            <Money value={latest.total} tone="balance" />
          </span>
          {trend && trend.values.length > 1 && (
            <svg
              className="home-sparkline"
              viewBox={`0 0 ${sparkWidth} ${sparkHeight}`}
              preserveAspectRatio="none"
              role="img"
              aria-label={t("wealthTrendSince").replace("{date}", since ?? "")}
            >
              <polyline
                points={sparklinePoints(trend.values, sparkWidth, sparkHeight)}
                fill="none"
                stroke={balanceChartColor.realized}
                strokeWidth={2}
                strokeLinejoin="round"
                vectorEffect="non-scaling-stroke"
              />
            </svg>
          )}
          <span className={`home-tile-note ${trend ? "" : "is-optional"}`}>
            {trend ? (
              <>
                <Money value={trend.change} tone="result" /> {t("since")} {since}
              </>
            ) : (
              t("wealthInsufficientHistory")
            )}
            {latest.fx?.stale && t("staleRates")}
          </span>
        </span>
      </HomeTile>
    </ReportCurrencyContext.Provider>
  );
}
