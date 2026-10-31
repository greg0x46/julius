import { useTranslation } from "../i18n/context";
import { hasNetWorthValue } from "../api/contracts";
import { ApiError } from "../api/problems";
import { ReportValuation } from "../components/shared/ReportValuation";
import { Skeleton } from "../components/ui";

import { UnavailableState } from "../components/AsyncState";
import { EmptyState, Page, Section } from "../components/layout";
import { NetWorthBreakdownCard } from "../components/netWorth/NetWorthBreakdownCard";
import { NetWorthChart } from "../components/netWorth/NetWorthChart";
import { useNetWorth } from "../hooks/useNetWorth";

export function NetWorthPage() {
  const { t } = useTranslation();
  const netWorth = useNetWorth();
  // A single point has no evolution to plot — a line chart with one dot
  // reads as broken, not "just started". Wait for at least two valued days.
  const hasSeries = netWorth.series.filter(hasNetWorthValue).length > 1;
  const hasStaleRates = netWorth.series.some((point) => point.fx?.stale);
  const hasUnavailablePoints = netWorth.series.some((point) => point.valuation_status === "unavailable");

  // One footnote, under the chart it qualifies.
  const footnote = `${t("netWorthHistoryNote")}${hasStaleRates ? t("netWorthStaleDays") : ""}${hasUnavailablePoints ? t("netWorthGapDays") : ""}`;

  return (
    <Page title={t("netWorthTitle")} description={t("netWorthDescription")} compactMobileHeader>
      {netWorth.isLoading && (
        // Hero strip, then the chart: the shape of the loaded page.
        <div className="net-worth-page" role="status" aria-label={t("loadingNetWorth")}>
          <Skeleton active title={{ width: "30%" }} paragraph={{ rows: 2 }} />
          <Skeleton.Node active style={{ width: "100%", height: 260 }}>
            <span />
          </Skeleton.Node>
        </div>
      )}
      {netWorth.error && !netWorth.isLoading && (
        <UnavailableState onRetry={() => netWorth.refetch()}>
          {netWorth.error instanceof ApiError
            ? netWorth.error.message
            : t("netWorthLoadFailed")}
        </UnavailableState>
      )}

      {!netWorth.isLoading && !netWorth.error && !netWorth.latest && (
        <EmptyState
          title={t("noNetWorthYet")}
          hint={t("noNetWorthHint")}
        />
      )}

      {!netWorth.isLoading && !netWorth.error && netWorth.latest && (
        <ReportValuation valuation={netWorth.latest}>
          <div className="net-worth-page">
            {hasNetWorthValue(netWorth.latest) ? (
              <NetWorthBreakdownCard point={netWorth.latest} />
            ) : (
              <EmptyState
                title={t("netWorthUnavailable")}
                hint={t("netWorthUnavailableHint")}
              />
            )}
            {hasSeries ? (
              <NetWorthChart
                points={netWorth.series}
                footnote={t("valuesInCurrency", { currency: netWorth.latest.currency_code ?? "BRL", note: footnote })}
              />
            ) : (
              <Section title={t("netWorthEvolution")}>
                <EmptyState
                  title={t("noNetWorthHistory")}
                  hint={t("noNetWorthHistoryHint")}
                />
              </Section>
            )}
          </div>
        </ReportValuation>
      )}
    </Page>
  );
}
