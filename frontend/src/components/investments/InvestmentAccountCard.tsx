import { Button, Collapse } from "antd";

import type {
  InvestmentAccount,
  InvestmentOperation,
  InvestmentPortfolio,
  InvestmentPosition,
  InvestmentSummaryAccount,
} from "../../api/contracts";
import { investmentAccountKindLabel } from "../../presentation/investmentWorkspaceLabels";
import { sumBRL } from "../../presentation/money";
import { Section } from "../layout";
import { Money } from "../shared/Money";
import { RecordMenu, type RecordMenuItem } from "../shared/RecordMenu";
import { useConfirm } from "../shared/useConfirm";
import { InvestmentFigures, InvestmentYieldValue } from "./InvestmentFigures";
import { InvestmentOperationsTable } from "./InvestmentOperationsTable";
import { InvestmentPositionsTable } from "./InvestmentPositionsTable";
import {
  accountMovements,
  aggregateYield,
  costsByPosition,
  formatDate,
  groupCost,
  latestDate,
  type LinkedInvestments,
} from "./investmentMath";

/**
 * One investment account as a Section: its figures, its positions as rows
 * (a table from `md` up) and its movements behind a disclosure. The account's
 * own actions — Registrar movimentação, Nova posição, Editar, Excluir — sit in
 * one "···" menu on the title line instead of a row of buttons.
 */
export function InvestmentAccountCard({
  account,
  summary,
  positions,
  allPositions,
  operations,
  portfolios,
  linked,
  onRename,
  onRemove,
  onNewPosition,
  onNewOperation,
  onEditPosition,
  onRemovePosition,
  onEditOperation,
  onRemoveOperation,
  onAssignGoal,
  busy,
}: {
  account: InvestmentAccount;
  summary: InvestmentSummaryAccount | undefined;
  /** Rows of the positions table (closed ones hidden unless asked for). */
  positions: InvestmentPosition[];
  /** Every position of the account, so operations of a closed one keep its name. */
  allPositions: InvestmentPosition[];
  operations: InvestmentOperation[];
  portfolios: InvestmentPortfolio[];
  /** Provider holdings by id: a synced position reads its rendimento and IR/IOF from here. */
  linked: LinkedInvestments;
  onRename: (account: InvestmentAccount) => void;
  onRemove: (account: InvestmentAccount) => void;
  onNewPosition: (account: InvestmentAccount) => void;
  onNewOperation: (account: InvestmentAccount) => void;
  onEditPosition: (position: InvestmentPosition) => void;
  onRemovePosition: (position: InvestmentPosition) => void;
  onEditOperation: (operation: InvestmentOperation) => void;
  onRemoveOperation: (operation: InvestmentOperation) => void;
  onAssignGoal: (position: InvestmentPosition, portfolioId: string | null) => void;
  busy: boolean;
}) {
  const confirm = useConfirm();
  const editable = account.kind === "manual";
  const movements = accountMovements(operations);
  // The server already nets the account's value; positions alone would miss
  // the cash sitting in the investment account.
  const currentValue = sumBRL([summary?.current_value ?? sumBRL(positions.filter((position) => position.currency_code === "BRL").map((position) => position.current_value)), summary?.cash_balance ?? account.cash_balance]);
  const updatedOn = latestDate([
    account.updated_at,
    ...positions.map((position) => position.valued_on),
    ...operations.map((operation) => operation.occurred_on),
  ]);

  // Costs and rendimento read every position of the account, closed ones
  // included: hiding a sold position's row must not change what it cost.
  const costs = costsByPosition(operations, allPositions, linked);
  const yieldAggregate = aggregateYield(positions, linked, costs);
  const cost = groupCost(operations, allPositions, linked);

  const menuItems: RecordMenuItem[] = [
    { key: "operation", label: "Registrar movimentação", onClick: () => onNewOperation(account) },
    { key: "position", label: "Nova posição", onClick: () => onNewPosition(account) },
    ...(editable
      ? [
          { key: "edit", label: "Editar conta", onClick: () => onRename(account) },
          {
            key: "delete",
            label: "Excluir conta",
            danger: true,
            disabled: busy,
            onClick: () =>
              confirm({
                title: "Excluir conta de investimento",
                description: "Só é possível excluir uma conta sem posições nem movimentações.",
                onConfirm: () => onRemove(account),
              }),
          },
        ]
      : [
          {
            key: "link",
            label: account.financial_account_id === null ? "Vincular caixa da corretora" : "Caixa da corretora",
            onClick: () => onRename(account),
          },
        ]),
  ];

  return (
    <Section
      title={account.name}
      trailing={<RecordMenu label={`Ações de ${account.name}`} items={menuItems} />}
      className="investment-section"
    >
      <p className="investment-note">
        Conta {investmentAccountKindLabel[account.kind].toLowerCase()} · Atualizado em {formatDate(updatedOn)}
        {!account.active && " · Inativa"}
      </p>
      {!editable && (
        <p className="investment-note">
          Saldos e posições da instituição vêm dela e não podem ser alterados. Registre aportes, resgates e
          rendimentos para conciliá-los com o extrato, ou adicione posições manuais, como criptoativos que a
          instituição não informa.
        </p>
      )}

      <InvestmentFigures
        figures={[
          { label: "Valor atual", value: <Money value={currentValue} tone="balance" /> },
          {
            label: "Rendimento líquido",
            value: <InvestmentYieldValue gain={yieldAggregate.gain} percent={yieldAggregate.percent} />,
            hint: "Já descontados IR, IOF e taxas das posições que têm rendimento conhecido.",
          },
          {
            label: "IR/Taxas",
            value: <Money value={cost} tone="neutral" />,
            hint: "Taxas e impostos das movimentações, mais IR e IOF informados pela instituição.",
          },
          {
            label: "Aportes",
            value: <Money value={movements.deposits} tone="neutral" />,
            hint: "Dinheiro que entrou nesta conta de investimento, vindo do seu caixa, desde o início.",
          },
          {
            label: "Resgates",
            value: <Money value={movements.withdrawals} tone="neutral" />,
            hint: "Dinheiro que saiu desta conta de investimento de volta para o seu caixa, desde o início.",
          },
          {
            label: "Saldo para investir",
            value: <Money value={summary?.cash_balance ?? account.cash_balance} tone="neutral" />,
            hint: "Valor já aportado que ainda não foi aplicado em nenhuma posição.",
          },
        ]}
      />

      <InvestmentPositionsTable
        positions={positions}
        portfolios={portfolios}
        costs={costs}
        linked={linked}
        onAssignGoal={onAssignGoal}
        onEdit={onEditPosition}
        onDelete={onRemovePosition}
        markManual={!editable}
        busy={busy}
        emptyTitle="Nenhuma posição nesta conta"
        emptyAction={<Button onClick={() => onNewPosition(account)}>Nova posição</Button>}
      />

      <Collapse
        ghost
        className="investment-operations"
        items={[
          {
            key: "operations",
            label: `Movimentações (${operations.length})`,
            children: (
              <InvestmentOperationsTable
                operations={operations}
                positions={allPositions}
                onEdit={onEditOperation}
                onDelete={onRemoveOperation}
                busy={busy}
              />
            ),
          },
        ]}
      />
    </Section>
  );
}
