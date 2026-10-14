import dayjs from "dayjs";
import { useMemo } from "react";
import { Link } from "react-router-dom";

import { ReportingDetails } from "../shared/ReportingDetails";
import { reportingError } from "../../presentation/reporting";
import type { Period } from "../../hooks/usePeriod";
import { useHomePeriodBounds } from "../../hooks/useHomePeriodBounds";
import { useTimeline } from "../../hooks/useTimeline";
import { formatBRL } from "../../presentation/money";
import { Skeleton } from "antd";

import { UnavailableState } from "../AsyncState";
import { Section, SummaryStrip } from "../layout";
import { Money } from "../shared/Money";

const dateFormat = "YYYY-MM-DD";

// Entradas and saídas arrive as unsigned magnitudes, so the direction is
// stated; a zero total stays unsigned and untinted ("+R$ 0,00" would claim
// something happened).
function FlowFigure({ value, direction }: { value: string; direction: "inflow" | "outflow" }) {
  const zero = /^-?0*(\.0*)?$/.test(value);
  return zero ? <Money value={value} tone="neutral" /> : <Money value={value} tone="flow" direction={direction} />;
}

/**
 * Home's hero: the period's result as the page's one big figure, with
 * entradas and saídas as a two-column definition strip underneath (each a
 * link to the matching transactions). There is no share bar between them:
 * the two figures already say it, and a green/red bar would paint an ordinary
 * outflow in the colour reserved for trouble. The scenario caveat is small
 * print under the figure, never a sentence above it.
 */
export function PeriodBalanceCard({ period }: { period: Period }) {
  const { bounds, isLoading: rangeLoading, error: rangeError, refetch: refetchRange } =
    useHomePeriodBounds(period);
  // Only the day span matters here, not the exact instant, so this needs
  // none of ProjectionSummaryCard's midnight-rollover handling.
  const referenceDate = useMemo(() => dayjs().format(dateFormat), []);
  const params = useMemo(
    () => ({
      referenceDate,
      from: bounds ? bounds.from : referenceDate,
      to: bounds ? bounds.to : referenceDate,
    }),
    [referenceDate, bounds],
  );
  const timeline = useTimeline(params, bounds !== null);
  // periodTotals — not the daily points — is what matches the transactions
  // page: it walks every real (and, with active scenarios, projected) entry
  // in the window, the same income/expense basis Query's totals use
  // (Included, not just MovesCash), so a same-owner transfer or a settled
  // credit-card purchase is counted exactly once, on the right side. The
  // points, by contrast, drop a settled purchase entirely once its bill is
  // paid (see internal/timeline's cashEntries) — correct for the balance
  // curve, wrong for an entradas/saídas total.
  const totals = timeline.periodTotals;
  const inflow = totals?.income ?? null;
  const outflow = totals?.expense ?? null;
  const balance = totals?.result ?? null;
  const empty = inflow === "0.00" && outflow === "0.00";
  const isLoading = timeline.isLoading || rangeLoading;
  const error = timeline.error ?? rangeError;

  if (isLoading || error || !bounds || balance === null || inflow === null || outflow === null) {
    return (
      <Section title="Resultado do período">
        {isLoading && (
          // The shape of the strip it becomes: hero figure, then the two figures.
          <div role="status" aria-label="Carregando o resultado do período">
            <Skeleton.Input active block style={{ height: 36, marginBottom: 12 }} />
            <Skeleton active title={false} paragraph={{ rows: 1 }} />
          </div>
        )}
        {!isLoading && (
          <UnavailableState
            onRetry={() => {
              if (rangeError) refetchRange();
              if (timeline.error) timeline.refetch();
            }}
          >
            {reportingError(timeline.error)?.message ?? "Não foi possível carregar o resultado do período."}
            <ReportingDetails report={reportingError(timeline.error)?.problem?.reporting} totals={reportingError(timeline.error)?.problem?.totals} />
          </UnavailableState>
        )}
      </Section>
    );
  }

  const transactionsLink = (classification: "inflow" | "outflow") =>
    `/transacoes?date_from=${bounds.from}&date_to=${bounds.to}&classification=${classification}`;

  return (
    <>
      <SummaryStrip
        className="period-balance"
        label="Resultado do período"
        value={<Money value={balance} tone="result" size="hero" />}
        note={empty ? "Nenhuma movimentação no período" : "Entradas menos saídas, incluindo cenários ativos"}
        items={[
          {
            label: "Entradas",
            value: (
              <Link
                to={transactionsLink("inflow")}
                aria-label={`Entradas: ${formatBRL(inflow)}. Ver transações`}
              >
                <FlowFigure value={inflow} direction="inflow" />
              </Link>
            ),
          },
          {
            label: "Saídas",
            value: (
              <Link
                to={transactionsLink("outflow")}
                aria-label={`Saídas: ${formatBRL(outflow)}. Ver transações`}
              >
                <FlowFigure value={outflow} direction="outflow" />
              </Link>
            ),
          },
        ]}
      />
    <ReportingDetails report={totals?.reporting} totals={totals?.totals} />
    </>
  );
}
