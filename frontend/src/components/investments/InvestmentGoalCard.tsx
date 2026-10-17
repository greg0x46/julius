import { valueInReferenceCurrency } from "../../presentation/valuation";
import type { ReportValuation } from "../../api/contracts/valuation";
import { Progress } from "antd";

import type {
  InvestmentOperation,
  InvestmentPortfolio,
  InvestmentPosition,
  InvestmentSummaryPortfolio,
} from "../../api/contracts";
import { sumBRL } from "../../presentation/money";
import { Section } from "../layout";
import { Money } from "../shared/Money";
import { RecordMenu } from "../shared/RecordMenu";
import { useConfirm } from "../shared/useConfirm";
import { InvestmentFigures, InvestmentYieldValue } from "./InvestmentFigures";
import { InvestmentPositionsTable } from "./InvestmentPositionsTable";
import {
  aggregateYield,
  costsByPosition,
  formatDate,
  goalMovements,
  groupCost,
  latestDate,
  type LinkedInvestments,
} from "./investmentMath";

/**
 * One goal as a Section: a goal only groups positions, so its figures and the
 * positions it holds read the same way an account's do. Editar and Excluir
 * live in one "···" menu on the title line.
 */
export function InvestmentGoalCard({
  portfolio,
  summary,
  valuation,
  positions,
  operations,
  portfolios,
  linked,
  accountNameOf,
  onEdit,
  onRemove,
  onAssignGoal,
  busy,
}: {
  /** null is the "sem objetivo" bucket: positions nobody has grouped yet. */
  portfolio: InvestmentPortfolio | null;
  valuation?: ReportValuation;
  summary: InvestmentSummaryPortfolio | undefined;
  positions: InvestmentPosition[];
  operations: InvestmentOperation[];
  portfolios: InvestmentPortfolio[];
  /** Provider holdings by id: a synced position reads its rendimento and IR/IOF from here. */
  linked: LinkedInvestments;
  accountNameOf: (accountId: string) => string;
  onEdit: (portfolio: InvestmentPortfolio) => void;
  onRemove: (portfolio: InvestmentPortfolio) => void;
  onAssignGoal: (position: InvestmentPosition, portfolioId: string | null) => void;
  busy: boolean;
}) {
  const confirm = useConfirm();
  const movements = goalMovements(operations);
  const convert = (value: string) => valueInReferenceCurrency(value, valuation?.fx);
  const currencyCode = valuation?.currency_code ?? "BRL";
  const currentValue =
    summary?.current_value ?? portfolio?.current_value ?? sumBRL(positions.filter((position) => position.currency_code === "BRL").map((position) => position.current_value));
  const updatedOn = latestDate([
    ...positions.map((position) => position.valued_on),
    ...operations.map((operation) => operation.occurred_on),
  ]);
  const target = summary?.target_amount ?? portfolio?.target_amount ?? null;
  const progress = summary?.progress ?? portfolio?.progress ?? null;

  const costs = costsByPosition(operations, positions, linked);
  const yieldAggregate = aggregateYield(positions, linked, costs);
  const cost = groupCost(operations, positions.filter((position) => position.currency_code === "BRL"), linked);

  return (
    <Section
      title={portfolio?.name ?? "Sem objetivo"}
      className="investment-section"
      trailing={
        portfolio ? (
          <RecordMenu
            label={`Ações de ${portfolio.name}`}
            items={[
              { key: "edit", label: "Editar objetivo", onClick: () => onEdit(portfolio) },
              {
                key: "delete",
                label: "Excluir objetivo",
                danger: true,
                disabled: busy,
                onClick: () =>
                  confirm({
                    title: "Excluir objetivo",
                    description: "As posições continuam onde estão; elas apenas deixam de ter objetivo.",
                    onConfirm: () => onRemove(portfolio),
                  }),
              },
            ]}
          />
        ) : undefined
      }
    >
      <p className="investment-note">Atualizado em {formatDate(updatedOn)}</p>
      <InvestmentFigures
        figures={[
          {
            label: "Valor atual",
            value: <Money report currencyCode={currencyCode} value={currentValue} tone="balance" />,
          },
          {
            label: "Rendimento líquido",
            value: (
              <InvestmentYieldValue
                currencyCode={currencyCode}
                gain={convert(yieldAggregate.gain)}
                percent={yieldAggregate.percent}
              />
            ),
            hint: "Já descontados IR, IOF e taxas das posições que têm rendimento conhecido.",
          },
          {
            label: "IR/Taxas",
            value: <Money report currencyCode={currencyCode} value={convert(cost)} tone="neutral" />,
            hint: "Taxas e impostos das movimentações, mais IR e IOF informados pela instituição.",
          },
          {
            label: "Compras e saldo inicial",
            value: <Money report currencyCode={currencyCode} value={convert(movements.deposits)} tone="neutral" />,
            hint: "Compras e saldos iniciais registrados nas posições deste objetivo.",
          },
          {
            label: "Vendas",
            value: (
              <Money report currencyCode={currencyCode} value={convert(movements.withdrawals)} tone="neutral" />
            ),
            hint: "Vendas registradas nas posições deste objetivo.",
          },
        ]}
      />

      {target !== null && (
        <div className="investment-goal">
          <span>
            Meta de <Money report currencyCode={currencyCode} value={target} tone="neutral" />
          </span>
          {progress !== null && <Progress percent={Math.min(100, Math.round(Number(progress) * 100))} />}
        </div>
      )}

      <InvestmentPositionsTable
        positions={positions}
        portfolios={portfolios}
        costs={costs}
        linked={linked}
        accountNameOf={accountNameOf}
        showAccount
        showGoal={false}
        onAssignGoal={onAssignGoal}
        busy={busy}
        emptyTitle={portfolio ? "Nenhuma posição neste objetivo ainda" : "Todas as posições já têm um objetivo"}
        emptyHint={portfolio ? "Escolha este objetivo em qualquer posição." : undefined}
      />
    </Section>
  );
}
