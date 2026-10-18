import { DatePicker, Input, Select } from "antd";
import { useEffect, useState } from "react";

import { goalMetrics, type Goal, type GoalWrite } from "../../api/contracts";
import { goalMetricLabel } from "../../presentation/goals";
import { FormDrawer } from "../forms/FormDrawer";
import { FormField } from "../forms/FormField";
import { MoneyInput } from "../forms/MoneyInput";
import { fromMoneyInput, toMoneyInput } from "../investments/moneyDraft";
import { draftFromGoal, goalWriteFrom, type GoalDraft } from "./goalDraft";

const metricOptions = goalMetrics.map((metric) => ({ value: metric, label: goalMetricLabel[metric] }));

export function GoalForm({
  open,
  goal,
  submitting,
  submitError,
  onSubmit,
  onCancel,
}: {
  open: boolean;
  goal: Goal | null;
  submitting: boolean;
  submitError: string | null;
  onSubmit: (write: GoalWrite) => void;
  onCancel: () => void;
}) {
  const [draft, setDraft] = useState<GoalDraft>(() => draftFromGoal(goal));
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setDraft(draftFromGoal(goal));
      setError(null);
    }
  }, [open, goal]);

  const submit = () => {
    const result = goalWriteFrom(draft);
    if ("error" in result) {
      setError(result.error);
      return;
    }
    setError(null);
    onSubmit(result.write);
  };

  return (
    <FormDrawer
      title={goal ? "Editar meta" : "Nova meta"}
      open={open}
      onClose={onCancel}
      onSubmit={submit}
      submitting={submitting}
      error={error ?? submitError}
    >
      <FormField label="Nome da meta" htmlFor="goal-name">
        <Input
          id="goal-name"
          value={draft.name}
          onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))}
          placeholder="Ex.: Chegar a R$ 100 mil investidos"
          autoFocus
        />
      </FormField>
      <FormField label="O que acompanhar" htmlFor="goal-metric">
        <Select
          id="goal-metric"
          value={draft.metric}
          options={metricOptions}
          onChange={(metric) => setDraft((current) => ({ ...current, metric }))}
        />
      </FormField>
      <FormField label="Valor da meta" htmlFor="goal-target">
        <MoneyInput
          id="goal-target"
          min="0"
          value={toMoneyInput(draft.targetAmount)}
          onChange={(value) => setDraft((current) => ({ ...current, targetAmount: fromMoneyInput(value) }))}
        />
      </FormField>
      <FormField label="Data-alvo (opcional)" htmlFor="goal-date">
        <DatePicker
          id="goal-date"
          style={{ width: "100%" }}
          format="DD/MM/YYYY"
          value={draft.targetDate}
          onChange={(value) => setDraft((current) => ({ ...current, targetDate: value }))}
        />
      </FormField>
      <FormField label="Observações (opcional)" htmlFor="goal-notes">
        <Input.TextArea
          id="goal-notes"
          value={draft.notes}
          rows={4}
          onChange={(event) => setDraft((current) => ({ ...current, notes: event.target.value }))}
          placeholder="Para que serve esta meta?"
        />
      </FormField>
    </FormDrawer>
  );
}
