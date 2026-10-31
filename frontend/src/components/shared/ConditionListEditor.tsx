import { useTranslation } from "../../i18n/context";
import { DeleteOutlined, PlusOutlined } from "../ui/icons";
import { Button, Flex, Input, Select } from "../ui";

import type { RuleCondition, RuleConditionField, RuleConditionOperator } from "../../api/contracts";

/**
 * Per-field rendering rules for one row of the shared condition-list
 * builder (used by AutomationRuleForm and RecurringCommitmentForm — see
 * docs/development/architecture.md, motor de regras). A field is either:
 * - a literal text-match field (operator select + free/select value), or
 * - a "pair" field (amount/day_of_month): the operator is implied by the
 *   field itself, and the input is two plain numbers packed into
 *   Condition.Value as "<first>:<second>" — a base value + tolerance for
 *   amount, a min/max day for day_of_month. Both are typed in full by the
 *   user; neither depends on the reconcile action's linked commitment for
 *   its reference (see internal/rules and internal/recurrences/match.go).
 */
export interface ConditionFieldConfig {
  field: RuleConditionField;
  label: string;
  operators: { value: RuleConditionOperator; label: string }[];
  pairOperator?: RuleConditionOperator;
  pairFirstLabel?: string;
  pairFirstSuffix?: string;
  pairSecondLabel?: string;
  pairSecondSuffix?: string;
  pairMin?: number;
  pairMax?: number;
  valueOptions?: (field: RuleConditionField) => { value: string; label: string }[];
  valueOptionsLoading?: boolean;
  valuePlaceholder?: (field: RuleConditionField) => string;
  valueNotFoundContent?: (field: RuleConditionField) => string;
}

function blankConditionFor(config: ConditionFieldConfig): RuleCondition {
  return {
    field: config.field,
    operator: config.pairOperator ?? config.operators[0].value,
    value: "",
  };
}

function parsePair(value: string): [string, string] {
  const [first, second] = value.split(":");
  return [first ?? "", second ?? ""];
}

export function ConditionListEditor({
  conditions,
  onChange,
  fieldConfigs,
  minConditions = 1,
}: {
  conditions: RuleCondition[];
  onChange: (conditions: RuleCondition[]) => void;
  fieldConfigs: ConditionFieldConfig[];
  minConditions?: number;
}) {
  const { t } = useTranslation();
  const fieldOptions = fieldConfigs.map((config) => ({ value: config.field, label: config.label }));
  const configFor = (field: RuleConditionField) =>
    fieldConfigs.find((config) => config.field === field) ?? fieldConfigs[0];

  const updateCondition = (index: number, patch: Partial<RuleCondition>) => {
    onChange(conditions.map((condition, i) => (i === index ? { ...condition, ...patch } : condition)));
  };

  const changeField = (index: number, field: RuleConditionField) => {
    const config = configFor(field);
    updateCondition(index, {
      field,
      operator: config.pairOperator ?? config.operators[0].value,
      value: "",
    });
  };

  const removeCondition = (index: number) => {
    onChange(conditions.filter((_, i) => i !== index));
  };

  const addCondition = () => {
    onChange([...conditions, blankConditionFor(fieldConfigs[0])]);
  };

  return (
    <Flex vertical gap="small">
      {conditions.map((condition, index) => {
        const config = configFor(condition.field);
        return (
          <Flex key={index} gap="small" align="center" className="condition-row">
            <Select
              aria-label={t("field")}
              value={condition.field}
              options={fieldOptions}
              onChange={(value: RuleConditionField) => changeField(index, value)}
            />
            {config.pairOperator ? (
              (() => {
                const [first, second] = parsePair(condition.value);
                return (
                  <>
                    <Input
                      aria-label={config.pairFirstLabel ?? t("valueLabel")}
                      type="number"
                      min={config.pairMin}
                      max={config.pairMax}
                      value={first}
                      onChange={(event) => updateCondition(index, { value: `${event.target.value}:${second}` })}
                      placeholder={config.pairFirstLabel ?? t("valueLabel")}
                      suffix={config.pairFirstSuffix}
                    />
                    <Input
                      aria-label={config.pairSecondLabel ?? t("valueLabel")}
                      type="number"
                      min={config.pairMin}
                      max={config.pairMax}
                      value={second}
                      onChange={(event) => updateCondition(index, { value: `${first}:${event.target.value}` })}
                      placeholder={config.pairSecondLabel ?? t("valueLabel")}
                      suffix={config.pairSecondSuffix}
                    />
                  </>
                );
              })()
            ) : (
              <>
                <Select
                  aria-label={t("operator")}
                  value={condition.operator}
                  options={config.operators}
                  onChange={(value: RuleConditionOperator) => updateCondition(index, { operator: value })}
                />
                {config.valueOptions ? (
                  <Select
                    aria-label={t("valueLabel")}
                    value={condition.value || undefined}
                    options={config.valueOptions(condition.field)}
                    loading={config.valueOptionsLoading}
                    showSearch
                    optionFilterProp="label"
                    placeholder={config.valuePlaceholder?.(condition.field) ?? t("select")}
                    notFoundContent={config.valueNotFoundContent?.(condition.field)}
                    onChange={(value: string) => updateCondition(index, { value })}
                  />
                ) : (
                  <Input
                    aria-label={t("valueLabel")}
                    value={condition.value}
                    onChange={(event) => updateCondition(index, { value: event.target.value })}
                    placeholder={t("valueLabel")}
                  />
                )}
              </>
            )}
            <Button
              aria-label={t("removeCondition")}
              icon={<DeleteOutlined aria-hidden="true" />}
              disabled={conditions.length <= minConditions}
              onClick={() => removeCondition(index)}
            />
          </Flex>
        );
      })}
      <Button icon={<PlusOutlined aria-hidden="true" />} onClick={addCondition}>
        {t("addCondition")}
      </Button>
    </Flex>
  );
}
