import { useState } from "react";

import type { InvestmentPeriod, InvestmentPeriodPart, InvestmentPeriodPosition } from "../../api/contracts";
import { formatDateOnly } from "../../presentation/dates";
import { isZeroDecimal } from "../../presentation/decimal";
import { partialReasons } from "../../presentation/investmentPeriod";
import { SummaryStrip } from "../layout";
import { Money } from "../shared/Money";
import { StatusTag } from "../shared/StatusTag";
import { InvestmentHint } from "./InvestmentHint";
import { InvestmentYieldValue } from "./InvestmentYieldValue";

const partIsEmpty = (part: InvestmentPeriodPart) =>
  [part.start_value, part.end_value, part.contributions, part.withdrawals, part.yield, part.capital_base].every(isZeroDecimal);

const sourceNote = {
  manual: "Somente posições manuais.",
  synced: "Somente posições sincronizadas.",
} as const;

/**
 * The period's result, as the backend states it: patrimônio at the end of the
 * period as the hero, rendimento and rentabilidade beside it, aportes,
 * resgates and custos as secondary figures. Nothing here is summed or divided
 * in the browser; positions the backend could not measure are named, never
 * counted as zero.
 */
export function InvestmentPeriodSummary({
  data,
  positions,
  updating,
}: {
  data: InvestmentPeriod;
  positions: InvestmentPeriodPosition[];
  updating: boolean;
}) {
  const [explained, setExplained] = useState(false);
  const { totals } = data;
  const reasons = partialReasons(positions);
  const items = [
    { label: "Resultado do período", value: <InvestmentYieldValue gain={totals.yield} rate={null} /> },
    {
      label: "Rentabilidade",
      value:
        totals.yield_rate === null ? (
          <InvestmentHint
            label="Rentabilidade indisponível: não há capital positivo investido no período."
            content="Não há capital positivo investido no período para calcular a rentabilidade."
          >
            —
          </InvestmentHint>
        ) : (
          <InvestmentYieldValue gain={null} rate={totals.yield_rate} />
        ),
    },
    { label: "Aportes", value: <Money value={totals.contributions} tone="neutral" /> },
    { label: "Resgates", value: <Money value={totals.withdrawals} tone="neutral" /> },
    ...(isZeroDecimal(totals.costs) ? [] : [{ label: "Custos", value: <Money value={totals.costs} tone="neutral" /> }]),
  ];

  const notes = [
    data.source !== null ? sourceNote[data.source] : null,
    "O saldo para investir (caixa) das contas não entra nestes valores.",
    isZeroDecimal(totals.provisioned_taxes)
      ? null
      : "Em posições sincronizadas o rendimento é antes do IR/IOF provisionado pela instituição, que não é deduzido.",
    data.to_clamped ? `Período em andamento: considerado até hoje, ${formatDateOnly(data.to)}.` : null,
    totals.positions_partial_start > 0
      ? `${totals.positions_partial_start} ${totals.positions_partial_start === 1 ? "posição começa" : "posições começam"} depois do primeiro dia do período, por falta de preço nesse dia.`
      : null,
  ].filter(Boolean);

  return (
    <SummaryStrip
      className="investment-period-summary"
      label={`Patrimônio em ${formatDateOnly(data.to)}`}
      value={<Money value={totals.end_value} tone="balance" size="hero" />}
      busy={updating}
      items={items}
      note={notes.join(" ")}
    >
      {totals.partial && (
        <InvestmentHint
          label={`Parcial: ${totals.positions_unavailable} de ${totals.positions_total} posições sem histórico suficiente. ${reasons.join(", ")}`}
          content={
            <>
              <p>As posições abaixo ficaram de fora dos valores e da rentabilidade; não foram contadas como zero.</p>
              <ul>
                {reasons.map((reason) => (
                  <li key={reason}>{reason}</li>
                ))}
              </ul>
            </>
          }
        >
          <StatusTag tone="warning">
            Parcial: {totals.positions_unavailable} de {totals.positions_total} posições sem histórico suficiente
          </StatusTag>
        </InvestmentHint>
      )}
      <button
        type="button"
        className="investment-reconcile-toggle"
        aria-expanded={explained}
        aria-controls="investment-reconcile"
        onClick={() => setExplained((open) => !open)}
      >
        Inicial + aportes − resgates + rendimento = final
      </button>
      {explained && (
        <>
        <dl id="investment-reconcile" className="investment-reconcile">
          <div>
            <dt>Valor inicial ({data.from === null ? "início de cada posição" : `antes de ${formatDateOnly(data.from)}`})</dt>
            <dd><Money value={totals.start_value} tone="neutral" /></dd>
          </div>
          <div>
            <dt>+ Aportes</dt>
            <dd><Money value={totals.contributions} tone="neutral" /></dd>
          </div>
          <div>
            <dt>− Resgates</dt>
            <dd><Money value={totals.withdrawals} tone="neutral" /></dd>
          </div>
          <div>
            <dt>+ Rendimento</dt>
            <dd><Money value={totals.yield} tone="result" /></dd>
          </div>
          <div className="investment-reconcile-total">
            <dt>= Valor final ({formatDateOnly(data.to)})</dt>
            <dd><Money value={totals.end_value} tone="balance" /></dd>
          </div>
        </dl>
        <dl className="investment-reconcile" role="group" aria-label="De onde vem o resultado do período">
          <div className="investment-reconcile-total">
            <dt>Resultado do período = mantidas + encerradas</dt>
            <dd><InvestmentYieldValue gain={totals.yield} rate={totals.yield_rate} /></dd>
          </div>
          <div>
            <dt>Posições mantidas em {formatDateOnly(data.to)}</dt>
            <dd><InvestmentYieldValue gain={totals.held.yield} rate={totals.held.yield_rate} /></dd>
          </div>
          {!partIsEmpty(totals.closed) && (
            <div>
              <dt>Encerradas no período (resultado realizado)</dt>
              <dd><InvestmentYieldValue gain={totals.closed.yield} rate={totals.closed.yield_rate} /></dd>
            </div>
          )}
        </dl>
        </>
      )}
    </SummaryStrip>
  );
}
