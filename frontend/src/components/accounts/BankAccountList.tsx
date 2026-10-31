import { useTranslation } from "../../i18n/context";
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
  const { t, language } = useTranslation();
  if (failed) return null;
  return (
    <Section title={t("bankAccounts")}>
      {isLoading ? (
        <div role="status" aria-label={t("loadingBankAccounts")}>
          <Skeleton active paragraph={{ rows: 2 }} title={false} />
        </div>
      ) : accounts.length === 0 ? (
        <EmptyState
          title={t("noBankAccounts")}
          hint={t("noBankAccountsHint")}
        />
      ) : (
        <ul className="list-rows" aria-label={t("bankAccounts")}>
          {accounts.map((account) => (
            <ListRow
              key={account.id}
              className="account-row"
              title={accountDisplayName(account, language)}
              meta={accountMetaLine(account, language) || undefined}
              trailing={
                <AccountMoney value={account.balance} currencyCode={account.currency_code} tone="balance" />
              }
              truncate
              onClick={() => onOpen(account)}
              ariaLabel={`${t("openItem")} ${accountDisplayName(account, language)}`}
            />
          ))}
        </ul>
      )}
    </Section>
  );
}
