import { useTranslation } from "../../i18n/context";
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
  recurringCommitmentKindName,
} from "../../presentation/recurringCommitmentLabels";
import { ResponsiveList } from "../layout";
import { Money } from "../shared/Money";
import { RecordMenu } from "../shared/RecordMenu";
import { StatusTag } from "../shared/StatusTag";
import { useConfirm } from "../shared/useConfirm";
import { RecurrenceOccurrenceList } from "./RecurrenceOccurrenceList";

const standingLabelKey = { paused: "standingPaused", ended: "standingEnded" } as const;

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
  const { t, language } = useTranslation();
  const loading = (
    <div className="recurrences-loading" role="status" aria-label={t("loadingRecurrences")}>
      <Skeleton active paragraph={{ rows: 3 }} />
    </div>
  );
  const confirm = useConfirm();
  const today = localToday();

  const categoryName = (categoryId: string) =>
    categories.find((category) => category.id === categoryId)?.name ?? t("removedCategory");

  const direction = (commitment: RecurringCommitment) => (commitment.kind === "income" ? "inflow" : "outflow");

  const columns: ColumnsType<RecurringCommitment> = [
    {
      title: t("name"),
      dataIndex: "name",
      render: (_, commitment) => <span className="recurrence-name">{commitment.name}</span>,
    },
    {
      title: t("type"),
      dataIndex: "kind",
      render: (_, commitment) => recurringCommitmentKindName(commitment.kind, language),
    },
    {
      title: t("amount"),
      dataIndex: "amount",
      align: "right",
      render: (_, commitment) => (
        <Money value={commitment.amount} tone="flow" direction={direction(commitment)} size="row" />
      ),
    },
    {
      title: t("category"),
      dataIndex: "category_id",
      render: (_, commitment) => categoryName(commitment.category_id),
    },
    {
      title: t("recurrence"),
      dataIndex: "cadence",
      className: "recurrence-schedule-cell",
      render: (_, commitment) =>
        recurrenceScheduleLabel(commitment.cadence, commitment.day_of_month, commitment.month_of_year, language),
    },
    {
      title: t("activeFeminineHeader"),
      dataIndex: "is_active",
      render: (_, commitment) => (
        <Switch
          aria-label={t("toggleRecurrence", { action: commitment.is_active ? t("pause") : t("activate"), name: commitment.name })}
          checked={commitment.is_active}
          loading={togglingCommitmentId === commitment.id}
          onChange={(checked) => onToggle(commitment, checked)}
        />
      ),
    },
    {
      title: <span className="visually-hidden">{t("actions")}</span>,
      key: "actions",
      align: "right",
      render: (_, commitment) => (
        <RecordMenu
          label={t("actionsFor", { name: commitment.name })}
          items={[
            { key: "edit", label: t("edit"), onClick: () => onEdit(commitment) },
            {
              key: "delete",
              label: t("delete"),
              danger: true,
              onClick: () =>
                confirm({
                  title: t("deleteRecurrence"),
                  description: t("irreversibleAction"),
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
      label={t("recurrencesTitle")}
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
          recurringCommitmentKindName(commitment.kind, language),
          recurrenceScheduleSentence(commitment.cadence, commitment.day_of_month, commitment.month_of_year, language),
          next !== null ? t("nextOccurrenceOn", { date: formatNextDay(next, today) }) : null,
        ]
          .filter(Boolean)
          .join(" · ");
        return {
          title: commitment.name,
          meta,
          trailing: <Money value={commitment.amount} tone="flow" direction={direction(commitment)} />,
          status: standing !== null ? <StatusTag tone="neutral">{t(standingLabelKey[standing])}</StatusTag> : undefined,
          onClick: () => onEdit(commitment),
          ariaLabel: t("openNamedItem", { name: commitment.name }),
        };
      }}
      wide={
        <Table<RecurringCommitment>
          className="recurrences-table"
          aria-label={t("recurrencesTitle")}
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
