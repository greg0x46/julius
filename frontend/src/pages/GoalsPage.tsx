import { useTranslation } from "../i18n/context";
import { Alert, Button, Skeleton } from "../components/ui";
import { useState } from "react";

import type { Goal, GoalWrite } from "../api/contracts";
import { GoalCard } from "../components/goals/GoalCard";
import { GoalForm } from "../components/goals/GoalForm";
import { EmptyState, Page, PageAction } from "../components/layout";
import { useFeedback } from "../components/shared/useFeedback";
import { useGoals } from "../hooks/useGoals";
import { errorMessage } from "../presentation/errors";
import { sortGoals } from "../presentation/goals";

export function GoalsPage() {
  const { t } = useTranslation();
  const goals = useGoals();
  const feedback = useFeedback();
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<Goal | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const openForm = (goal: Goal | null) => {
    setSaveError(null);
    setEditing(goal);
    setFormOpen(true);
  };

  const submit = async (write: GoalWrite) => {
    setSaveError(null);
    try {
      await (editing ? goals.updateGoal({ goalId: editing.id, write }) : goals.createGoal(write));
      setFormOpen(false);
      feedback.success(t("saved"));
    } catch (error) {
      setSaveError(errorMessage(error, t("goalSaveError")));
    }
  };

  const remove = async (goal: Goal) => {
    setActionError(null);
    try {
      await goals.deleteGoal(goal.id);
      feedback.success(t("deleted"));
    } catch (error) {
      const message = errorMessage(error, t("goalDeleteError"));
      setActionError(message);
      feedback.error(message);
    }
  };

  const nothingYet = goals.goals.length === 0;
  // A failed load with nothing to show is not "nothing yet": it only says it failed.
  const loadFailed = goals.error !== null && nothingYet;

  return (
    <Page
      title={t("goals")}
      description={t("goalsDescription")}
      width="narrow"
      compactMobileHeader
      actions={<PageAction label={t("newGoal")} shortLabel={t("newFeminine")} onClick={() => openForm(null)} />}
    >
      {goals.error && (
        <Alert
          type="error"
          showIcon
          message={t("goalsLoadErrorShort")}
          action={<Button onClick={() => goals.refetch()}>{t("retry")}</Button>}
          style={{ marginBottom: 16 }}
        />
      )}
      {actionError && (
        <Alert
          type="error"
          showIcon
          closable
          onClose={() => setActionError(null)}
          message={actionError}
          style={{ marginBottom: 16 }}
        />
      )}

      {goals.isLoading ? (
        <div className="section" role="status" aria-label={t("loadingGoals")}>
          <div className="section-body">
            <Skeleton active paragraph={{ rows: 3 }} />
          </div>
        </div>
      ) : loadFailed ? null : nothingYet ? (
        <div className="section">
          <EmptyState
            title={t("noGoals")}
            hint={t("noGoalsHint")}
          />
        </div>
      ) : (
        <div className="goals-list">
          {sortGoals(goals.goals).map((goal) => (
            <GoalCard
              key={goal.id}
              goal={goal}
              busy={goals.isDeleting}
              onEdit={openForm}
              onRemove={(target) => void remove(target)}
            />
          ))}
          <button type="button" className="goal-add" onClick={() => openForm(null)}>
            <span className="goal-add-plus" aria-hidden="true">
              +
            </span>
            {t("addGoal")}
          </button>
        </div>
      )}

      <GoalForm
        open={formOpen}
        goal={editing}
        submitting={goals.isSaving}
        submitError={saveError}
        onSubmit={(write) => void submit(write)}
        onCancel={() => setFormOpen(false)}
      />
    </Page>
  );
}
