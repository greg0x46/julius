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
  recurringCommitmentCadenceLabel,
  recurringCommitmentKindLabel,
} from "../../presentation/recurringCommitmentLabels";
import {
  ruleConditionFieldLabel,
  ruleConditionOperatorLabel,
  ruleLogicOperatorLabel,
} from "../../presentation/ruleConditionLabels";
import { categoryKindLabel } from "../../presentation/categoryLabels";
import { AccountSelect } from "../forms/AccountSelect";
import { FormDrawer } from "../forms/FormDrawer";
import { FormField } from "../forms/FormField";
import { MoneyInput } from "../forms/MoneyInput";
import { ConditionFieldConfig, ConditionListEditor } from "../shared/ConditionListEditor";
import { deleteRuleDescription } from "./deleteRuleDescription";

const dateFormat = "YYYY-MM-DD";

const actionOptions: { value: AutomationActionType; label: string }[] = [
  { value: "ignore", label: "Ignorar transação" },
  { value: "reconcile", label: "Conciliar recorrência" },
  { value: "set_category", label: "Aplicar categoria" },
];

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

const ruleLogicOptions = (Object.keys(ruleLogicOperatorLabel) as RuleLogicOperator[]).map((value) => ({
  value,
  label: ruleLogicOperatorLabel[value],
}));
const kindOptions = (Object.keys(recurringCommitmentKindLabel) as RecurringCommitmentKind[]).map(
  (value) => ({ value, label: recurringCommitmentKindLabel[value] }),
);
const cadenceOptions = (
  Object.keys(recurringCommitmentCadenceLabel) as RecurringCommitmentCadence[]
).map((value) => ({ value, label: recurringCommitmentCadenceLabel[value] }));
const monthOptions = Array.from({ length: 12 }, (_, index) => ({
  value: index + 1,
  label: new Intl.DateTimeFormat("pt-BR", { month: "long" }).format(new Date(2000, index, 1)),
}));
const commitmentModeOptions: { value: "new" | "existing"; label: string }[] = [
  { value: "new", label: "Nova recorrência" },
  { value: "existing", label: "Recorrência existente" },
];

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

const blankCommitmentDraft: CommitmentDraft = {
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
};

function commitmentDraftFrom(commitment: RecurringCommitment | null): CommitmentDraft {
  if (!commitment) return blankCommitmentDraft;
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
      setCommitmentDraft(blankCommitmentDraft);
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
      setCommitmentDraft(blankCommitmentDraft);
    }
    if (!nextActionTypes.includes("set_category") && !nextActionTypes.includes("reconcile")) {
      setCategoryEdited(false);
      setCategoryId(null);
    }
  };

  const baseFieldConfigs: ConditionFieldConfig[] = [
    {
      field: "description",
      label: ruleConditionFieldLabel.description,
      operators: [
        { value: "contains", label: ruleConditionOperatorLabel.contains },
        { value: "equals", label: ruleConditionOperatorLabel.equals },
      ],
    },
    {
      field: "card",
      label: ruleConditionFieldLabel.card,
      operators: [
        { value: "contains", label: ruleConditionOperatorLabel.contains },
        { value: "equals", label: ruleConditionOperatorLabel.equals },
      ],
      valueOptions: () => conditionOptions.cards.map((value) => ({ value, label: value })),
      valueOptionsLoading: conditionOptionsLoading,
      valuePlaceholder: () => "Selecione um cartão",
      valueNotFoundContent: () => (conditionOptionsLoading ? "Carregando…" : "Nenhum cartão encontrado"),
    },
    {
      field: "account",
      label: ruleConditionFieldLabel.account,
      operators: [
        { value: "contains", label: ruleConditionOperatorLabel.contains },
        { value: "equals", label: ruleConditionOperatorLabel.equals },
      ],
      valueOptions: () => conditionOptions.accounts.map((value) => ({ value, label: value })),
      valueOptionsLoading: conditionOptionsLoading,
      valuePlaceholder: () => "Selecione uma conta",
      valueNotFoundContent: () => (conditionOptionsLoading ? "Carregando…" : "Nenhuma conta encontrada"),
    },
  ];

  const reconcileFieldConfigs: ConditionFieldConfig[] = [
    {
      field: "amount",
      label: ruleConditionFieldLabel.amount,
      operators: [{ value: "within_percent", label: ruleConditionOperatorLabel.within_percent }],
      pairOperator: "within_percent",
      pairFirstLabel: "Valor esperado",
      pairSecondLabel: "Tolerância",
      pairSecondSuffix: "%",
      pairMin: 0,
    },
    {
      field: "day_of_month",
      label: ruleConditionFieldLabel.day_of_month,
      operators: [{ value: "day_range", label: ruleConditionOperatorLabel.day_range }],
      pairOperator: "day_range",
      pairFirstLabel: "Do dia",
      pairSecondLabel: "Até o dia",
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
      label: `${categoryKindLabel[category.kind]}: ${category.name}`,
    }));

  const commitmentCategoryOptions = categories
    .filter((category) => category.is_active && category.kind !== "transfer")
    .map((category) => ({ value: category.id, label: category.name }));

  const submit = () => {
    if (ruleDraft.name.trim() === "") {
      setError("Informe um nome para a regra.");
      return;
    }
    if (ruleDraft.conditions.length === 0) {
      setError("Adicione ao menos uma condição.");
      return;
    }
    if (ruleDraft.conditions.some((condition) => condition.value.trim() === "")) {
      setError("Preencha o valor de todas as condições.");
      return;
    }
    if (ruleDraft.conditions.some(hasZeroAmountReference)) {
      setError("Informe um valor esperado maior que zero.");
      return;
    }
    if (actionTypes.length === 0) {
      setError("Selecione ao menos uma ação.");
      return;
    }
    if (actionTypes.includes("set_category") && !actionTypes.includes("reconcile") && categoryId === null) {
      setError("Selecione uma categoria para aplicar.");
      return;
    }

    let reconcile: AutomationEntrySubmitPayload["reconcile"] = null;
    if (actionTypes.includes("reconcile")) {
      if (commitmentDraft.name.trim() === "") {
        setError("Informe um nome para a recorrência.");
        return;
      }
      if (commitmentDraft.amount === null || !isPositiveDecimal(commitmentDraft.amount)) {
        setError("Informe um valor maior que zero.");
        return;
      }
      if (commitmentDraft.categoryId === null) {
        setError("Selecione uma categoria.");
        return;
      }
      if (commitmentDraft.dayOfMonth === null) {
        setError("Informe o dia do mês.");
        return;
      }
      if (commitmentDraft.cadence === "annual" && commitmentDraft.monthOfYear === null) {
        setError("Informe o mês do ano para uma recorrência anual.");
        return;
      }
      if (commitmentDraft.endDate && commitmentDraft.endDate.isBefore(commitmentDraft.startDate, "day")) {
        setError("A data de término não pode ser anterior à data de início.");
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
      title={isEditing ? "Editar automação" : "Nova automação"}
      open={open}
      onClose={onCancel}
      onSubmit={submit}
      submitting={submitting}
      error={error ?? submitError}
    >
      <FormField label="Nome da automação" htmlFor="automation-rule-name">
        <Input
          id="automation-rule-name"
          value={ruleDraft.name}
          onChange={(event) => setRuleDraft((current) => ({ ...current, name: event.target.value }))}
          placeholder="Ex.: Ignorar assinaturas, Conciliar aluguel"
        />
      </FormField>

      <FormField label="Ativa" htmlFor="automation-rule-active">
        <Switch
          id="automation-rule-active"
          aria-label="Automação ativa"
          checked={ruleDraft.isActive}
          onChange={(checked) => setRuleDraft((current) => ({ ...current, isActive: checked }))}
          style={{ width: "fit-content" }}
        />
      </FormField>

      <FormField label="Condições" labelId="automation-conditions-label">
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
        <FormField label="Combinar condições com" htmlFor="automation-rule-logic">
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

      <FormField label="Ações" labelId="automation-actions-label">
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
          label={actionTypes.includes("reconcile") ? "Categoria a aplicar (opcional)" : "Categoria a aplicar"}
          htmlFor="automation-entry-category"
          hint={actionTypes.includes("reconcile") ? "Aplica a categoria às transações que casarem com as condições. Categorias definidas manualmente são preservadas." : undefined}
        >
          <Select
            id="automation-entry-category"
            allowClear={actionTypes.includes("reconcile")}
            aria-describedby={actionTypes.includes("reconcile") ? "automation-entry-category-hint" : undefined}
            value={categoryId ?? undefined}
            options={ruleCategoryOptions}
            showSearch
            optionFilterProp="label"
            placeholder={actionTypes.includes("reconcile") ? "Sem categoria a aplicar" : "Selecione uma categoria"}
            onChange={(value: string | undefined) => {
              setCategoryEdited(true);
              setCategoryId(value ?? null);
            }}
          />
        </FormField>
      )}

      {(actionTypes.includes("ignore") || (actionTypes.includes("reconcile") ? categoryId !== null : actionTypes.includes("set_category"))) && (
        <Checkbox checked={applyRetroactively} onChange={(event) => setApplyRetroactively(event.target.checked)}>
          Aplicar agora às transações existentes que casarem com as condições
        </Checkbox>
      )}

      {actionTypes.includes("reconcile") && (
        <>
          {!isEditing && (
            <FormField label="Recorrência" labelId="recurring-commitment-mode-label">
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
            <FormField label="Selecione uma recorrência" htmlFor="recurring-commitment-existing">
              <Select
                id="recurring-commitment-existing"
                showSearch
                optionFilterProp="label"
                loading={commitmentsLoading}
                placeholder="Selecione uma recorrência existente"
                value={selectedCommitmentId ?? undefined}
                options={commitments.map((commitment) => ({ value: commitment.id, label: commitment.name }))}
                onChange={(value: string) => selectExistingCommitment(value)}
              />
            </FormField>
          )}

          <Divider orientation="left" plain style={{ margin: 0 }}>
            Dados da recorrência
          </Divider>

          <FormField label="Nome" htmlFor="recurring-commitment-name">
            <Input
              id="recurring-commitment-name"
              value={commitmentDraft.name}
              onChange={(event) => setCommitmentDraft((current) => ({ ...current, name: event.target.value }))}
              placeholder="Ex.: Salário, Aluguel, Netflix"
            />
          </FormField>

          <FormField label="Tipo" htmlFor="recurring-commitment-kind">
            <Select
              id="recurring-commitment-kind"
              value={commitmentDraft.kind}
              options={kindOptions}
              onChange={(value: RecurringCommitmentKind) =>
                setCommitmentDraft((current) => ({ ...current, kind: value }))
              }
            />
          </FormField>

          <FormField label="Valor" htmlFor="recurring-commitment-amount">
            <MoneyInput
              id="recurring-commitment-amount"
              min="0"
              value={commitmentDraft.amount}
              onChange={(value) => setCommitmentDraft((current) => ({ ...current, amount: value }))}
            />
          </FormField>

          <FormField label="Categoria" htmlFor="recurring-commitment-category">
            <Select
              id="recurring-commitment-category"
              value={commitmentDraft.categoryId ?? undefined}
              options={commitmentCategoryOptions}
              showSearch
              optionFilterProp="label"
              placeholder="Selecione uma categoria"
              onChange={(value: string) => setCommitmentDraft((current) => ({ ...current, categoryId: value }))}
            />
          </FormField>

          <FormField label="Conta (opcional)" htmlFor="recurring-commitment-account">
            <AccountSelect
              id="recurring-commitment-account"
              optional
              value={commitmentDraft.accountId}
              onChange={(accountId) => setCommitmentDraft((current) => ({ ...current, accountId: accountId ?? "" }))}
            />
          </FormField>

          <FormField label="Cadência" htmlFor="recurring-commitment-cadence">
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
            label="Dia do mês"
            htmlFor="recurring-commitment-day"
            hint="Usado só para agendar a data esperada no calendário — não obriga a conciliação a exigir esse dia."
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
            <FormField label="Mês do ano" htmlFor="recurring-commitment-month">
              <Select
                id="recurring-commitment-month"
                value={commitmentDraft.monthOfYear ?? undefined}
                options={monthOptions}
                placeholder="Selecione um mês"
                onChange={(value: number) => setCommitmentDraft((current) => ({ ...current, monthOfYear: value }))}
              />
            </FormField>
          )}

          <FormField label="Data de início" htmlFor="recurring-commitment-start-date">
            <DatePicker
              id="recurring-commitment-start-date"
              style={{ width: "100%" }}
              format="DD/MM/YYYY"
              value={commitmentDraft.startDate}
              allowClear={false}
              onChange={(value) => value && setCommitmentDraft((current) => ({ ...current, startDate: value }))}
            />
          </FormField>

          <FormField label="Data de término (opcional)" htmlFor="recurring-commitment-end-date">
            <DatePicker
              id="recurring-commitment-end-date"
              style={{ width: "100%" }}
              format="DD/MM/YYYY"
              value={commitmentDraft.endDate}
              onChange={(value) => setCommitmentDraft((current) => ({ ...current, endDate: value }))}
            />
          </FormField>

          <FormField label="Recorrência ativa" htmlFor="recurring-commitment-active">
            <Switch
              id="recurring-commitment-active"
              aria-label="Recorrência ativa"
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
            title="Excluir automação"
            description={deleteRuleDescription(entry.rule)}
            onConfirm={() => onDelete(entry.rule)}
            okText="Excluir"
            cancelText="Cancelar"
          >
            <Button danger>Excluir automação</Button>
          </Popconfirm>
        </div>
      )}
    </FormDrawer>
  );
}
