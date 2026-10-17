import { ReportValuation } from "../shared/ReportValuation";
import { valueInReferenceCurrency } from "../../presentation/valuation";
import type { ReportValuation as Valuation } from "../../api/contracts/valuation";
import type { Investment } from "../../api/contracts";
import { investmentYield } from "../../presentation/investmentLabels";
import { sumBRL } from "../../presentation/money";
import { SummaryStrip } from "../layout";
import { Money } from "../shared/Money";

/** What the synced investments add up to: the amount applied and the yield the institution lets us state. */
export function InvestmentsSummary({
  investments,
  valuation,
}: {
  investments: Investment[];
  valuation?: Valuation;
}) {
  const currencyCode = valuation?.currency_code ?? "BRL";
  const eligible = investments.filter(
    (investment) => !investment.currency_code || investment.currency_code === "BRL",
  );
  const convert = (value: string) => valueInReferenceCurrency(value, valuation?.fx);
  const balanceTotal = sumBRL(eligible.map((investment) => investment.balance ?? "0"));
  const yields = eligible.map((investment) => investmentYield(investment)).filter((y) => y !== null);
  const yieldTotal = sumBRL(yields.map((y) => y.value));
  const missing = eligible.length - yields.length;

  return (
    <ReportValuation valuation={valuation ?? {}}>
      <SummaryStrip
        label="Aplicado nos sincronizados"
        value={<Money report currencyCode={currencyCode} value={convert(balanceTotal)} tone="balance" size="hero" />}
        items={[
          {
            label: "Rendimento acumulado",
            value: <Money report currencyCode={currencyCode} value={convert(yieldTotal)} tone="result" />,
          },
          ...(missing > 0 ? [{ label: "Sem dado de rendimento", value: String(missing) }] : []),
        ]}
      />
    </ReportValuation>
  );
}
