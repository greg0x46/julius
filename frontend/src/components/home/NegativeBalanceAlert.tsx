import { WarningOutlined } from "../ui/icons";
import { Link } from "react-router-dom";

import type { Period } from "../../hooks/usePeriod";
import { useHomeProjection } from "../../hooks/useHomeProjection";
import { formatDateOnly, formatDayMonth } from "../../presentation/dates";
import { formatReportMoney } from "../../presentation/money";

/** "21/11", or "21/11/2027" when the day is not in the year of the reference day. */
function alertDate(date: string, referenceDate: string): string {
  return date.slice(0, 4) === referenceDate.slice(0, 4) ? formatDayMonth(date) : formatDateOnly(date);
}

/**
 * The first thing to know about the forecast: the balance goes below zero.
 * `first_negative` and `lowest_balance` are the server's own readings of the
 * chart's series (from today onward), shown as they come — nothing is derived
 * here (the lowest point is left out when it is just the last day, which the
 * chart already shows). Silent while loading, on error (the chart already reports it) and when
 * the balance never goes negative.
 */
export function NegativeBalanceAlert({ period }: { period: Period }) {
  const { timeline, base, bounds, referenceDate, isLoading, error } = useHomeProjection(period);
  if (isLoading || error || !base || !bounds || base.first_negative === null) return null;

  // When the lowest point is the last day of the window, that figure is already
  // the chart's end balance: say it once.
  const lastDate = base.points.length > 0 ? base.points[base.points.length - 1].date : null;
  const lowestIsLast = base.lowest_balance.date === lastDate;

  return (
    <section className="home-alert" aria-label="Alerta de projeção">
      <WarningOutlined className="home-alert-icon" aria-hidden="true" />
      <p className="home-alert-text">
        <strong>O saldo fica negativo em {alertDate(base.first_negative, referenceDate)}</strong> se nada mudar.
        {!lowestIsLast && (
          <>
            {" "}
            Ponto mais baixo: {formatReportMoney(base.lowest_balance.balance, timeline.currencyCode)} em{" "}
            {alertDate(base.lowest_balance.date, referenceDate)}.
          </>
        )}{" "}
        <Link
          className="home-alert-link"
          to={`/transacoes?date_from=${bounds.from}&date_to=${bounds.to}`}
          aria-label="Ver transações do período"
        >
          Ver transações
        </Link>
      </p>
    </section>
  );
}
