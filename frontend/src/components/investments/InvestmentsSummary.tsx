import type { Investment, InvestmentSummary } from "../../api/contracts";
import { investmentYield } from "../../presentation/investmentLabels";
import { sumBRL } from "../../presentation/money";
import { SummaryStrip } from "../layout";
import { Money } from "../shared/Money";

/** What the synced investments add up to: the amount applied and the yield the institution lets us state. */
export function InvestmentsSummary({ investments, summary }: { investments: Investment[]; summary: InvestmentSummary | null }) {
  const balanceTotal = summary?.synced_value;
  const hasForeign = investments.some((investment) => investment.currency_code !== null && investment.currency_code !== "BRL");
  const yields = investments.map((investment) => investmentYield(investment)).filter((y) => y !== null);
  const yieldTotal = sumBRL(yields.map((y) => y.value));
  const missing = investments.length - yields.length;

  return (
    <SummaryStrip
      label="Aplicado nos sincronizados"
      value={balanceTotal === undefined ? "Indisponível" : <Money value={balanceTotal} tone="balance" size="hero" />}
      note={summary?.valuation ? `Valores em reais. Câmbio observado até ${new Date(summary.valuation.valued_at).toLocaleString("pt-BR")}.${hasForeign ? " Rendimento em outras moedas disponível nas posições." : ""}` : undefined}
      items={[
        { label: "Rendimento acumulado", value: hasForeign ? "Indisponível em reais" : <Money value={yieldTotal} tone="result" /> },
        ...(missing > 0 ? [{ label: "Sem dado de rendimento", value: String(missing) }] : []),
      ]}
    />
  );
}
