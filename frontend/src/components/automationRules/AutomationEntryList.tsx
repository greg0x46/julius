import type { Column } from "../ui/Table";
import Table from "../ui/Table";
import { Skeleton, Switch } from "../ui";

import type { AutomationActionType, AutomationRule, Category, RecurringCommitment } from "../../api/contracts";
import { summarizeConditions } from "../../presentation/ruleConditionLabels";
import { DataCard, EmptyState, ResponsiveList } from "../layout";
import { RecordMenu } from "../shared/RecordMenu";
import { StatusTag } from "../shared/StatusTag";
import { useConfirm } from "../shared/useConfirm";
import { deleteRuleDescription } from "./deleteRuleDescription";

type EntryRow = {
  key: string;
  rule: AutomationRule;
};

const actionLabel: Record<AutomationActionType, string> = {
  ignore: "Ignorar",
  reconcile: "Conciliar",
  set_category: "Aplicar categoria",
};

/**
 * The rules as one list, two shapes: stacked rows below `lg` (the table has
 * five columns and clips well above a phone) and a table from there up. In
 * both, a tap on the row opens the editor; the table adds the active switch
 * and a `···` with Editar (the keyboard's way in) and Excluir. What a rule
 * does is plain text — every rule has an action, so a tag on each would say
 * nothing; only "Inativa" is a state worth a tag.
 */
export function AutomationEntryList({
  rules,
  commitments,
  categories,
  isLoading,
  togglingRuleId,
  onEditRule,
  onToggleRule,
  onDeleteRule,
}: {
  rules: AutomationRule[];
  commitments: RecurringCommitment[];
  categories: Category[];
  isLoading: boolean;
  togglingRuleId: string | null;
  onEditRule: (rule: AutomationRule) => void;
  onToggleRule: (rule: AutomationRule, isActive: boolean) => void;
  onDeleteRule: (rule: AutomationRule) => void | Promise<unknown>;
}) {
  const confirm = useConfirm();
  const commitmentName = (scenarioId: string) =>
    commitments.find((commitment) => commitment.id === scenarioId)?.name ??
    "Recorrência removida";
  const categoryName = (categoryId: string) =>
    categories.find((category) => category.id === categoryId)?.name ?? "Categoria removida";

  const actionText = (action: AutomationRule["actions"][number]) =>
    action.type === "reconcile" && action.scenario_id
      ? `Concilia: ${commitmentName(action.scenario_id)}`
      : action.type === "set_category" && action.category_id
        ? `Categoriza: ${categoryName(action.category_id)}`
        : actionLabel[action.type];

  const rows: EntryRow[] = rules.map((rule) => ({ key: rule.id, rule }));

  const columns: Column<EntryRow>[] = [
    { title: "Nome", dataIndex: ["rule", "name"] },
    {
      title: "Ações",
      key: "actions",
      render: (_, row) => (
        <ul className="automation-actions">
          {row.rule.actions.map((action) => (
            <li key={action.type}>{actionText(action)}</li>
          ))}
        </ul>
      ),
    },
    {
      title: "Condições",
      key: "summary",
      render: (_, row) => summarizeConditions(row.rule.conditions, row.rule.logic_operator),
    },
    {
      title: "Ativa",
      dataIndex: ["rule", "is_active"],
      render: (_, row) => (
        <span onClick={(event) => event.stopPropagation()}>
          <Switch
            aria-label={`${row.rule.is_active ? "Desativar" : "Ativar"} automação ${row.rule.name}`}
            checked={row.rule.is_active}
            loading={togglingRuleId === row.rule.id}
            onChange={(checked) => onToggleRule(row.rule, checked)}
          />
        </span>
      ),
    },
    {
      title: <span className="visually-hidden">Ações da automação</span>,
      key: "options",
      width: 56,
      render: (_, row) => (
        <span onClick={(event) => event.stopPropagation()}>
          <RecordMenu
            label={`Ações de ${row.rule.name}`}
            items={[
              { key: "edit", label: "Editar", onClick: () => onEditRule(row.rule) },
              {
                key: "delete",
                label: "Excluir",
                danger: true,
                onClick: () =>
                  confirm({
                    title: "Excluir automação",
                    description: deleteRuleDescription(row.rule),
                    onConfirm: () => onDeleteRule(row.rule),
                  }),
              },
            ]}
          />
        </span>
      ),
    },
  ];

  return (
    <DataCard flush className="settings-list-card">
      <ResponsiveList<AutomationRule>
        label="Automações"
        items={rules}
        getKey={(rule) => rule.id}
        stackBelow="xl"
        isLoading={isLoading}
        loading={<Skeleton active paragraph={{ rows: 4 }} />}
        empty={
          <EmptyState
            title="Nenhuma automação ainda"
            hint="Crie regras no estilo de filtros de e-mail: ignore transações repetidas ou concilie recorrências (salário, aluguel, assinaturas) com as transações reais."
          />
        }
        row={(rule) => ({
          title: rule.name,
          meta: (
            <span className="meta-clamp">
              {[rule.actions.map(actionText).join(", "), summarizeConditions(rule.conditions, rule.logic_operator)]
                .filter((part) => part !== "")
                .join(" · ")}
            </span>
          ),
          status: rule.is_active ? undefined : <StatusTag tone="neutral">Inativa</StatusTag>,
          onClick: () => onEditRule(rule),
          ariaLabel: `Editar automação ${rule.name}`,
        })}
        wide={
          <Table<EntryRow>
            aria-label="Automações"
            columns={columns}
            dataSource={rows}
            rowKey="key"
                pagination={false}
            onRow={(row) => ({ onClick: () => onEditRule(row.rule), style: { cursor: "pointer" } })}
          />
        }
      />
    </DataCard>
  );
}
