import { Table } from "antd";
import type { ColumnsType } from "antd/es/table";
import type { ReactNode } from "react";
import { Link, useNavigate } from "react-router-dom";

import type { InvestmentPortfolio, InvestmentPosition } from "../../api/contracts";
import {
  investmentPositionSourceLabel,
  investmentValuationBasisLabel,
} from "../../presentation/investmentWorkspaceLabels";
import { formatDecimal } from "../../presentation/investmentLabels";
import { formatBRL, formatMoney } from "../../presentation/money";
import { EmptyState, ResponsiveList } from "../layout";
import { Money } from "../shared/Money";
import { InvestmentYieldValue } from "./InvestmentFigures";
import { formatDate, isZeroBRL, positionYield, type LinkedInvestments } from "./investmentMath";
import { RecordMenu, type RecordMenuItem } from "../shared/RecordMenu";
import { useConfirm } from "../shared/useConfirm";

/** The position's value in its own currency; only BRL goes through the money tone rule. */
function PositionValue({ position }: { position: InvestmentPosition }) {
  return position.currency_code === "BRL" ? (
    <Money value={position.current_value} tone="balance" />
  ) : (
    <>{formatMoney(position.current_value, position.currency_code)}</>
  );
}

/**
 * Rendimento líquido: a signed result with its %, or why there is none. The
 * reason is plain text next to the figure (not a hover tooltip), so a phone
 * can read it too.
 */
function YieldCell({
  position,
  linked,
  cost,
  short = false,
}: {
  position: InvestmentPosition;
  linked: LinkedInvestments;
  cost: string;
  short?: boolean;
}) {
  const estimate = positionYield(position, linked, cost);
  if (!estimate.known) {
    return <span className="investment-quiet">{short ? estimate.short : estimate.reason}</span>;
  }
  return <InvestmentYieldValue gain={estimate.value} percent={estimate.percent} />;
}

export function InvestmentPositionsTable({
  positions,
  portfolios,
  costs,
  linked,
  accountNameOf,
  showAccount = false,
  showGoal = true,
  onAssignGoal,
  onEdit,
  onDelete,
  markManual = false,
  busy,
  emptyTitle,
  emptyHint,
  emptyAction,
}: {
  positions: InvestmentPosition[];
  portfolios: InvestmentPortfolio[];
  /** position_id -> taxas e impostos lançados nas movimentações dessa posição. */
  costs: Record<string, string>;
  linked: LinkedInvestments;
  accountNameOf?: (accountId: string) => string;
  showAccount?: boolean;
  /** The goal column: off inside a goal's own section, where every row has that goal. */
  showGoal?: boolean;
  onAssignGoal: (position: InvestmentPosition, portfolioId: string | null) => void;
  onEdit?: (position: InvestmentPosition) => void;
  onDelete?: (position: InvestmentPosition) => void;
  /** Tags manual rows of an integrated account, where they sit next to the provider's. */
  markManual?: boolean;
  busy: boolean;
  emptyTitle: string;
  emptyHint?: ReactNode;
  emptyAction?: ReactNode;
}) {
  const navigate = useNavigate();
  const confirm = useConfirm();
  const goalName = (position: InvestmentPosition) =>
    portfolios.find((portfolio) => portfolio.id === position.portfolio_id)?.name ?? null;
  const subtitle = (position: InvestmentPosition) =>
    [position.ticker, position.asset_type].filter(Boolean).join(" · ");
  const costOf = (position: InvestmentPosition) => costs[position.id] ?? null;
  const manualTag = (position: InvestmentPosition) =>
    markManual && position.source === "manual" ? investmentPositionSourceLabel.manual : null;

  /**
   * An imported position is the institution's record: only the goal, which is
   * a local grouping, is ours to change. The goal is plain text in the row at
   * every width and is changed here, from "Mudar objetivo" — a bordered
   * select on every row made the table read as a form.
   */
  const menuItems = (position: InvestmentPosition): RecordMenuItem[] => {
    const items: RecordMenuItem[] = [];
    if (position.linked_investment_id) {
      items.push({
        key: "details",
        label: "Ver detalhes",
        onClick: () => navigate(`/investimentos/${position.linked_investment_id}`),
      });
    }
    if (portfolios.length > 0) {
      items.push({
        key: "goal",
        label: "Mudar objetivo",
        disabled: busy,
        children: [
          {
            key: "goal-none",
            label: "Sem objetivo",
            disabled: position.portfolio_id === null,
            onClick: () => onAssignGoal(position, null),
          },
          ...portfolios.map((portfolio) => ({
            key: `goal-${portfolio.id}`,
            label: portfolio.name,
            disabled: position.portfolio_id === portfolio.id,
            onClick: () => onAssignGoal(position, portfolio.id),
          })),
        ],
      });
    }
    if (position.source !== "synced") {
      if (onEdit) items.push({ key: "edit", label: "Editar", onClick: () => onEdit(position) });
      if (onDelete) {
        items.push({
          key: "delete",
          label: "Excluir",
          danger: true,
          onClick: () =>
            confirm({
              title: "Excluir posição",
              description: "Só é possível excluir uma posição sem movimentações.",
              onConfirm: () => onDelete(position),
            }),
        });
      }
    }
    return items;
  };

  const menu = (position: InvestmentPosition) => (
    <RecordMenu label={`Ações de ${position.name}`} items={menuItems(position)} />
  );

  const columns: ColumnsType<InvestmentPosition> = [
    {
      title: "Posição",
      key: "name",
      render: (_, position) => (
        <div className="investment-cell">
          <span className="investment-cell-title">
            {/* Same look with or without a detail page behind it: the name
                is text; the link shows on hover and in the "···" menu. */}
            {position.linked_investment_id ? (
              <Link className="investment-name-link" to={`/investimentos/${position.linked_investment_id}`}>
                {position.name}
              </Link>
            ) : (
              position.name
            )}
            {manualTag(position) && <span className="investment-quiet"> · {manualTag(position)}</span>}
            {position.closed && <span className="investment-quiet"> · Encerrada</span>}
          </span>
          <span className="investment-quiet">{subtitle(position)}</span>
        </div>
      ),
    },
    ...(showAccount
      ? [
          {
            title: "Conta",
            key: "account",
            render: (_: unknown, position: InvestmentPosition) =>
              accountNameOf?.(position.account_id) ?? "Conta não encontrada",
          },
        ]
      : []),
    ...(showGoal
      ? [
          {
            title: "Objetivo",
            key: "portfolio",
            render: (_: unknown, position: InvestmentPosition) => {
              const name = goalName(position);
              return name !== null ? name : <span className="investment-quiet">Sem objetivo</span>;
            },
          },
        ]
      : []),
    {
      title: "Quantidade",
      key: "quantity",
      align: "right",
      render: (_, position) => (
        <div className="investment-cell investment-cell-end">
          <span className="investment-number">{formatDecimal(position.quantity)}</span>
          {!isZeroBRL(position.average_cost) && (
            <span className="investment-quiet">Custo médio {formatBRL(position.average_cost as string)}</span>
          )}
        </div>
      ),
    },
    {
      title: "Valor atual",
      key: "current_value",
      align: "right",
      render: (_, position) => (
        <div className="investment-cell investment-cell-end">
          <span className="investment-cell-value">
            <PositionValue position={position} />
          </span>
          <span className="investment-quiet">
            {investmentValuationBasisLabel[position.valuation_basis]} · {formatDate(position.valued_on)}
          </span>
        </div>
      ),
    },
    {
      title: "Rendimento líquido",
      key: "yield",
      align: "right",
      render: (_, position) => {
        const cost = costOf(position);
        return (
          <div className="investment-cell investment-cell-end">
            <YieldCell position={position} linked={linked} cost={cost ?? "0"} />
            {!isZeroBRL(cost) && <span className="investment-quiet">IR/Taxas {formatBRL(cost as string)}</span>}
          </div>
        );
      },
    },
    {
      title: <span className="investment-visually-hidden">Ações</span>,
      key: "actions",
      width: 48,
      render: (_, position) => menu(position),
    },
  ];

  return (
    <ResponsiveList
      label="Posições"
      stackBelow="lg"
      items={positions}
      getKey={(position) => position.id}
      empty={<EmptyState title={emptyTitle} hint={emptyHint} action={emptyAction} />}
      row={(position) => ({
        title: position.name,
        meta: [manualTag(position), subtitle(position), showAccount ? accountNameOf?.(position.account_id) : goalName(position)]
          .filter(Boolean)
          .join(" · "),
        trailing: (
          <span className="investment-row-trailing">
            <span className="investment-row-figures">
              <span className="investment-row-value">
                <PositionValue position={position} />
              </span>
              <span className="investment-row-yield">
                <YieldCell position={position} linked={linked} cost={costOf(position) ?? "0"} short />
              </span>
            </span>
            {menu(position)}
          </span>
        ),
        status: position.closed ? "Encerrada" : undefined,
        ariaLabel: position.name,
      })}
      wide={
        <Table<InvestmentPosition>
          aria-label="Posições"
          className="investment-table"
          size="small"
          rowKey="id"
          columns={columns}
          dataSource={positions}
          pagination={false}
        />
      }
    />
  );
}
