import { Table } from "antd";
import type { ColumnsType } from "antd/es/table";

import type { InvestmentOperation, InvestmentPosition } from "../../api/contracts";
import { investmentOperationKindLabel } from "../../presentation/investmentWorkspaceLabels";
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
  const confirm = useConfirm();
  const positionName = new Map(positions.map((position) => [position.id, position.name]));
  const positionOf = (operation: InvestmentOperation) =>
    operation.position_id ? positionName.get(operation.position_id) ?? "Posição removida" : "Só caixa";
  const kindOf = (operation: InvestmentOperation) => investmentOperationKindLabel[operation.kind];

  // A synced movement is the institution's record and has no actions at all.
  const menu = (operation: InvestmentOperation) => {
    if (!operation.is_editable) return null;
    const items: RecordMenuItem[] = [
      { key: "edit", label: "Corrigir", disabled: busy, onClick: () => onEdit(operation) },
      {
        key: "delete",
        label: "Excluir",
        danger: true,
        disabled: busy,
        onClick: () =>
          confirm({
            title: "Excluir movimentação",
            description: "Os saldos e custos da conta serão recalculados.",
            onConfirm: () => onDelete(operation),
          }),
      },
    ];
    return <RecordMenu label={`Ações da movimentação de ${formatDate(operation.occurred_on)}`} items={items} />;
  };

  const columns: ColumnsType<InvestmentOperation> = [
    { title: "Data", key: "occurred_on", render: (_, operation) => formatDate(operation.occurred_on) },
    {
      title: "Tipo",
      key: "kind",
      render: (_, operation) => (
        <div className="investment-cell">
          <span className="investment-cell-title">{kindOf(operation)}</span>
          {operation.source === "synced" && <span className="investment-quiet">Informado pela instituição</span>}
        </div>
      ),
    },
    { title: "Posição", key: "position", render: (_, operation) => positionOf(operation) },
    {
      title: "Valor",
      key: "amount",
      align: "right",
      render: (_, operation) => <Money value={operation.amount} tone="neutral" />,
    },
    {
      title: <span className="investment-visually-hidden">Ações</span>,
      key: "actions",
      width: 48,
      render: (_, operation) => menu(operation),
    },
  ];

  return (
    <ResponsiveList
      label="Movimentações"
      stackBelow="lg"
      items={operations}
      getKey={(operation) => operation.id}
      empty={<EmptyState title="Nenhuma movimentação" />}
      row={(operation) => ({
        title: kindOf(operation),
        meta: [
          formatDate(operation.occurred_on),
          positionOf(operation),
          operation.source === "synced" ? "Informado pela instituição" : null,
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
          aria-label="Movimentações"
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
