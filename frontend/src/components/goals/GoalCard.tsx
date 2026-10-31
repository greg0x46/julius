import { useTranslation } from "../../i18n/context";
import { useId } from "react";

import type { Goal } from "../../api/contracts";
import { formatDateOnly } from "../../presentation/dates";
import { compareDecimals } from "../../presentation/decimal";
import { goalMonthsLabel, goalPercent } from "../../presentation/goals";
import { GoalIcon, TargetDateIcon } from "./GoalIcons";
import { GoalProgressRing, type GoalProgressTone } from "./GoalProgressRing";
import { GoalProjection } from "./GoalProjection";
import { Money } from "../shared/Money";
import { RecordMenu } from "../shared/RecordMenu";
import { StatusTag } from "../shared/StatusTag";
import { useConfirm } from "../shared/useConfirm";

/**
 * One goal as a card: where it stands against its target. Editar and
 * Excluir live in one "···" menu on the title line. A behind/reached goal
 * gets a quiet colour cue (icon, ring, top edge, status tag) — on track
 * carries no top edge or tag, per the app's "no tag for the common case" rule.
 */
export function GoalCard({
  goal,
  busy,
  onEdit,
  onRemove,
}: {
  goal: Goal;
  busy: boolean;
  onEdit: (goal: Goal) => void;
  onRemove: (goal: Goal) => void;
}) {
  const { t, language } = useTranslation();
  const confirm = useConfirm();
  const headingId = useId();
  const percent = goalPercent(goal.progress);
  const reached = compareDecimals(goal.progress, "1") >= 0;
  const behind = !reached && goal.projection?.status === "behind";
  const tone: GoalProgressTone = reached ? "success" : behind ? "warning" : "accent";

  return (
    <section
      className={["section", "goal-card", `goal-card-${reached ? "reached" : behind ? "behind" : "ontrack"}`].join(" ")}
      aria-labelledby={headingId}
    >
      <div className="section-body">
        <header className="goal-card-header">
          <span className="goal-card-icon" aria-hidden="true">
            <GoalIcon reached={reached} />
          </span>
          <div className="goal-card-heading">
            <h2 id={headingId}>{goal.name}</h2>
            <p className="goal-metric">{t("investedValue")}</p>
          </div>
          <RecordMenu
            label={t("actionsFor", { name: goal.name })}
            items={[
              { key: "edit", label: t("editGoal"), onClick: () => onEdit(goal) },
              {
                key: "delete",
                label: t("deleteGoal"),
                danger: true,
                disabled: busy,
                onClick: () =>
                  confirm({
                    title: t("deleteGoal"),
                    description: t("deleteGoalHint"),
                    onConfirm: () => onRemove(goal),
                  }),
              },
            ]}
          />
        </header>
        <div className="goal-summary">
          <GoalProgressRing percent={percent} tone={tone} label={t("goalProgress", { name: goal.name })} />
          <div className="goal-figures-col">
            <div className="goal-figures">
              <Money currencyCode={goal.currency_code} value={goal.current_value} tone="balance" size="hero" />
              <span className="goal-target">
                {t("of")} <Money currencyCode={goal.currency_code} value={goal.target_amount} tone="neutral" />
              </span>
            </div>
            {reached ? (
              <StatusTag tone="success">{t("goalReached")}</StatusTag>
            ) : behind ? (
              <StatusTag tone="warning">{t("behindPace")}</StatusTag>
            ) : null}
          </div>
        </div>
        <p className="goal-date">
          <TargetDateIcon />
          <span>
            {goal.target_date === null
              ? t("noTargetDate")
              : `${formatDateOnly(goal.target_date, language)}${goal.projection !== null ? t("monthRemaining").replace("{months}", goalMonthsLabel(goal.projection.months_remaining, language)) : ""}`}
          </span>
        </p>
        <GoalProjection goal={goal} />
        {goal.notes !== null && <p className="goal-note">{goal.notes}</p>}
      </div>
    </section>
  );
}
