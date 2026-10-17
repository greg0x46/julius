import type { ReactNode } from "react";

import type { Investment, InvestmentTransaction } from "../../api/contracts";
import { formatOptionalDay } from "../../presentation/dates";
import {
  formatDecimal,
  investmentTypeLabel,
  investmentYield,
  netContributed,
  yieldUnavailable,
} from "../../presentation/investmentLabels";
import { Section, SummaryStrip } from "../layout";
import { Money } from "../shared/Money";

/**
 * The investment's summary: its saldo as the hero, rendimento / aportes
 * líquidos / atualização beside it, and everything else the institution tells
 * us as a plain list of facts under it. Where the rendimento comes from (or
 * why there is none) is a line of text, not a hover-only tooltip.
 */
export function InvestmentHeaderCard({
  investment,
  transactions,
}: {
  investment: Investment;
  transactions: InvestmentTransaction[];
}) {
  const yieldEstimate = investmentYield(investment);
  const unavailable = yieldUnavailable(investment);
  // A history the backend judged too partial to net is too partial to show a
  // total for either. Without this the page contradicts itself: "Rendimento:
  // Histórico incompleto" directly above a confident "Aportes líquidos"
  // netted from the very history that was just rejected. The backend owns
  // that judgement — it has the quantity evidence (cotas sold against cotas
  // bought) this component does not.
  const historyIsTrustworthy = investment.yield_unavailable_reason !== "historico_incompleto";
  const contributed =
    transactions.length > 0 && historyIsTrustworthy ? netContributed(transactions) : null;

  const yieldNote =
    yieldEstimate === null
      ? unavailable.hint === ""
        ? undefined
        : unavailable.hint
      : yieldEstimate.source === "informado"
        ? "Rendimento informado pela instituição."
        : "Rendimento calculado a partir do histórico de aplicações e resgates.";

  const facts: { label: string; value: ReactNode }[] = [
    { label: "Tipo", value: investmentTypeLabel(investment.investment_type) },
    ...(investment.source_display_name !== null
      ? [{ label: "Instituição", value: investment.source_display_name }]
      : []),
    ...(investment.amount_original !== null
      ? [{ label: "Valor aplicado", value: <Money value={investment.amount_original} tone="neutral" /> }]
      : []),
    ...(investment.taxes !== null
      ? [{ label: "IR retido", value: <Money value={investment.taxes} tone="neutral" /> }]
      : []),
    ...(investment.taxes2 !== null
      ? [{ label: "IOF retido", value: <Money value={investment.taxes2} tone="neutral" /> }]
      : []),
    ...(investment.annual_rate !== null ? [{ label: "Taxa anual", value: `${formatDecimal(investment.annual_rate)}%` }] : []),
    ...(investment.last_twelve_months_rate !== null
      ? [{ label: "Rentabilidade 12 meses", value: `${formatDecimal(investment.last_twelve_months_rate)}%` }]
      : []),
    ...(investment.rate !== null
      ? [
          {
            label: "Taxa contratada",
            value: `${formatDecimal(investment.rate)}${investment.rate_type !== null ? ` ${investment.rate_type}` : ""}`,
          },
        ]
      : []),
    ...(investment.issuer !== null ? [{ label: "Emissor", value: investment.issuer }] : []),
    ...(investment.due_date !== null
      ? [{ label: "Vencimento", value: formatOptionalDay(investment.due_date) }]
      : []),
  ];

  return (
    <>
      <SummaryStrip
        label="Saldo"
        value={
          investment.balance !== null ? <Money value={investment.balance} tone="balance" size="hero" /> : "—"
        }
        items={[
          {
            label: "Rendimento",
            value: yieldEstimate !== null ? <Money value={yieldEstimate.value} tone="result" /> : unavailable.label,
          },
          ...(contributed !== null
            ? [{ label: "Aportes líquidos", value: <Money value={contributed} tone="neutral" /> }]
            : []),
          { label: "Atualizado em", value: formatOptionalDay(investment.as_of_date) },
        ]}
        note={yieldNote}
      />
      <Section title="Dados do investimento">
        <dl className="investment-facts">
          {facts.map((fact) => (
            <div key={fact.label} className="investment-fact">
              <dt>{fact.label}</dt>
              <dd>{fact.value}</dd>
            </div>
          ))}
        </dl>
      </Section>
    </>
  );
}
