import { ReportValuation } from "../shared/ReportValuation";
import { valueInReferenceCurrency } from "../../presentation/valuation";
import type { InvestmentOperation, InvestmentPosition, InvestmentSummary } from "../../api/contracts";
import { Money } from "../shared/Money";
import { SummaryStrip } from "../layout";
import { InvestmentYieldValue } from "./InvestmentFigures";
import { accountMovements, aggregateYield, costsByPosition, groupCost, type LinkedInvestments } from "./investmentMath";

/**
 * The page's hero figure — total value across every investment account —
 * with rendimento líquido, aportes, resgates, IR/Taxas, saldo para investir
 * and o quanto vem de instituições as secondary figures. Aportes and resgates
 * sum the whole recorded history (there is no period here), so the labels say
 * "desde o início"; the note explains the others instead of hiding them in a
 * hover-only tooltip.
 */
export function InvestmentWorkspaceSummary({
  summary,
  positions,
  operations,
  linked,
}: {
  summary: InvestmentSummary | null;
  positions: InvestmentPosition[];
  operations: InvestmentOperation[];
  /** Provider holdings by id: a synced position reads its rendimento and IR/IOF from here. */
  linked: LinkedInvestments;
}) {
  const convert = (value: string) => valueInReferenceCurrency(value, summary?.fx);
  const currencyCode = summary?.currency_code ?? "BRL";
  const movements = accountMovements(operations);
  const yieldAggregate = aggregateYield(positions, linked, costsByPosition(operations, positions, linked));
  const cost = groupCost(operations, positions.filter((position) => position.currency_code === "BRL"), linked);

  return (
    <ReportValuation valuation={summary ?? {}}>
      <SummaryStrip
        label="Valor atual"
        value={
          <Money report currencyCode={currencyCode} value={summary?.total_value ?? "0"} tone="balance" size="hero" />
        }
        items={[
          {
            label: "Rendimento líquido",
            value: (
              <InvestmentYieldValue
                currencyCode={currencyCode}
                gain={convert(yieldAggregate.gain)}
                percent={yieldAggregate.percent}
              />
            ),
          },
          {
            label: "Aportes desde o início",
            value: <Money report currencyCode={currencyCode} value={convert(movements.deposits)} tone="neutral" />,
          },
          {
            label: "Resgates desde o início",
            value: (
              <Money report currencyCode={currencyCode} value={convert(movements.withdrawals)} tone="neutral" />
            ),
          },
          {
            label: "IR/Taxas",
            value: <Money report currencyCode={currencyCode} value={convert(cost)} tone="neutral" />,
          },
          {
            label: "Saldo para investir",
            value: <Money report currencyCode={currencyCode} value={summary?.cash_balance ?? "0"} tone="neutral" />,
          },
          {
            label: "Informado pela instituição",
            value: <Money report currencyCode={currencyCode} value={summary?.synced_value ?? "0"} tone="neutral" />,
          },
        ]}
        note="Rendimento líquido: soma das posições com rendimento conhecido, já descontados IR, IOF e taxas. IR/Taxas: taxas e impostos das movimentações, mais o IR e o IOF informados pela instituição. Saldo para investir: já saiu da conta corrente e ainda não virou posição. Informado pela instituição: parte do total que vem da integração."
      />
    </ReportValuation>
  );
}
