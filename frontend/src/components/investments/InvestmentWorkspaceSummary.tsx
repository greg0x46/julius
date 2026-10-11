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
  const hasForeign = positions.some((position) => position.currency_code !== "BRL");
  const movements = accountMovements(operations);
  const yieldAggregate = aggregateYield(positions, linked, costsByPosition(operations, positions, linked));
  const cost = groupCost(operations, positions, linked);

  return (
    <SummaryStrip
      label="Valor atual"
      value={<Money value={summary?.total_value ?? "0"} tone="balance" size="hero" />}
      items={[
        {
          label: "Rendimento líquido",
          value: hasForeign ? "Indisponível em reais" : <InvestmentYieldValue gain={yieldAggregate.gain} percent={yieldAggregate.percent} />,
        },
        { label: "Aportes desde o início", value: <Money value={movements.deposits} tone="neutral" /> },
        { label: "Resgates desde o início", value: <Money value={movements.withdrawals} tone="neutral" /> },
        { label: "IR/Taxas", value: hasForeign ? "Indisponível em reais" : <Money value={cost} tone="neutral" /> },
        { label: "Saldo para investir", value: <Money value={summary?.cash_balance ?? "0"} tone="neutral" /> },
        { label: "Informado pela instituição", value: <Money value={summary?.synced_value ?? "0"} tone="neutral" /> },
      ]}
      note={`${summary?.valuation ? `Valores em reais. Câmbio observado até ${new Date(summary.valuation.valued_at).toLocaleString("pt-BR")}. ` : ""}Rendimento líquido: soma das posições com rendimento conhecido, já descontados IR, IOF e taxas. IR/Taxas: taxas e impostos das movimentações, mais o IR e o IOF informados pela instituição. Saldo para investir: já saiu da conta corrente e ainda não virou posição. Informado pela instituição: parte do total que vem da integração.${hasForeign ? " Rendimento e impostos em outras moedas disponíveis nas posições." : ""}`}
    />
  );
}
