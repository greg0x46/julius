import { useTranslation } from "../../i18n/context";
import { Skeleton } from "../ui";
import { Link } from "react-router-dom";

import type { Goal } from "../../api/contracts";
import { useGoals } from "../../hooks/useGoals";
import { formatDateOnly } from "../../presentation/dates";
import { compareDecimals } from "../../presentation/decimal";
import { goalPercent, sortGoals } from "../../presentation/goals";
import { problemMessage } from "../../presentation/reporting";
import { UnavailableState } from "../AsyncState";
import { Money } from "../shared/Money";
import { HomeTile } from "./HomeTile";

const route = "/metas";

/** Where the goal stands against its date, in a sentence; the server's projection, shown as it comes. */
function ProjectionLine({ goal }: { goal: Goal }) {
  const { t, language } = useTranslation();
  if (compareDecimals(goal.progress, "1") >= 0) return <>{t("goalReached")}</>;
  if (goal.projection === null || goal.target_date === null) return <>{t("noTargetDate")}</>;
  const date = formatDateOnly(goal.target_date, language);
  if (goal.projection.status === "behind") {
    return (
      <>
        {t("behindGoalPace").replace("{date}", date)}{" "}
        <Money currencyCode={goal.currency_code} value={goal.projection.required_monthly_contribution} tone="neutral" />{" "}
        {t("perMonth")}
      </>
    );
  }
  return <>{t("onGoalPace").replace("{date}", date)}</>;
}

/**
 * The first open goal (the order the Metas page uses) with its progress and
 * projection, as a link to that page. Progress and projection are the
 * server's, recomputed on read; goals are in BRL and never converted. With no
 * goals it is a quiet invitation to create one rather than an empty card.
 */
export function PrimaryGoalCard() {
  const { t } = useTranslation();
  const goals = useGoals();

  if (goals.isLoading) {
    return (
      <HomeTile label={t("goal")}>
        <div role="status" aria-label={t("loadingGoal")}>
          <Skeleton active title={false} paragraph={{ rows: 2 }} />
        </div>
      </HomeTile>
    );
  }
  if (goals.error && goals.goals.length === 0) {
    return (
      <HomeTile label={t("goal")}>
        <UnavailableState onRetry={() => goals.refetch()}>
          {problemMessage(goals.error) ?? t("goalsLoadError")}
        </UnavailableState>
      </HomeTile>
    );
  }
  if (goals.goals.length === 0) {
    return (
      <HomeTile label={t("goal")}>
        <span className="home-tile-line is-inline">
          <span className="home-tile-note">{t("noGoalsYet")}</span>
          <Link className="home-tile-action" to={route}>
            {t("createGoal")}
          </Link>
        </span>
      </HomeTile>
    );
  }

  const [goal, ...others] = sortGoals(goals.goals);
  const percent = goalPercent(goal.progress);

  return (
    <HomeTile label={`${t("goal")} · ${goal.name}`} to={route}>
      <span className="home-tile-line">
        <span className="home-tile-figure">{percent}%</span>
        <span
          className="home-meter"
          role="progressbar"
          aria-label={t("goalProgress", { name: goal.name })}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={Math.min(percent, 100)}
        >
          <span className="home-meter-fill" style={{ width: `${Math.min(percent, 100)}%` }} />
        </span>
        <span className="home-tile-note is-optional">
          <Money currencyCode={goal.currency_code} value={goal.current_value} tone="neutral" /> {t("of")}{" "}
          <Money currencyCode={goal.currency_code} value={goal.target_amount} tone="neutral" />
        </span>
        <span className="home-tile-note home-goal-projection">
          <ProjectionLine goal={goal} />
          {others.length > 0 && t(others.length === 1 ? "moreGoal" : "moreGoals").replace("{count}", String(others.length))}
        </span>
      </span>
    </HomeTile>
  );
}
