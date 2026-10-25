import { Skeleton, Switch, Table } from "../ui";
import type { ColumnsType } from "../ui/Table";
import type { ReactNode } from "react";

import type { Category, RecurringCommitment } from "../../api/contracts";
import {
  formatNextDay,
  localToday,
  nextOccurrenceDate,
  recurrenceScheduleLabel,
  recurrenceScheduleSentence,
  recurrenceStanding,
  recurringCommitmentKindLabel,
} from "../../presentation/recurringCommitmentLabels";
import { ResponsiveList } from "../layout";
import { Money } from "../shared/Money";
import { RecordMenu } from "../shared/RecordMenu";
import { StatusTag } from "../shared/StatusTag";
import { useConfirm } from "../shared/useConfirm";
import { RecurrenceOccurrenceList } from "./RecurrenceOccurrenceList";

const standingLabel = { paused: "Pausada", ended: "Encerrada" } as const;

const loading = (
  <div className="recurrences-loading" role="status" aria-label="Carregando recorrências">
    <Skeleton active paragraph={{ rows: 3 }} />
  </div>
);

/**
 * The recurrences as one list, two shapes: stacked rows below `lg` — the
 * seven columns are clipped on a tablet — where a tap opens the editor, which
 * also carries the occurrences and the delete; and from `lg` up a table whose
 * rows expand into the occurrences. In the table too a click on the row opens
 * the editor, and the other actions (Editar, Excluir) sit behind one `···`.
 */
export function RecurringCommitmentList({
  commitments,
  categories,
  isLoading,
  togglingCommitmentId,
  empty,
  onEdit,
  onToggle,
  onDelete,
}: {
  commitments: RecurringCommitment[];
  categories: Category[];
  isLoading: boolean;
  togglingCommitmentId: string | null;
  empty: ReactNode;
  onEdit: (commitment: RecurringCommitment) => void;
  onToggle: (commitment: RecurringCommitment, isActive: boolean) => void;
  onDelete: (commitment: RecurringCommitment) => void;
}) {
  const confirm = useConfirm();
  const today = localToday();

  const categoryName = (categoryId: string) =>
    categories.find((category) => category.id === categoryId)?.name ?? "Categoria removida";

  const direction = (commitment: RecurringCommitment) => (commitment.kind === "income" ? "inflow" : "outflow");

  const columns: ColumnsType<RecurringCommitment> = [
    {
      title: "Nome",
      dataIndex: "name",
      render: (_, commitment) => <span className="recurrence-name">{commitment.name}</span>,
    },
    {
      title: "Tipo",
      dataIndex: "kind",
      render: (_, commitment) => recurringCommitmentKindLabel[commitment.kind],
    },
    {
      title: "Valor",
      dataIndex: "amount",
      align: "right",
      render: (_, commitment) => (
        <Money value={commitment.amount} tone="flow" direction={direction(commitment)} size="row" />
      ),
    },
    {
      title: "Categoria",
      dataIndex: "category_id",
      render: (_, commitment) => categoryName(commitment.category_id),
    },
    {
      title: "Recorrência",
      dataIndex: "cadence",
      className: "recurrence-schedule-cell",
      render: (_, commitment) =>
        recurrenceScheduleLabel(commitment.cadence, commitment.day_of_month, commitment.month_of_year),
    },
    {
      title: "Ativa",
      dataIndex: "is_active",
      render: (_, commitment) => (
        <Switch
          aria-label={`${commitment.is_active ? "Pausar" : "Ativar"} recorrência ${commitment.name}`}
          checked={commitment.is_active}
          loading={togglingCommitmentId === commitment.id}
          onChange={(checked) => onToggle(commitment, checked)}
        />
      ),
    },
    {
      title: <span className="visually-hidden">Ações</span>,
      key: "actions",
      align: "right",
      render: (_, commitment) => (
        <RecordMenu
          label={`Ações de ${commitment.name}`}
          items={[
            { key: "edit", label: "Editar", onClick: () => onEdit(commitment) },
            {
              key: "delete",
              label: "Excluir",
              danger: true,
              onClick: () =>
                confirm({
                  title: "Excluir recorrência",
                  description: "Esta ação não pode ser desfeita.",
                  onConfirm: () => onDelete(commitment),
                }),
            },
          ]}
        />
      ),
    },
  ];

  return (
    <ResponsiveList
      label="Recorrências"
      items={commitments}
      getKey={(commitment) => commitment.id}
      isLoading={isLoading}
      loading={loading}
      empty={empty}
      stackBelow="xl"
      row={(commitment) => {
        const standing = recurrenceStanding(commitment, today);
        const next = standing === null ? nextOccurrenceDate(commitment, today) : null;
        const meta = [
          recurringCommitmentKindLabel[commitment.kind],
          recurrenceScheduleSentence(commitment.cadence, commitment.day_of_month, commitment.month_of_year),
          next !== null ? `próxima ${formatNextDay(next, today)}` : null,
        ]
          .filter(Boolean)
          .join(" · ");
        return {
          title: commitment.name,
          meta,
          trailing: <Money value={commitment.amount} tone="flow" direction={direction(commitment)} />,
          status: standing !== null ? <StatusTag tone="neutral">{standingLabel[standing]}</StatusTag> : undefined,
          onClick: () => onEdit(commitment),
          ariaLabel: `Abrir ${commitment.name}`,
        };
      }}
      wide={
        <Table<RecurringCommitment>
          className="recurrences-table"
          aria-label="Recorrências"
          columns={columns}
          dataSource={commitments}
          rowKey="id"
          pagination={false}
          // A tap on the row edits it; the switch, the expand chevron and the
          // menu keep their own meaning (the menu's Editar is the keyboard way).
          onRow={(commitment) => ({
            className: "recurrence-row",
            onClick: (event) => {
              const target = event.target as HTMLElement;
              // A click inside the menu's dropdown bubbles here through the React tree
              // although its DOM lives in a portal: only the row's own DOM counts.
              if (!event.currentTarget.contains(target)) return;
              if (target.closest("button, a, .ui-switch, .ui-table-row-expand-icon")) return;
              onEdit(commitment);
            },
          })}
          // Occurrences load per expanded row rather than with the list: a
          // user opens one recurrence to check it, and eager-loading every
          // row would be one request per recurrence for data usually never
          // looked at.
          expandable={{
            expandedRowRender: (commitment) => <RecurrenceOccurrenceList commitment={commitment} />,
            rowExpandable: (commitment) => commitment.is_active,
          }}
        />
      }
    />
  );
}
