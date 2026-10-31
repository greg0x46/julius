import { translate, type Language } from "../../i18n/messages";
import { calendar, type CalendarDate } from "../../presentation/calendar";

import type { Goal, GoalMetric, GoalWrite } from "../../api/contracts";
import { isPositiveDecimal } from "../../presentation/decimal";

export type GoalDraft = {
  name: string;
  metric: GoalMetric;
  targetAmount: string | null;
  targetDate: CalendarDate | null;
  notes: string;
};

export function draftFromGoal(goal: Goal | null): GoalDraft {
  if (!goal) return { name: "", metric: "invested_value", targetAmount: null, targetDate: null, notes: "" };
  return {
    name: goal.name,
    metric: goal.metric,
    targetAmount: goal.target_amount,
    targetDate: goal.target_date ? calendar(goal.target_date) : null,
    notes: goal.notes ?? "",
  };
}

/** The write the API accepts, or the message to show when the draft cannot be saved. */
export function goalWriteFrom(draft: GoalDraft, language: Language = "pt-BR"): { write: GoalWrite } | { error: string } {
  if (draft.name.trim() === "") return { error: translate(language, "goalNameRequired") };
  if (draft.targetAmount === null || !isPositiveDecimal(draft.targetAmount)) {
    return { error: translate(language, "goalAmountPositive") };
  }
  return {
    write: {
      name: draft.name.trim(),
      metric: draft.metric,
      target_amount: draft.targetAmount,
      target_date: draft.targetDate?.format("YYYY-MM-DD") ?? null,
      notes: draft.notes.trim() === "" ? null : draft.notes.trim(),
    },
  };
}
