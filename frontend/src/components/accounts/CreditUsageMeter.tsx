import { useTranslation } from "../../i18n/context";
import { creditUsagePercent, creditUsageLevel, creditUsageShare } from "../../presentation/accountLabels";

/** Renders how much of a card's limit is committed: one thin fill bar, not the
 *  red/paid-vs-green/remaining pairing that fits a debt payoff (see
 *  payables.css) but not spending room. Color only escalates once usage
 *  actually needs attention — see creditUsageLevel. The caption reads
 *  "R$ 8.157,44 disponível · 32% usado" when the available amount is passed
 *  (already formatted), otherwise just the share. Returns null when the ratio
 *  couldn't be computed (no limit, no balance, or a bank account), so callers
 *  can drop it in unconditionally. */
export function CreditUsageMeter({ ratio, available }: { ratio: string | null; available?: string }) {
  const { t, language } = useTranslation();
  if (ratio === null) return null;
  const share = creditUsageShare(ratio);
  const level = creditUsageLevel(ratio);
  const used = t("creditUsed").replace("{percent}", creditUsagePercent(ratio, language));

  return (
    <div className="credit-usage-meter">
      <div className="credit-usage-meter-track" aria-hidden="true">
        <span className={`credit-usage-meter-fill is-${level}`} style={{ width: `${share}%` }} />
      </div>
      <span className="credit-usage-meter-caption">
        {available !== undefined ? t("creditAvailable").replace("{amount}", available).replace("{used}", used) : used}
      </span>
    </div>
  );
}
