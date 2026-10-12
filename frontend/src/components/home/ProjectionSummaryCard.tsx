import { QuestionCircleOutlined } from "@ant-design/icons";
import { Popover, Skeleton } from "antd";
import { useMemo } from "react";
import { useNavigate } from "react-router-dom";

import type { TimelineDayPoint } from "../../api/contracts";
import { UnavailableState } from "../AsyncState";
import type { Period } from "../../hooks/usePeriod";
import { periodNavigation } from "../filters/periodNavigation";
import { ProjectionTimeline } from "../timeline/ProjectionTimeline";
import { useHomePeriodBounds } from "../../hooks/useHomePeriodBounds";
import { useTimeline } from "../../hooks/useTimeline";
import { useToday } from "../../hooks/useToday";
import { formatDateOnly } from "../../presentation/dates";
import { formatBRL } from "../../presentation/money";
import { EmptyState, Section } from "../layout";
import { Money } from "../shared/Money";

const dateFormat = "YYYY-MM-DD";

function lowestPoint(points: TimelineDayPoint[]): TimelineDayPoint | null {
  return points.reduce<TimelineDayPoint | null>(
    (lowest, point) => (lowest === null || Number(point.balance) < Number(lowest.balance) ? point : lowest),
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
  return (
    <Popover
      trigger="click"
      placement="bottomRight"
      content={
        <p className="projection-summary-help">
          Saldo realizado até hoje e, daí em diante, o que já está previsto, incluindo cenários ativos. Selecione
          um dia até hoje para ver as transações dele.
        </p>
      }
    >
      <button type="button" className="projection-summary-help-button" aria-label="Como ler este gráfico">
        <QuestionCircleOutlined aria-hidden="true" />
      </button>
    </Popover>
  );
}

/** One figure of the strip above the chart: label, balance, and the day it refers to as quiet secondary text. */
function BalanceFigure({ label, value, date }: { label: string; value: string; date?: string }) {
  return (
    <div className="balance-strip-item">
      <dt>{label}</dt>
      <dd>
        <Money value={value} tone="balance" size="row" />
        {date !== undefined && <span className="balance-strip-date">{formatDateOnly(date)}</span>}
      </dd>
    </div>
  );
}

/** Nothing to plot: no balance, no day with a different balance, no entry in the window. */
function hasNoActivity(series: { starting_balance: string; points: TimelineDayPoint[]; entries: unknown[] }): boolean {
  return (
    series.entries.length === 0 &&
    Number(series.starting_balance) === 0 &&
    series.points.every((point) => Number(point.balance) === 0)
  );
}

/** Same blocks as the loaded card — three figures, then the plot — so nothing jumps when it arrives. */
function ProjectionSkeleton() {
  return (
    <div role="status" aria-label="Carregando o saldo">
      <Skeleton active title={false} paragraph={{ rows: 2 }} />
      <Skeleton.Node active style={{ width: "100%", height: 240 }}>
        <span />
      </Skeleton.Node>
    </div>
  );
}

export function ProjectionSummaryCard({ period }: { period: Period }) {
  const today = useToday();
  const navigate = useNavigate();

  // "Todo o período" is the one window whose bounds live in the database — the
  // oldest transaction, the last planned installment — so it stays unresolved
  // until useHomePeriodBounds answers.
  const { bounds, wholePeriod, isLoading: rangeLoading, error: rangeError, refetch: refetchRange } =
    useHomePeriodBounds(period);
  const referenceDate = today.format(dateFormat);
  const params = useMemo(
    () => ({
      // referenceDate is always today, never the start of the window: it is
      // what anchors the series to the real balance, so days before it read as
      // history and days after it as projection.
      referenceDate,
      from: bounds ? bounds.from : referenceDate,
      to: bounds ? bounds.to : referenceDate,
    }),
    [referenceDate, bounds],
  );
  const timeline = useTimeline(params, bounds !== null);
  const base = timeline.base;
  const isLoading = timeline.isLoading || rangeLoading;
  const error = timeline.error ?? rangeError;
  const retry = () => {
    if (rangeError) refetchRange();
    if (timeline.error) timeline.refetch();
  };
  const lastPoint = base && base.points.length > 0 ? base.points[base.points.length - 1] : null;
  const finalPoint = lastPoint && lastPoint.date > referenceDate ? lastPoint : null;
  // The server's lowest_balance deliberately ignores the past — a low balance
  // already lived through is no warning. A window that ends today has no
  // future to warn about, so there the card answers the question the period
  // actually poses: how low did it get?
  const lowest = base ? (finalPoint ? base.lowest_balance : lowestPoint(base.points)) : null;
  const lowestTitle = finalPoint ? "Menor saldo previsto" : "Menor saldo no período";
  // Three columns hold ordinary balances; a nine-figure one needs a row of its own.
  const longestFigure = base
    ? Math.max(
        ...[base.starting_balance, lowest?.balance, finalPoint?.balance]
          .filter((value): value is string => value !== undefined)
          .map((value) => formatBRL(value).length),
      )
    : 0;

  return (
    <Section title="Evolução do saldo" trailing={<ChartHelp />}>
      {isLoading && <ProjectionSkeleton />}
      {error && !isLoading && (
        <UnavailableState onRetry={retry}>Não foi possível carregar o saldo.</UnavailableState>
      )}

      {!isLoading && !error && base && bounds && hasNoActivity(base) && (
        <EmptyState
          title="Ainda não há saldo para mostrar"
          hint="Conecte um banco ou importe um extrato para acompanhar a evolução do saldo."
        />
      )}

      {!isLoading && !error && base && bounds && !hasNoActivity(base) && (
        <div className="projection-summary">
          {/* The global control already names the chosen window, so spelling it
              out again would only repeat it — except under "todo o
              período", where the bounds come from the database and are
              worth showing. */}
          {wholePeriod && (
            <p className="projection-summary-period">{periodNavigation(bounds.from, bounds.to)?.label}</p>
          )}
          <dl className={`balance-strip ${longestFigure > 13 ? "is-long" : ""}`}>
            <BalanceFigure label="Saldo hoje" value={base.starting_balance} />
            {lowest && <BalanceFigure label={lowestTitle} value={lowest.balance} date={lowest.date} />}
            {finalPoint && (
              <BalanceFigure label="Saldo no fim do período" value={finalPoint.balance} date={finalPoint.date} />
            )}
          </dl>
          <ProjectionTimeline
            series={base}
            referenceDate={referenceDate}
            height={280}
            onSelectDay={(date) => navigate(`/transacoes?date_from=${date}&date_to=${date}`)}
          />
        </div>
      )}
    </Section>
  );
}
