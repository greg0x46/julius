import { useTranslation } from "../../i18n/context";
import { InfoCircleOutlined } from "../ui/icons";

import type { Goal } from "../../api/contracts";
import { formatDateOnly } from "../../presentation/dates";
import { formatRatioPercent } from "../../presentation/investmentPeriod";
import { InvestmentHint } from "../investments/InvestmentHint";
import { Money } from "../shared/Money";

/** Where the goal lands on its target date and what it takes to get there. */
export function GoalProjection({ goal }: { goal: Goal }) {
  const { t, language } = useTranslation();
  const projection = goal.projection;
  if (projection === null || goal.target_date === null) return null;
  const { monthly_return: monthlyReturn } = projection;

  const explanation =
    monthlyReturn === null
      ? t("goalHistoryMissing")
      : t("goalReturnExplanation").replace("{date}", formatDateOnly(projection.return_from ?? goal.target_date, language)).replace("{partial}", projection.return_partial ? t("goalPartialPositions") : "");

  return (
    <div className="goal-projection" aria-label={t("goalProjectionLabel", { name: goal.name })}>
      <div className="goal-projection-highlight">
        <span>{t("requiredMonthlyContribution")}</span>
        <strong>
          {projection.status === "behind" ? (
            <Money currencyCode={goal.currency_code} value={projection.required_monthly_contribution} tone="neutral" />
          ) : projection.status === "reached" ? (
            t("goalAlreadyReached")
          ) : (
            t("returnEnough")
          )}
        </strong>
      </div>
      <dl className="goal-projection-list">
        <div>
          <dt>
            {t("averageReturn")}
            <InvestmentHint label={t("aboutGoalProjection", { name: goal.name, explanation })} content={explanation}>
              <InfoCircleOutlined className="goal-projection-info" />
            </InvestmentHint>
          </dt>
          <dd>
            {monthlyReturn === null ? (
              t("insufficientHistory")
            ) : (
              `${formatRatioPercent(monthlyReturn, { signed: true, language })} ${t("perMonthSuffix")}`
            )}
          </dd>
        </div>
        <div>
          <dt>{t("projectedWithoutContributions")}</dt>
          <dd>
            <Money currencyCode={goal.currency_code} value={projection.projected_value} tone="neutral" /> (
            {formatRatioPercent(projection.projected_progress, { language })} {t("ofGoal")})
          </dd>
        </div>
      </dl>
    </div>
  );
}
