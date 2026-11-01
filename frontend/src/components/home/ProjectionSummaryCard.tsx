import { useTranslation } from "../../i18n/context";
import { ReportValuation } from "../shared/ReportValuation";
import { QuestionCircleOutlined } from "../ui/icons";
import { Popover, Skeleton } from "../ui";
import { calendar } from "../../presentation/calendar";
import { useNavigate } from "react-router-dom";

import { problemMessage } from "../../presentation/reporting";
import type { TimelineDayPoint } from "../../api/contracts";
import { UnavailableState } from "../AsyncState";
import type { Period } from "../../hooks/usePeriod";
import { periodNavigation } from "../filters/periodNavigation";
import { ProjectionTimeline } from "../timeline/ProjectionTimeline";
import { useHomeProjection } from "../../hooks/useHomeProjection";
import { formatDateOnly } from "../../presentation/dates";
import { formatReportMoney } from "../../presentation/money";
import { compareDecimals, isZeroDecimal } from "../../presentation/decimal";
import { EmptyState, Section } from "../layout";
import { Money } from "../shared/Money";
import { useCompactScreen } from "../shared/useCompactScreen";

function lowestPoint(points: TimelineDayPoint[]): TimelineDayPoint | null {
  return points.reduce<TimelineDayPoint | null>(
    (lowest, point) => (lowest === null || compareDecimals(point.balance, lowest.balance) < 0 ? point : lowest),
    null,
  );
}

/**
 * What used to be an always-on paragraph above the figures: how to read the
 * chart. Behind a "?" it costs nothing until someone wants it; a click-popover
 * rather than a hover tooltip so it also opens by touch. The scenario wording
 * mirrors the backend, which includes every active scenario when no scenario
 * is selected — the same basis as "Resultado do período".
 */
function ChartHelp() {
  const { t } = useTranslation();
  return (
    <Popover
      trigger="click"
      placement="bottomRight"
      content={
        <p className="projection-summary-help">
          {t("chartHelp")}
        </p>
      }
    >
      <button type="button" className="projection-summary-help-button" aria-label={t("howToReadChart")}>
        <QuestionCircleOutlined aria-hidden="true" />
      </button>
    </Popover>
  );
}

/** One figure of the strip above the chart: label, balance, and the day it refers to as quiet secondary text. */
function BalanceFigure({ label, value, date }: { label: string; value: string; date?: string }) {
  const { language } = useTranslation();
  return (
    <div className="balance-strip-item">
      <dt>{label}</dt>
      <dd>
        <Money value={value} tone="balance" size="row" />
        {date !== undefined && <span className="balance-strip-date">{formatDateOnly(date, language)}</span>}
      </dd>
    </div>
  );
}

/** Nothing to plot: no balance, no day with a different balance, no entry in the window. */
function hasNoActivity(series: { starting_balance: string; points: TimelineDayPoint[]; entries: unknown[] }): boolean {
  return (
    series.entries.length === 0 &&
    isZeroDecimal(series.starting_balance) &&
    series.points.every((point) => isZeroDecimal(point.balance))
  );
}

/** Same blocks as the loaded card — the figures, then the plot — so nothing jumps when it arrives. */
function ProjectionSkeleton() {
  const { t } = useTranslation();
  return (
    <div role="status" aria-label={t("loadingBalance")}>
      <Skeleton active title={false} paragraph={{ rows: 2 }} />
      <Skeleton.Node active style={{ width: "100%", height: 240 }}>
        <span />
      </Skeleton.Node>
    </div>
  );
}

export function ProjectionSummaryCard({ period }: { period: Period }) {
  const { t, language } = useTranslation();
  const navigate = useNavigate();
  const compact = useCompactScreen();
  const { timeline, base, bounds, wholePeriod, referenceDate, isLoading, error, retry } = useHomeProjection(period);
  const lastPoint = base && base.points.length > 0 ? base.points[base.points.length - 1] : null;
  const finalPoint = lastPoint && lastPoint.date > referenceDate ? lastPoint : null;
  // The server's lowest_balance deliberately ignores the past — a low balance
  // already lived through is no warning. A window that ends today has no
  // future to warn about, so there the card answers the question the period
  // actually poses: how low did it get?
  const lowest = base ? (finalPoint ? base.lowest_balance : lowestPoint(base.points)) : null;
  const lowestTitle = finalPoint ? t("lowestProjectedBalance") : t("lowestPeriodBalance");
  // Each figure is shown once above the fold: today's cash and the month-end
  // balance are the top band's, and a forecast that goes negative already says
  // its lowest point in the alert. What is left here is what nothing else says.
  const monthEnd = calendar(referenceDate).endOf("month").format("YYYY-MM-DD");
  const shownFinal = finalPoint && finalPoint.date !== monthEnd ? finalPoint : null;
  const topBandBalance = finalPoint && finalPoint.date === monthEnd ? finalPoint.balance : null;
  const lowestRepeats =
    lowest !== null &&
    finalPoint !== null &&
    (base?.first_negative != null || (topBandBalance !== null && compareDecimals(lowest.balance, topBandBalance) === 0));
  const shownLowest = lowest && !lowestRepeats ? lowest : null;
  // Two columns hold ordinary balances; a nine-figure one needs a row of its own.
  const longestFigure = Math.max(
    ...[shownLowest?.balance, shownFinal?.balance]
      .filter((value): value is string => value !== undefined)
      .map((value) => formatReportMoney(value, timeline.currencyCode).length),
  );

  return (
    <ReportValuation valuation={timeline.valuation}>
    <Section title={t("balanceTrend")} trailing={<ChartHelp />}>
      {isLoading && <ProjectionSkeleton />}
      {error && !isLoading && (
        <UnavailableState onRetry={retry}>{problemMessage(error) ?? t("balanceLoadError")}</UnavailableState>
      )}

      {!isLoading && !error && base && bounds && hasNoActivity(base) && (
        <EmptyState
          title={t("noBalanceYet")}
          hint={t("noBalanceHint")}
        />
      )}

      {!isLoading && !error && base && bounds && !hasNoActivity(base) && (
        <div className="projection-summary">
          {base.balance_reporting?.status === "stale" && <p role="status">{t("staleConvertedBalance")}</p>}
          {/* The global control already names the chosen window, so spelling it
              out again would only repeat it — except under "todo o
              período", where the bounds come from the database and are
              worth showing. */}
          {wholePeriod && (
            <p className="projection-summary-period">{periodNavigation(bounds.from, bounds.to, language)?.label}</p>
          )}
          {(shownLowest || shownFinal) && (
            <dl className={`balance-strip ${longestFigure > 13 ? "is-long" : ""}`}>
              {shownLowest && <BalanceFigure label={lowestTitle} value={shownLowest.balance} date={shownLowest.date} />}
              {shownFinal && (
                <BalanceFigure label={t("endOfPeriodBalance")} value={shownFinal.balance} date={shownFinal.date} />
              )}
            </dl>
          )}
          <ProjectionTimeline
            series={base}
            referenceDate={referenceDate}
            height={compact ? 340 : 320}
            onSelectDay={(date) => navigate(`/transacoes?date_from=${date}&date_to=${date}`)}
          />
        </div>
      )}
    </Section>
    </ReportValuation>
  );
}
