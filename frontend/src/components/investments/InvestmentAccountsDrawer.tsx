import { ArrowLeftOutlined } from "@ant-design/icons";
import { Button, Drawer } from "antd";

import type { InvestmentAccount, InvestmentOperation, InvestmentPosition } from "../../api/contracts";
import { investmentAccountKindLabel } from "../../presentation/investmentWorkspaceLabels";
import { EmptyState } from "../layout";
import { Money } from "../shared/Money";
import { RecordMenu, type RecordMenuItem } from "../shared/RecordMenu";
import { useCompactScreen } from "../shared/useCompactScreen";
import { InvestmentOperationsTable } from "./InvestmentOperationsTable";

/** What the drawer shows: the account list, one account's movements, or one position's. */
export interface InvestmentAccountsScope {
  accountId: string | null;
  positionId: string | null;
}

/**
 * Where accounts are managed whatever the positions table is grouped by: an
 * account with no position in the period still has its menu, its cash and its
 * movimentações here. The cash is the account's balance today, not at the end
 * of the period.
 */
export function InvestmentAccountsDrawer({
  scope,
  hidden = false,
  accounts,
  positions,
  operations,
  menuOf,
  onNewAccount,
  onScope,
  onEditOperation,
  onRemoveOperation,
  onClose,
  busy,
}: {
  scope: InvestmentAccountsScope | null;
  /** Out of sight while a form is open, keeping its scope for when the form closes. */
  hidden?: boolean;
  accounts: InvestmentAccount[];
  /** Every position, closed ones included, so an operation keeps its asset's name. */
  positions: InvestmentPosition[];
  operations: InvestmentOperation[];
  menuOf: (account: InvestmentAccount) => RecordMenuItem[];
  onNewAccount: () => void;
  onScope: (scope: InvestmentAccountsScope) => void;
  onEditOperation: (operation: InvestmentOperation) => void;
  onRemoveOperation: (operation: InvestmentOperation) => void;
  onClose: () => void;
  busy: boolean;
}) {
  const compact = useCompactScreen();
  const account = accounts.find((candidate) => candidate.id === scope?.accountId);
  const position = positions.find((candidate) => candidate.id === scope?.positionId);

  const title = position?.name ?? account?.name ?? "Contas de investimento";
  const shown = operations.filter((operation) =>
    position ? operation.position_id === position.id : operation.account_id === account?.id,
  );

  return (
    <Drawer
      open={scope !== null && !hidden}
      onClose={onClose}
      width={compact ? "100%" : 560}
      destroyOnHidden
      // The record menu's bottom sheet nests inside this drawer; antd would push the drawer aside for it.
      push={false}
      title={
        <span className="investment-drawer-title">
          {account && (
            <Button
              type="text"
              icon={<ArrowLeftOutlined aria-hidden="true" />}
              aria-label="Voltar para as contas"
              onClick={() => onScope({ accountId: null, positionId: null })}
            />
          )}
          {title}
        </span>
      }
      extra={account ? <RecordMenu label={`Ações da conta ${account.name}`} items={menuOf(account)} /> : undefined}
    >
      {account ? (
        <>
          <div className="investment-account-cash">
            <span>
              Disponível hoje · conta {investmentAccountKindLabel[account.kind].toLowerCase()}
              {!account.active && " · Inativa"}
            </span>
            <Money currencyCode="BRL" value={account.cash_balance} tone="balance" />
          </div>
          <InvestmentOperationsTable
            operations={shown}
            positions={positions}
            onEdit={onEditOperation}
            onDelete={onRemoveOperation}
            busy={busy}
          />
        </>
      ) : accounts.length === 0 ? (
        <EmptyState
          title="Nenhuma conta de investimento ainda"
          hint="Crie uma conta manual ou conecte uma instituição."
          action={<Button onClick={onNewAccount}>Nova conta de investimento</Button>}
        />
      ) : (
        <ul className="investment-account-list">
          {accounts.map((item) => (
            <li key={item.id} className="investment-account">
              <div className="investment-account-header">
                <div className="investment-cell">
                  <Button
                    type="link"
                    className="investment-account-name"
                    onClick={() => onScope({ accountId: item.id, positionId: null })}
                  >
                    {item.name}
                  </Button>
                  <span className="investment-quiet">
                    Conta {investmentAccountKindLabel[item.kind].toLowerCase()}
                    {!item.active && " · Inativa"}
                  </span>
                </div>
                <div className="investment-account-cash">
                  <span>Disponível hoje</span>
                  <Money currencyCode="BRL" value={item.cash_balance} tone="balance" />
                </div>
                <RecordMenu label={`Ações da conta ${item.name}`} items={menuOf(item)} />
              </div>
            </li>
          ))}
        </ul>
      )}
    </Drawer>
  );
}
