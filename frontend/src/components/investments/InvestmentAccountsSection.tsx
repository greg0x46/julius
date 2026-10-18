import { Button, Collapse } from "antd";

import type { InvestmentAccount, InvestmentOperation, InvestmentPosition } from "../../api/contracts";
import { investmentAccountKindLabel } from "../../presentation/investmentWorkspaceLabels";
import { EmptyState, Section } from "../layout";
import { Money } from "../shared/Money";
import { RecordMenu, type RecordMenuItem } from "../shared/RecordMenu";
import { InvestmentOperationsTable } from "./InvestmentOperationsTable";

/**
 * Where the accounts themselves are managed, whatever the positions table is
 * grouped by: an account with no position in the period (or none at all)
 * still has its menu, its cash and its movimentações here. The cash is the
 * account's balance today, not at the end of the period.
 */
export function InvestmentAccountsSection({
  accounts,
  positions,
  operations,
  menuOf,
  onNewAccount,
  onEditOperation,
  onRemoveOperation,
  busy,
}: {
  accounts: InvestmentAccount[];
  /** Every position, closed ones included, so an operation keeps its asset's name. */
  positions: InvestmentPosition[];
  operations: InvestmentOperation[];
  menuOf: (account: InvestmentAccount) => RecordMenuItem[];
  onNewAccount: () => void;
  onEditOperation: (operation: InvestmentOperation) => void;
  onRemoveOperation: (operation: InvestmentOperation) => void;
  busy: boolean;
}) {
  if (accounts.length === 0) {
    return (
      <Section title="Contas e movimentações">
        <EmptyState
          title="Nenhuma conta de investimento ainda"
          hint="Crie uma conta manual ou conecte uma instituição."
          action={<Button onClick={onNewAccount}>Nova conta de investimento</Button>}
        />
      </Section>
    );
  }
  return (
    <Section
      title="Contas e movimentações"
      trailing={`${accounts.length} ${accounts.length === 1 ? "conta" : "contas"}`}
      className="investment-accounts"
    >
      <ul className="investment-account-list">
        {accounts.map((account) => {
          const own = operations.filter((operation) => operation.account_id === account.id);
          return (
            <li key={account.id} className="investment-account">
              <div className="investment-account-header">
                <div className="investment-cell">
                  <span className="investment-cell-title">{account.name}</span>
                  <span className="investment-quiet">
                    Conta {investmentAccountKindLabel[account.kind].toLowerCase()}
                    {!account.active && " · Inativa"}
                  </span>
                </div>
                <div className="investment-account-cash">
                  <span>Disponível hoje</span>
                  <Money currencyCode="BRL" value={account.cash_balance} tone="balance" />
                </div>
                <RecordMenu label={`Ações da conta ${account.name}`} items={menuOf(account)} />
              </div>
              <Collapse
                ghost
                className="investment-operations"
                items={[
                  {
                    key: "operations",
                    label: `Histórico de movimentações (${own.length})`,
                    children: (
                      <InvestmentOperationsTable
                        operations={own}
                        positions={positions}
                        onEdit={onEditOperation}
                        onDelete={onRemoveOperation}
                        busy={busy}
                      />
                    ),
                  },
                ]}
              />
            </li>
          );
        })}
      </ul>
    </Section>
  );
}
