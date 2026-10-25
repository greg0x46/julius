import { Skeleton } from "../ui";

import type { Account } from "../../api/contracts";
import { accountDisplayName, accountMetaLine } from "../../presentation/accountLabels";
import { EmptyState, ListRow, Section } from "../layout";
import { AccountMoney } from "./AccountMoney";

/**
 * Bank accounts as rows: name (500) and the balance (600, right, on the
 * name's line) on one line, type, number and institution as the one quiet line
 * under it. The whole row opens the account. See CreditCardList for the cards
 * section.
 *
 * `failed` is a load that went wrong: the page then shows only its retry
 * Alert, so this section says nothing at all instead of "no accounts yet".
 */
export function BankAccountList({
  accounts,
  isLoading,
  failed = false,
  onOpen,
}: {
  accounts: Account[];
  isLoading: boolean;
  failed?: boolean;
  onOpen: (account: Account) => void;
}) {
  if (failed) return null;
  return (
    <Section title="Contas bancárias">
      {isLoading ? (
        <div role="status" aria-label="Carregando contas bancárias">
          <Skeleton active paragraph={{ rows: 2 }} title={false} />
        </div>
      ) : accounts.length === 0 ? (
        <EmptyState
          title="Nenhuma conta bancária"
          hint="Crie uma conta manual, conecte um banco em Configurações ou importe um extrato."
        />
      ) : (
        <ul className="list-rows" aria-label="Contas bancárias">
          {accounts.map((account) => (
            <ListRow
              key={account.id}
              className="account-row"
              title={accountDisplayName(account)}
              meta={accountMetaLine(account) || undefined}
              trailing={
                <AccountMoney value={account.balance} currencyCode={account.currency_code} tone="balance" />
              }
              truncate
              onClick={() => onOpen(account)}
              ariaLabel={`Abrir ${accountDisplayName(account)}`}
            />
          ))}
        </ul>
      )}
    </Section>
  );
}
