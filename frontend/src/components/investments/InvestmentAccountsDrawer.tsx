import { useTranslation } from "../../i18n/context";
import { ArrowLeftOutlined } from "../ui/icons";
import { Button, Drawer } from "../ui";

import type { InvestmentAccount, InvestmentOperation, InvestmentPosition } from "../../api/contracts";
import { investmentAccountKindName } from "../../presentation/investmentWorkspaceLabels";
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
  const { t, language } = useTranslation();
  const compact = useCompactScreen();
  const account = accounts.find((candidate) => candidate.id === scope?.accountId);
  const position = positions.find((candidate) => candidate.id === scope?.positionId);

  const title = position?.name ?? account?.name ?? t("investmentAccounts");
  const shown = operations.filter((operation) =>
    position ? operation.position_id === position.id : operation.account_id === account?.id,
  );

  return (
    <Drawer
      open={scope !== null && !hidden}
      onClose={onClose}
      width={compact ? "100%" : 560}
      destroyOnHidden
      push={false}
      title={
        <span className="investment-drawer-title">
          {account && (
            <Button
              type="text"
              icon={<ArrowLeftOutlined aria-hidden="true" />}
              aria-label={t("backToAccountsLabel")}
              onClick={() => onScope({ accountId: null, positionId: null })}
            />
          )}
          {title}
        </span>
      }
      extra={account ? <RecordMenu label={t("accountActionsNamed", { name: account.name })} items={menuOf(account)} /> : undefined}
    >
      {account ? (
        <>
          <div className="investment-account-cash">
            <span>
              {t("availableTodayAccount", { kind: investmentAccountKindName(account.kind, language).toLocaleLowerCase(language) })}
              {!account.active && t("inactiveSuffix")}
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
          title={t("noInvestmentAccounts")}
          hint={t("noInvestmentAccountsHint")}
          action={<Button onClick={onNewAccount}>{t("newInvestmentAccount")}</Button>}
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
                    {t("accountKindNamed", { kind: investmentAccountKindName(item.kind, language).toLocaleLowerCase(language) })}
                    {!item.active && t("inactiveSuffix")}
                  </span>
                </div>
                <div className="investment-account-cash">
                  <span>{t("availableToday")}</span>
                  <Money currencyCode="BRL" value={item.cash_balance} tone="balance" />
                </div>
                <RecordMenu label={t("accountActionsNamed", { name: item.name })} items={menuOf(item)} />
              </div>
            </li>
          ))}
        </ul>
      )}
    </Drawer>
  );
}
