import { formatRatioPercent, ratioDirection } from "../../presentation/investmentPeriod";
import { Money } from "../shared/Money";

/**
 * Rendimento in R$ with its rentabilidade beside it. The amount goes through
 * the app's money rule for a result (explicit +/−, never colour alone); the %
 * is the backend's ratio and takes the tone of the percentage itself. Either
 * part may be left out (null).
 */
export function InvestmentYieldValue({ gain, rate }: { gain: string | null; rate: string | null }) {
  const direction = rate === null ? "zero" : ratioDirection(rate);
  const color = direction === "positive" ? "positive" : direction === "negative" ? "negative" : "neutral";
  return (
    <span className="investment-yield">
      {gain !== null && <Money value={gain} tone="result" />}
      {rate !== null && (
        <span className={`investment-yield-percent money-${color}`}>{formatRatioPercent(rate, { signed: true })}</span>
      )}
    </span>
  );
}
