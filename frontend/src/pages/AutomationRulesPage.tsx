import { useTranslation } from "../i18n/context";
import { translate, type Language } from "../i18n/messages";
import { Alert, Button } from "../components/ui";
import { useRef, useState } from "react";

import type { AutomationAction, AutomationRule, RetroactiveApplyResult } from "../api/contracts";
import {
  AutomationEntry,
  AutomationEntryForm,
  AutomationEntrySubmitPayload,
} from "../components/automationRules/AutomationEntryForm";
import { AutomationEntryList } from "../components/automationRules/AutomationEntryList";
import { PageAction } from "../components/layout";
import { SettingsPageContainer as PageContainer } from "../components/SettingsPageContainer";
import { useFeedback } from "../components/shared/useFeedback";
import { useAutomationRules } from "../hooks/useAutomationRules";
import { useCategories } from "../hooks/useCategories";
import { useRecurringCommitments } from "../hooks/useRecurringCommitments";
import { errorMessage } from "../presentation/errors";

function retroactiveMessage(result: RetroactiveApplyResult, language: Language): string {
  const changes: string[] = [];
  if (result.ignored > 0) changes.push(translate(language, "retroIgnored", { count: result.ignored }));
  if (result.categorized > 0) changes.push(translate(language, "retroCategorized", { count: result.categorized }));
  return changes.length > 0
    ? translate(language, "retroFound", { matched: result.matched, changes: changes.join(", ") })
    : translate(language, "retroNone", { matched: result.matched });
}

export function AutomationRulesPage() {
  const { t, language } = useTranslation();
  const automationRules = useAutomationRules();
  const commitments = useRecurringCommitments();
  const categories = useCategories();
  const feedback = useFeedback();
  const [formOpen, setFormOpen] = useState(false);
  const [editingEntry, setEditingEntry] = useState<AutomationEntry | null>(null);
  const [togglingRuleId, setTogglingRuleId] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [lastResult, setLastResult] = useState<RetroactiveApplyResult | null>(null);
  // A commitment created before the rule write failed: a retry updates it instead of creating another.
  const createdCommitmentId = useRef<string | null>(null);

  const openCreate = () => {
    createdCommitmentId.current = null;
    setEditingEntry(null);
    setSaveError(null);
    setFormOpen(true);
  };

  const openEditRule = (rule: AutomationRule) => {
    createdCommitmentId.current = null;
    const target = rule.actions.find((action) => action.type === "reconcile");
    const linkedCommitment = target?.scenario_id
      ? commitments.commitments.find((commitment) => commitment.id === target.scenario_id) ?? null
      : null;
    setEditingEntry({ rule, linkedCommitment });
    setSaveError(null);
    setFormOpen(true);
  };

  const closeForm = () => setFormOpen(false);

  const submit = async (payload: AutomationEntrySubmitPayload) => {
    setSaveError(null);
    setSaving(true);
    try {
      const editingRuleId = editingEntry?.rule.id ?? null;

      const actions: AutomationAction[] = [];
      if (payload.actionTypes.includes("ignore")) {
        actions.push({ type: "ignore", scenario_id: null, category_id: null });
      }
      if (payload.actionTypes.includes("set_category") && payload.categoryId) {
        actions.push({ type: "set_category", scenario_id: null, category_id: payload.categoryId });
      }
      if (payload.actionTypes.includes("reconcile") && payload.reconcile) {
        const { reconcile } = payload;
        const existingId =
          reconcile.commitmentMode === "existing" && reconcile.existingCommitmentId
            ? reconcile.existingCommitmentId
            : createdCommitmentId.current;
        const savedCommitment = existingId
          ? await commitments.updateCommitment({ commitmentId: existingId, write: reconcile.commitment })
          : await commitments.createCommitment(reconcile.commitment);
        if (!existingId) createdCommitmentId.current = savedCommitment.id;
        actions.push({
          type: "reconcile",
          scenario_id: savedCommitment.id,
          category_id: null,
        });
      }

      const write = {
        name: payload.name,
        is_active: payload.isActive,
        logic_operator: payload.logicOperator,
        conditions: payload.conditions,
        actions,
        apply_retroactively: payload.applyRetroactively,
      };
      const result = editingRuleId
        ? await automationRules.updateRule({ ruleId: editingRuleId, write })
        : await automationRules.createRule(write);
      createdCommitmentId.current = null;
      setLastResult(result.retroactive_apply);
      setFormOpen(false);
      feedback.success(t("automationSaved"));
    } catch (error) {
      setSaveError(errorMessage(error, t("automationSaveError")));
    } finally {
      setSaving(false);
    }
  };

  const toggleRule = async (rule: AutomationRule, isActive: boolean) => {
    setActionError(null);
    setTogglingRuleId(rule.id);
    try {
      await automationRules.toggleRule({ ruleId: rule.id, isActive });
      feedback.success(isActive ? t("automationEnabled") : t("automationDisabled"));
    } catch (error) {
      const message = errorMessage(error, t("automationUpdateError"));
      setActionError(message);
      feedback.error(message);
    } finally {
      setTogglingRuleId(null);
    }
  };

  const removeRule = async (rule: AutomationRule) => {
    setActionError(null);
    setSaveError(null);
    try {
      await automationRules.deleteRule(rule.id);
      setFormOpen(false);
      feedback.success(t("automationDeleted"));
    } catch (error) {
      const message = errorMessage(error, t("automationDeleteError"));
      // Deleting from the edit drawer: the error must show there, not behind its mask.
      if (formOpen) {
        setSaveError(message);
      } else {
        setActionError(message);
        feedback.error(message);
      }
    }
  };

  const loadError = automationRules.error ?? commitments.error;

  return (
    <PageContainer
      title={t("automation")}
      subTitle={t("automationsSubtitle")}
      compactMobileHeader
      extra={[
        <PageAction key="new-entry" label={t("newAutomation")} shortLabel={t("newFeminine")} onClick={openCreate} />,
      ]}
    >
      {lastResult && (
        <Alert
          type="success"
          showIcon
          closable
          onClose={() => setLastResult(null)}
          message={retroactiveMessage(lastResult, language)}
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
      {loadError && (
        <Alert
          type="error"
          showIcon
          message={t("automationsLoadError")}
          description={
            <Button
              onClick={() => {
                automationRules.refetch();
                commitments.refetch();
              }}
            >
              {t("retry")}
            </Button>
          }
          style={{ marginBottom: 16 }}
        />
      )}
      {/* A failed load shows only the retry: an empty list beside it would say "nothing yet". */}
      {!loadError && (
        <AutomationEntryList
          rules={automationRules.rules}
          commitments={commitments.commitments}
          categories={categories.categories}
          isLoading={automationRules.isLoading || commitments.isLoading || categories.isLoading}
          togglingRuleId={togglingRuleId}
          onEditRule={openEditRule}
          onToggleRule={toggleRule}
          onDeleteRule={removeRule}
        />
      )}
      <AutomationEntryForm
        open={formOpen}
        entry={editingEntry}
        conditionOptions={automationRules.conditionOptions}
        conditionOptionsLoading={automationRules.areConditionOptionsLoading}
        categories={categories.categories}
        commitments={commitments.commitments}
        commitmentsLoading={commitments.isLoading}
        submitting={saving}
        submitError={saveError}
        onSubmit={submit}
        onCancel={closeForm}
        onDelete={removeRule}
      />
    </PageContainer>
  );
}
