import { useTranslation } from "../../i18n/context";
import { Table } from "../ui";
import type { ColumnsType } from "../ui/Table";

import type { InvestmentOperation, InvestmentPosition } from "../../api/contracts";
import { investmentOperationKindName } from "../../presentation/investmentWorkspaceLabels";
import { EmptyState, ResponsiveList } from "../layout";
import { Money } from "../shared/Money";
import { formatDate } from "./investmentDate";
import { RecordMenu, type RecordMenuItem } from "../shared/RecordMenu";
import { useConfirm } from "../shared/useConfirm";

export function InvestmentOperationsTable({
  operations,
  positions,
  onEdit,
  onDelete,
  busy,
}: {
  operations: InvestmentOperation[];
  positions: InvestmentPosition[];
  onEdit: (operation: InvestmentOperation) => void;
  onDelete: (operation: InvestmentOperation) => void;
  busy: boolean;
}) {
  const { t, language } = useTranslation();
  const confirm = useConfirm();
  const positionName = new Map(positions.map((position) => [position.id, position.name]));
  const positionOf = (operation: InvestmentOperation) =>
    operation.position_id ? positionName.get(operation.position_id) ?? t("removedPosition") : t("cashOnly");
  const kindOf = (operation: InvestmentOperation) => investmentOperationKindName(operation.kind, language);

  // A synced movement is the institution's record and has no actions at all.
  const menu = (operation: InvestmentOperation) => {
    if (!operation.is_editable) return null;
    const items: RecordMenuItem[] = [
      { key: "edit", label: t("correct"), disabled: busy, onClick: () => onEdit(operation) },
      {
        key: "delete",
        label: t("delete"),
        danger: true,
        disabled: busy,
        onClick: () =>
          confirm({
            title: t("deleteOperation"),
            description: t("deleteOperationHint"),
            onConfirm: () => onDelete(operation),
          }),
      },
    ];
    return <RecordMenu label={t("operationActionsOn", { date: formatDate(operation.occurred_on, language) })} items={items} />;
  };

  const columns: ColumnsType<InvestmentOperation> = [
    { title: t("date"), key: "occurred_on", render: (_, operation) => formatDate(operation.occurred_on, language) },
    {
      title: t("type"),
      key: "kind",
      render: (_, operation) => (
        <div className="investment-cell">
          <span className="investment-cell-title">{kindOf(operation)}</span>
          {operation.source === "synced" && <span className="investment-quiet">{t("reportedByInstitution")}</span>}
        </div>
      ),
    },
    { title: t("position"), key: "position", render: (_, operation) => positionOf(operation) },
    {
      title: t("amount"),
      key: "amount",
      align: "right",
      render: (_, operation) => <Money value={operation.amount} tone="neutral" />,
    },
    {
      title: <span className="investment-visually-hidden">{t("actions")}</span>,
      key: "actions",
      width: 48,
      render: (_, operation) => menu(operation),
    },
  ];

  return (
    <ResponsiveList
      label={t("operations")}
      stackBelow="lg"
      items={operations}
      getKey={(operation) => operation.id}
      empty={<EmptyState title={t("noOperations")} />}
      row={(operation) => ({
        title: kindOf(operation),
        meta: [
          formatDate(operation.occurred_on, language),
          positionOf(operation),
          operation.source === "synced" ? t("reportedByInstitution") : null,
        ]
          .filter(Boolean)
          .join(" · "),
        trailing: (
          <span className="investment-row-trailing">
            <span className="investment-row-value">
              <Money value={operation.amount} tone="neutral" />
            </span>
            {menu(operation)}
          </span>
        ),
        ariaLabel: kindOf(operation),
      })}
      wide={
        <Table<InvestmentOperation>
          aria-label={t("operations")}
          className="investment-table"
          size="small"
          rowKey="id"
          columns={columns}
          dataSource={operations}
          pagination={false}
        />
      }
    />
  );
}
