import { ReportValuation } from "../shared/ReportValuation";
import { Skeleton } from "../ui";

import { problemMessage, reportingError } from "../../presentation/reporting";
import { useMonthTimeline } from "../../hooks/useMonthTimeline";
import { dailyAllowance } from "../../presentation/dailyAllowance";
import { formatDayMonth } from "../../presentation/dates";
import { UnavailableState } from "../AsyncState";
import { Money } from "../shared/Money";
import { HomeTile } from "./HomeTile";

const title = "Pode gastar por dia";

/**
 * What can be spent per day until month-end without the projected balance
 * going negative. Always this month from today, whatever period Home shows;
 * derived on every render from the timeline, never stored. It is Home's one
 * hero figure: the first answer to "will I be OK?".
 */
export function DailyAllowanceCard() {
  const timeline = useMonthTimeline();
  const base = timeline.base;

  // Not `timeline.error`: a failed background refetch keeps the last data, which is still the best answer.
  if (!base || reportingError(timeline.error)) {
    return (
      <HomeTile label={title} className="home-hero">
        {timeline.isLoading && (
          <div role="status" aria-label="Carregando o gasto diário disponível">
            <Skeleton.Input active block style={{ height: 40 }} />
          </div>
        )}
        {!timeline.isLoading && (
          <UnavailableState onRetry={() => timeline.refetch()}>
            {problemMessage(timeline.error) ?? "Não foi possível calcular o gasto diário disponível."}
          </UnavailableState>
        )}
      </HomeTile>
    );
  }

  return (
    <ReportValuation valuation={timeline.valuation}>
      <HomeTile label={`${title} até ${formatDayMonth(timeline.monthEnd)}`} className="home-hero">
        {base.balance_reporting?.status === "stale" && <p role="status">Saldo convertido com cotações desatualizadas.</p>}
        <p className="daily-allowance-value">
          <Money value={dailyAllowance(base.points, base.starting_balance)} tone="neutral" />
          <span className="daily-allowance-unit"> / dia</span>
        </p>
        <p className="daily-allowance-note">
          Já descontando o que está previsto até o fim do mês. Não é orçamento nem reserva.
        </p>
      </HomeTile>
    </ReportValuation>
  );
}
