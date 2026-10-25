import { InfoCircleOutlined } from "../ui/icons";

import type { Goal } from "../../api/contracts";
import { formatDateOnly } from "../../presentation/dates";
import { formatRatioPercent } from "../../presentation/investmentPeriod";
import { InvestmentHint } from "../investments/InvestmentHint";
import { Money } from "../shared/Money";

/** Where the goal lands on its target date and what it takes to get there. */
export function GoalProjection({ goal }: { goal: Goal }) {
  const projection = goal.projection;
  if (projection === null || goal.target_date === null) return null;
  const { monthly_return: monthlyReturn } = projection;

  const explanation =
    monthlyReturn === null
      ? "Sem histórico suficiente de investimentos para medir a valorização; a projeção considera valorização zero."
      : `Valorização composta medida desde ${formatDateOnly(projection.return_from ?? goal.target_date)}, sobre os investimentos com rentabilidade apurada${projection.return_partial ? " (parte das posições ficou de fora)" : ""}. Aportes no fim de cada mês. É uma estimativa: rentabilidade passada não garante a futura.`;

  return (
    <div className="goal-projection" aria-label={`Projeção de ${goal.name}`}>
      <div className="goal-projection-highlight">
        <span>Aporte mensal necessário</span>
        <strong>
          {projection.status === "behind" ? (
            <Money currencyCode={goal.currency_code} value={projection.required_monthly_contribution} tone="neutral" />
          ) : projection.status === "reached" ? (
            "Meta já atingida"
          ) : (
            "Nenhum — a valorização basta"
          )}
        </strong>
      </div>
      <dl className="goal-projection-list">
        <div>
          <dt>
            Valorização média
            <InvestmentHint label={`Sobre a projeção de ${goal.name}: ${explanation}`} content={explanation}>
              <InfoCircleOutlined className="goal-projection-info" />
            </InvestmentHint>
          </dt>
          <dd>
            {monthlyReturn === null ? (
              "Sem histórico suficiente"
            ) : (
              `${formatRatioPercent(monthlyReturn, { signed: true })} ao mês`
            )}
          </dd>
        </div>
        <div>
          <dt>Valor projetado, sem novos aportes</dt>
          <dd>
            <Money currencyCode={goal.currency_code} value={projection.projected_value} tone="neutral" /> (
            {formatRatioPercent(projection.projected_progress)} da meta)
          </dd>
        </div>
      </dl>
    </div>
  );
}
