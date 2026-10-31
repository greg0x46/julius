import { longMonthName } from "../../i18n/dateFormats";
import { useTranslation } from "../../i18n/context";
import type { Language } from "../../i18n/messages";
import { isPositiveDecimal, sumDecimals } from "../../presentation/decimal";
import { Button, Checkbox, DatePicker, Divider, Input, InputNumber, Popconfirm, Segmented, Select, Switch } from "../ui";
import { calendar, type CalendarDate } from "../../presentation/calendar";
import { useEffect, useState } from "react";

import type {
  AutomationActionType,
  AutomationRule,
  AutomationRuleConditionOptions,
  Category,
  RecurringCommitment,
  RecurringCommitmentCadence,
  RecurringCommitmentKind,
  RecurringCommitmentWrite,
  RuleCondition,
  RuleLogicOperator,
} from "../../api/contracts";
import {
  recurringCommitmentCadenceName,
  recurringCommitmentKindName,
} from "../../presentation/recurringCommitmentLabels";
import {
  ruleConditionFieldName,
  ruleConditionOperatorName,
  ruleLogicOperatorName,
} from "../../presentation/ruleConditionLabels";
import { categoryKindName } from "../../presentation/categoryLabels";
import { AccountSelect } from "../forms/AccountSelect";
import { FormDrawer } from "../forms/FormDrawer";
import { FormField } from "../forms/FormField";
import { MoneyInput } from "../forms/MoneyInput";
import { ConditionFieldConfig, ConditionListEditor } from "../shared/ConditionListEditor";
import { deleteRuleDescription } from "./deleteRuleDescription";

const dateFormat = "YYYY-MM-DD";

export type AutomationEntry = {
  rule: AutomationRule;
  linkedCommitment: RecurringCommitment | null;
};

export type AutomationEntrySubmitPayload = {
  name: string;
  isActive: boolean;
  logicOperator: RuleLogicOperator;
  conditions: RuleCondition[];
  actionTypes: AutomationActionType[];
  applyRetroactively: boolean;
  categoryId: string | null;
  reconcile: {
    commitmentMode: "new" | "existing";
    existingCommitmentId: string | null;
    commitment: RecurringCommitmentWrite;
  } | null;
};

const monthOptions = (language: Language) =>
  Array.from({ length: 12 }, (_, index) => ({
    value: index + 1,
    label: longMonthName(language, index),
  }));

type RuleDraft = {
  name: string;
  isActive: boolean;
  logicOperator: RuleLogicOperator;
  conditions: RuleCondition[];
};

const blankIgnoreConditions: RuleCondition[] = [{ field: "description", operator: "contains", value: "" }];
// Tolerance fields by default, not "description" — avoids a stray blank
// text condition colliding (same "Valor" accessible name) with the "Dados
// da recorrência" section's own Valor field once that section renders.
const blankReconcileConditions: RuleCondition[] = [
  { field: "amount", operator: "within_percent", value: "0:10" },
  { field: "day_of_month", operator: "day_range", value: "1:10" },
];

// The API rejects a zero reference, which could never match an amount.
function hasZeroAmountReference(condition: RuleCondition): boolean {
  if (condition.operator !== "within_percent") return false;
  const reference = condition.value.split(":")[0]?.trim() ?? "";
  return /^[-+]?0*[.,]?0*$/.test(reference);
}

function blankConditionsFor(actionTypes: AutomationActionType[]): RuleCondition[] {
  return (actionTypes.includes("reconcile") ? blankReconcileConditions : blankIgnoreConditions).map(
    (condition) => ({ ...condition }),
  );
}

function ruleDraftFrom(rule: AutomationRule | null, actionTypes: AutomationActionType[]): RuleDraft {
  return rule
    ? {
        name: rule.name,
        isActive: rule.is_active,
        logicOperator: rule.logic_operator,
        conditions: rule.conditions.map((condition) => ({ ...condition })),
      }
    : { name: "", isActive: true, logicOperator: "or", conditions: blankConditionsFor(actionTypes) };
}

type CommitmentDraft = {
  name: string;
  kind: RecurringCommitmentKind;
  amount: string | null;
  categoryId: string | null;
  accountId: string;
  cadence: RecurringCommitmentCadence;
  dayOfMonth: number | null;
  monthOfYear: number | null;
  startDate: CalendarDate;
  endDate: CalendarDate | null;
  isActive: boolean;
};

// A function: a module-level `calendar()` would freeze the start date at page load.
const blankCommitmentDraft = (): CommitmentDraft => ({
  name: "",
  kind: "expense",
  amount: null,
  categoryId: null,
  accountId: "",
  cadence: "monthly",
  dayOfMonth: null,
  monthOfYear: null,
  startDate: calendar(),
  endDate: null,
  isActive: true,
});

function commitmentDraftFrom(commitment: RecurringCommitment | null): CommitmentDraft {
  if (!commitment) return blankCommitmentDraft();
  return {
    name: commitment.name,
    kind: commitment.kind,
    amount: commitment.amount,
    categoryId: commitment.category_id,
    accountId: commitment.account_id ?? "",
    cadence: commitment.cadence,
    dayOfMonth: commitment.day_of_month,
    monthOfYear: commitment.month_of_year,
    startDate: calendar(commitment.start_date),
    endDate: commitment.end_date ? calendar(commitment.end_date) : null,
    isActive: commitment.is_active,
  };
}

export function AutomationEntryForm({
  open,
  entry,
  conditionOptions,
  conditionOptionsLoading,
  categories,
  commitments,
  commitmentsLoading,
  submitting,
  submitError,
  onSubmit,
  onCancel,
  onDelete,
}: {
  open: boolean;
  entry: AutomationEntry | null;
  conditionOptions: AutomationRuleConditionOptions;
  conditionOptionsLoading: boolean;
  categories: Category[];
  commitments: RecurringCommitment[];
  commitmentsLoading: boolean;
  submitting: boolean;
  submitError: string | null;
  onSubmit: (payload: AutomationEntrySubmitPayload) => void;
  onCancel: () => void;
  /** Deleting lives in the edit sheet, not on the row: one tap target per row. */
  onDelete?: (rule: AutomationRule) => void;
}) {
  const { t, language } = useTranslation();
  const actionOptions: { value: AutomationActionType; label: string }[] = [
    { value: "ignore", label: t("ignoreTransaction") },
    { value: "reconcile", label: t("reconcileRecurrenceAction") },
    { value: "set_category", label: t("actionSetCategory") },
  ];
  const ruleLogicOptions = (["and", "or"] as RuleLogicOperator[]).map((value) => ({
    value,
    label: ruleLogicOperatorName(value, language),
  }));
  const kindOptions = (["income", "expense"] as RecurringCommitmentKind[]).map((value) => ({
    value,
    label: recurringCommitmentKindName(value, language),
  }));
  const cadenceOptions = (["monthly", "annual"] as RecurringCommitmentCadence[]).map((value) => ({
    value,
    label: recurringCommitmentCadenceName(value, language),
  }));
  const commitmentModeOptions: { value: "new" | "existing"; label: string }[] = [
    { value: "new", label: t("newRecurrence") },
    { value: "existing", label: t("existingRecurrence") },
  ];
  const isEditing = entry !== null;
  const initialActionTypes: AutomationActionType[] = entry
    ? entry.rule.actions.map((action) => action.type)
    : ["ignore"];
  const initialCategoryId =
    entry?.rule.actions.find((action) => action.type === "set_category")?.category_id ?? null;

  const [actionTypes, setActionTypes] = useState<AutomationActionType[]>(initialActionTypes);
  const [categoryId, setCategoryId] = useState<string | null>(initialCategoryId);
  const [categoryEdited, setCategoryEdited] = useState(false);
  const [commitmentMode, setCommitmentMode] = useState<"new" | "existing">(
    entry?.linkedCommitment ? "existing" : "new",
  );
  const [selectedCommitmentId, setSelectedCommitmentId] = useState<string | null>(
    entry?.linkedCommitment?.id ?? null,
  );
  const [ruleDraft, setRuleDraft] = useState<RuleDraft>(() => ruleDraftFrom(entry?.rule ?? null, initialActionTypes));
  const [commitmentDraft, setCommitmentDraft] = useState<CommitmentDraft>(() =>
    commitmentDraftFrom(entry?.linkedCommitment ?? null),
  );
  const [applyRetroactively, setApplyRetroactively] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setActionTypes(initialActionTypes);
      setCategoryId(initialCategoryId);
      setCategoryEdited(false);
      setCommitmentMode(entry?.linkedCommitment ? "existing" : "new");
      setSelectedCommitmentId(entry?.linkedCommitment?.id ?? null);
      setRuleDraft(ruleDraftFrom(entry?.rule ?? null, initialActionTypes));
      setCommitmentDraft(commitmentDraftFrom(entry?.linkedCommitment ?? null));
      setApplyRetroactively(false);
      setError(null);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, entry]);

  useEffect(() => {
    if (!isEditing && actionTypes.includes("reconcile") && !categoryEdited) {
      setCategoryId(commitmentDraft.categoryId);
    }
  }, [isEditing, actionTypes, categoryEdited, commitmentDraft.categoryId]);

  const selectExistingCommitment = (commitmentId: string | null) => {
    const selected = commitmentId ? commitments.find((item) => item.id === commitmentId) ?? null : null;
    setSelectedCommitmentId(selected?.id ?? null);
    setCommitmentDraft(commitmentDraftFrom(selected));
  };

  const setCommitmentModeAndReset = (mode: "new" | "existing") => {
    setCommitmentMode(mode);
    if (mode === "new") {
      setSelectedCommitmentId(null);
      setCommitmentDraft(blankCommitmentDraft());
    }
  };

  const setActionTypesAndReset = (nextActionTypes: AutomationActionType[]) => {
    setActionTypes(nextActionTypes);
    if (!isEditing) {
      setRuleDraft((current) => ({ ...current, conditions: blankConditionsFor(nextActionTypes) }));
    }
    if (!nextActionTypes.includes("reconcile")) {
      setCommitmentMode("new");
      setSelectedCommitmentId(null);
      setCommitmentDraft(blankCommitmentDraft());
    }
    if (!nextActionTypes.includes("set_category") && !nextActionTypes.includes("reconcile")) {
      setCategoryEdited(false);
      setCategoryId(null);
    }
  };

  const baseFieldConfigs: ConditionFieldConfig[] = [
    {
      field: "description",
      label: ruleConditionFieldName("description", language),
      operators: [
        { value: "contains", label: ruleConditionOperatorName("contains", language) },
        { value: "equals", label: ruleConditionOperatorName("equals", language) },
      ],
    },
    {
      field: "card",
      label: ruleConditionFieldName("card", language),
      operators: [
        { value: "contains", label: ruleConditionOperatorName("contains", language) },
        { value: "equals", label: ruleConditionOperatorName("equals", language) },
      ],
      valueOptions: () => conditionOptions.cards.map((value) => ({ value, label: value })),
      valueOptionsLoading: conditionOptionsLoading,
      valuePlaceholder: () => t("selectCard"),
      valueNotFoundContent: () => (conditionOptionsLoading ? t("loading") : t("noCardFound")),
    },
    {
      field: "account",
      label: ruleConditionFieldName("account", language),
      operators: [
        { value: "contains", label: ruleConditionOperatorName("contains", language) },
        { value: "equals", label: ruleConditionOperatorName("equals", language) },
      ],
      valueOptions: () => conditionOptions.accounts.map((value) => ({ value, label: value })),
      valueOptionsLoading: conditionOptionsLoading,
      valuePlaceholder: () => t("selectAnAccount"),
      valueNotFoundContent: () => (conditionOptionsLoading ? t("loading") : t("noAccountFound")),
    },
  ];

  const reconcileFieldConfigs: ConditionFieldConfig[] = [
    {
      field: "amount",
      label: ruleConditionFieldName("amount", language),
      operators: [{ value: "within_percent", label: ruleConditionOperatorName("within_percent", language) }],
      pairOperator: "within_percent",
      pairFirstLabel: t("expectedAmount"),
      pairSecondLabel: t("tolerance"),
      pairSecondSuffix: "%",
      pairMin: 0,
    },
    {
      field: "day_of_month",
      label: ruleConditionFieldName("day_of_month", language),
      operators: [{ value: "day_range", label: ruleConditionOperatorName("day_range", language) }],
      pairOperator: "day_range",
      pairFirstLabel: t("fromDay"),
      pairSecondLabel: t("untilDay"),
      pairMin: 1,
      pairMax: 31,
    },
  ];

  const conditionFieldConfigs: ConditionFieldConfig[] = [...reconcileFieldConfigs, ...baseFieldConfigs];

  // Two lists, deliberately. A rule's set_category action must be able to
  // assign a transfer category — that is how a transfer between the user's
  // own accounts gets kept out of income/expense totals without abusing
  // "ignore". A recurring commitment, on the other hand, projects an expense
  // or an income, so a transfer category there would never show up in the
  // flow at all.
  //
  // The kind prefix matches the label the transaction drawer already uses, so
  // a transfer category is recognizable as one in the list.
  const ruleCategoryOptions = categories
    .filter((category) => category.is_active)
    .map((category) => ({
      value: category.id,
      label: `${categoryKindName(category.kind, language)}: ${category.name}`,
    }));

  const commitmentCategoryOptions = categories
    .filter((category) => category.is_active && category.kind !== "transfer")
    .map((category) => ({ value: category.id, label: category.name }));

  const submit = () => {
    if (ruleDraft.name.trim() === "") {
      setError(t("ruleNameRequired"));
      return;
    }
    if (ruleDraft.conditions.length === 0) {
      setError(t("conditionRequired"));
      return;
    }
    if (ruleDraft.conditions.some((condition) => condition.value.trim() === "")) {
      setError(t("conditionValueRequired"));
      return;
    }
    if (ruleDraft.conditions.some(hasZeroAmountReference)) {
      setError(t("expectedAmountPositive"));
      return;
    }
    if (actionTypes.length === 0) {
      setError(t("actionRequired"));
      return;
    }
    if (actionTypes.includes("set_category") && !actionTypes.includes("reconcile") && categoryId === null) {
      setError(t("categoryToApplyRequired"));
      return;
    }

    let reconcile: AutomationEntrySubmitPayload["reconcile"] = null;
    if (actionTypes.includes("reconcile")) {
      if (commitmentDraft.name.trim() === "") {
        setError(t("recurrenceNameRequired"));
        return;
      }
      if (commitmentDraft.amount === null || !isPositiveDecimal(commitmentDraft.amount)) {
        setError(t("positiveAmountRequired"));
        return;
      }
      if (commitmentDraft.categoryId === null) {
        setError(t("categoryRequired"));
        return;
      }
      if (commitmentDraft.dayOfMonth === null) {
        setError(t("dayOfMonthRequired"));
        return;
      }
      if (commitmentDraft.cadence === "annual" && commitmentDraft.monthOfYear === null) {
        setError(t("monthOfYearRequired"));
        return;
      }
      if (commitmentDraft.endDate && commitmentDraft.endDate.isBefore(commitmentDraft.startDate, "day")) {
        setError(t("endBeforeStart"));
        return;
      }
      reconcile = {
        commitmentMode,
        existingCommitmentId: commitmentMode === "existing" ? selectedCommitmentId : null,
        commitment: {
          name: commitmentDraft.name.trim(),
          kind: commitmentDraft.kind,
          amount: sumDecimals([commitmentDraft.amount]),
          category_id: commitmentDraft.categoryId,
          account_id: commitmentDraft.accountId.trim() === "" ? null : commitmentDraft.accountId.trim(),
          cadence: commitmentDraft.cadence,
          day_of_month: commitmentDraft.dayOfMonth,
          month_of_year: commitmentDraft.cadence === "annual" ? commitmentDraft.monthOfYear : null,
          start_date: commitmentDraft.startDate.format(dateFormat),
          end_date: commitmentDraft.endDate ? commitmentDraft.endDate.format(dateFormat) : null,
          is_active: commitmentDraft.isActive,
        },
      };
    }

    setError(null);
    onSubmit({
      name: ruleDraft.name.trim(),
      isActive: ruleDraft.isActive,
      logicOperator: ruleDraft.logicOperator,
      conditions: ruleDraft.conditions.map((condition) => ({ ...condition, value: condition.value.trim() })),
      actionTypes: actionTypes.includes("reconcile")
        ? [...actionTypes.filter((type) => type !== "set_category"), ...(categoryId ? ["set_category" as const] : [])]
        : actionTypes,
      applyRetroactively,
      categoryId: actionTypes.includes("reconcile") || actionTypes.includes("set_category") ? categoryId : null,
      reconcile,
    });
  };

  return (
    <FormDrawer
      title={isEditing ? t("editAutomation") : t("newAutomation")}
      open={open}
      onClose={onCancel}
      onSubmit={submit}
      submitting={submitting}
      error={error ?? submitError}
    >
      <FormField label={t("automationName")} htmlFor="automation-rule-name">
        <Input
          id="automation-rule-name"
          value={ruleDraft.name}
          onChange={(event) => setRuleDraft((current) => ({ ...current, name: event.target.value }))}
          placeholder={t("automationNameExample")}
        />
      </FormField>

      <FormField label={t("activeFeminineHeader")} htmlFor="automation-rule-active">
        <Switch
          id="automation-rule-active"
          aria-label={t("automationActive")}
          checked={ruleDraft.isActive}
          onChange={(checked) => setRuleDraft((current) => ({ ...current, isActive: checked }))}
          style={{ width: "fit-content" }}
        />
      </FormField>

      <FormField label={t("conditionsHeader")} labelId="automation-conditions-label">
        <ConditionListEditor
          conditions={ruleDraft.conditions}
          fieldConfigs={conditionFieldConfigs}
          onChange={(conditions) => setRuleDraft((current) => ({ ...current, conditions }))}
        />
      </FormField>

      {/* With one condition "E" and "OU" mean the same thing, so the choice
          only appears once there is something to combine. The stored
          operator is untouched either way. */}
      {ruleDraft.conditions.length >= 2 && (
        <FormField label={t("combineConditionsWith")} htmlFor="automation-rule-logic">
          <Select
            id="automation-rule-logic"
            value={ruleDraft.logicOperator}
            options={ruleLogicOptions}
            onChange={(value: RuleLogicOperator) =>
              setRuleDraft((current) => ({ ...current, logicOperator: value }))
            }
          />
        </FormField>
      )}

      <FormField label={t("actions")} labelId="automation-actions-label">
        <Checkbox.Group
          aria-labelledby="automation-actions-label"
          value={actionTypes}
          options={actionOptions.filter((option) => !actionTypes.includes("reconcile") || option.value !== "set_category")}
          disabled={isEditing}
          onChange={(value) => setActionTypesAndReset(value as AutomationActionType[])}
        />
      </FormField>

      {(actionTypes.includes("set_category") || actionTypes.includes("reconcile")) && (
        <FormField
          label={actionTypes.includes("reconcile") ? t("categoryToApplyOptional") : t("categoryToApply")}
          htmlFor="automation-entry-category"
          hint={actionTypes.includes("reconcile") ? t("categoryToApplyHint") : undefined}
        >
          <Select
            id="automation-entry-category"
            allowClear={actionTypes.includes("reconcile")}
            aria-describedby={actionTypes.includes("reconcile") ? "automation-entry-category-hint" : undefined}
            value={categoryId ?? undefined}
            options={ruleCategoryOptions}
            showSearch
            optionFilterProp="label"
            placeholder={actionTypes.includes("reconcile") ? t("noCategoryToApply") : t("selectACategory")}
            onChange={(value: string | undefined) => {
              setCategoryEdited(true);
              setCategoryId(value ?? null);
            }}
          />
        </FormField>
      )}

      {(actionTypes.includes("ignore") || (actionTypes.includes("reconcile") ? categoryId !== null : actionTypes.includes("set_category"))) && (
        <Checkbox checked={applyRetroactively} onChange={(event) => setApplyRetroactively(event.target.checked)}>
          {t("applyNowToExisting")}
        </Checkbox>
      )}

      {actionTypes.includes("reconcile") && (
        <>
          {!isEditing && (
            <FormField label={t("recurrence")} labelId="recurring-commitment-mode-label">
              <Segmented
                id="recurring-commitment-mode"
                aria-labelledby="recurring-commitment-mode-label"
                value={commitmentMode}
                options={commitmentModeOptions}
                onChange={(value) => setCommitmentModeAndReset(value as "new" | "existing")}
              />
            </FormField>
          )}

          {commitmentMode === "existing" && (
            <FormField label={t("selectARecurrence")} htmlFor="recurring-commitment-existing">
              <Select
                id="recurring-commitment-existing"
                showSearch
                optionFilterProp="label"
                loading={commitmentsLoading}
                placeholder={t("selectAnExistingRecurrence")}
                value={selectedCommitmentId ?? undefined}
                options={commitments.map((commitment) => ({ value: commitment.id, label: commitment.name }))}
                onChange={(value: string) => selectExistingCommitment(value)}
              />
            </FormField>
          )}

          <Divider orientation="left" plain style={{ margin: 0 }}>
            {t("recurrenceData")}
          </Divider>

          <FormField label={t("name")} htmlFor="recurring-commitment-name">
            <Input
              id="recurring-commitment-name"
              value={commitmentDraft.name}
              onChange={(event) => setCommitmentDraft((current) => ({ ...current, name: event.target.value }))}
              placeholder={t("recurrenceNameExample")}
            />
          </FormField>

          <FormField label={t("type")} htmlFor="recurring-commitment-kind">
            <Select
              id="recurring-commitment-kind"
              value={commitmentDraft.kind}
              options={kindOptions}
              onChange={(value: RecurringCommitmentKind) =>
                setCommitmentDraft((current) => ({ ...current, kind: value }))
              }
            />
          </FormField>

          <FormField label={t("amount")} htmlFor="recurring-commitment-amount">
            <MoneyInput
              id="recurring-commitment-amount"
              min="0"
              value={commitmentDraft.amount}
              onChange={(value) => setCommitmentDraft((current) => ({ ...current, amount: value }))}
            />
          </FormField>

          <FormField label={t("category")} htmlFor="recurring-commitment-category">
            <Select
              id="recurring-commitment-category"
              value={commitmentDraft.categoryId ?? undefined}
              options={commitmentCategoryOptions}
              showSearch
              optionFilterProp="label"
              placeholder={t("selectACategory")}
              onChange={(value: string) => setCommitmentDraft((current) => ({ ...current, categoryId: value }))}
            />
          </FormField>

          <FormField label={t("accountOptional")} htmlFor="recurring-commitment-account">
            <AccountSelect
              id="recurring-commitment-account"
              optional
              value={commitmentDraft.accountId}
              onChange={(accountId) => setCommitmentDraft((current) => ({ ...current, accountId: accountId ?? "" }))}
            />
          </FormField>

          <FormField label={t("cadence")} htmlFor="recurring-commitment-cadence">
            <Select
              id="recurring-commitment-cadence"
              value={commitmentDraft.cadence}
              options={cadenceOptions}
              onChange={(value: RecurringCommitmentCadence) =>
                setCommitmentDraft((current) => ({ ...current, cadence: value }))
              }
            />
          </FormField>

          <FormField
            label={t("dayOfMonth")}
            htmlFor="recurring-commitment-day"
            hint={t("dayOfMonthHint")}
          >
            <InputNumber
              id="recurring-commitment-day"
              style={{ width: "100%" }}
              min={1}
              max={31}
              precision={0}
              inputMode="numeric"
              value={commitmentDraft.dayOfMonth}
              onChange={(value) => setCommitmentDraft((current) => ({ ...current, dayOfMonth: value }))}
            />
          </FormField>

          {commitmentDraft.cadence === "annual" && (
            <FormField label={t("monthOfYear")} htmlFor="recurring-commitment-month">
              <Select
                id="recurring-commitment-month"
                value={commitmentDraft.monthOfYear ?? undefined}
                options={monthOptions(language)}
                placeholder={t("selectAMonth")}
                onChange={(value: number) => setCommitmentDraft((current) => ({ ...current, monthOfYear: value }))}
              />
            </FormField>
          )}

          <FormField label={t("startDateLabel")} htmlFor="recurring-commitment-start-date">
            <DatePicker
              id="recurring-commitment-start-date"
              style={{ width: "100%" }}
              value={commitmentDraft.startDate}
              allowClear={false}
              onChange={(value) => value && setCommitmentDraft((current) => ({ ...current, startDate: value }))}
            />
          </FormField>

          <FormField label={t("endDateOptional")} htmlFor="recurring-commitment-end-date">
            <DatePicker
              id="recurring-commitment-end-date"
              style={{ width: "100%" }}
              value={commitmentDraft.endDate}
              onChange={(value) => setCommitmentDraft((current) => ({ ...current, endDate: value }))}
            />
          </FormField>

          <FormField label={t("recurrenceActive")} htmlFor="recurring-commitment-active">
            <Switch
              id="recurring-commitment-active"
              aria-label={t("recurrenceActive")}
              checked={commitmentDraft.isActive}
              onChange={(checked) => setCommitmentDraft((current) => ({ ...current, isActive: checked }))}
              style={{ width: "fit-content" }}
            />
          </FormField>
        </>
      )}

      {entry && onDelete && (
        <div className="automation-form-delete">
          <Popconfirm
            title={t("deleteAutomation")}
            description={deleteRuleDescription(entry.rule, language)}
            onConfirm={() => onDelete(entry.rule)}
            okText={t("delete")}
            cancelText={t("cancel")}
          >
            <Button danger>{t("deleteAutomation")}</Button>
          </Popconfirm>
        </div>
      )}
    </FormDrawer>
  );
}
