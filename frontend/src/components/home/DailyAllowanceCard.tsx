import { Skeleton } from "antd";
import { useMemo } from "react";

import { useTimeline } from "../../hooks/useTimeline";
import { useToday } from "../../hooks/useToday";
import { dailyAllowance } from "../../presentation/dailyAllowance";
import { UnavailableState } from "../AsyncState";
import { Section } from "../layout";
import { Money } from "../shared/Money";

const dateFormat = "YYYY-MM-DD";
const title = "Disponível por dia";

/**
 * What can be spent per day until month-end without the projected balance
 * going negative. Always this month from today, whatever period Home shows;
 * derived on every render from the timeline, never stored. A compact figure,
 * not a second hero: the page's one hero is the period result above it.
 */
export function DailyAllowanceCard() {
  const today = useToday();
  const referenceDate = today.format(dateFormat);
  const monthEnd = today.endOf("month").format(dateFormat);
  // from == referenceDate: one point per day, today first, so the point's
  // position is the number of days of spending it has to cover. React Query
  // keys by value, so this shares a request with any card whose window is the
  // same; the usual Home period starts at the first of the month, so it rarely is.
  const params = useMemo(
    () => ({ referenceDate, from: referenceDate, to: monthEnd }),
    [referenceDate, monthEnd],
  );
  const timeline = useTimeline(params);
  const base = timeline.base;

  // Not `timeline.error`: a failed background refetch keeps the last data, which is still the best answer.
  if (!base) {
    return (
      <Section title={title}>
        {timeline.isLoading && (
          <div role="status" aria-label="Carregando o gasto diário disponível">
            <Skeleton.Input active block style={{ height: 24 }} />
          </div>
        )}
        {!timeline.isLoading && (
          <UnavailableState onRetry={() => timeline.refetch()}>
            Não foi possível calcular o gasto diário disponível.
          </UnavailableState>
        )}
      </Section>
    );
  }

  return (
    <Section title={title}>
      <p className="daily-allowance-value">
        <Money value={dailyAllowance(base.points)} tone="neutral" size="row" />
        <span className="daily-allowance-unit"> / dia</span>
      </p>
      <p className="daily-allowance-note">Disponível para gastar até o fim do mês</p>
    </Section>
  );
}
