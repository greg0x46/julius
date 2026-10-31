import { useTranslation } from "../../i18n/context";
import type { Account } from "../../api/contracts";
import { sumBRL } from "../../presentation/money";
import { SummaryStrip } from "../layout";
import { Money } from "../shared/Money";

function isBRL(account: Account): boolean {
  return account.currency_code === null || account.currency_code === "BRL";
}

// Totals cover BRL accounts only: mixing currencies into one figure would be
// wrong, and the same restriction already applies to the credit-card total on
// the home dashboard.
function sumAccounts(accounts: Account[], pick: (account: Account) => string | null): string {
  return sumBRL(accounts.filter(isBRL).map((account) => pick(account) ?? "0"));
}

/**
 * The page's hero figure — the balance across bank accounts — with what the
 * cards currently owe as its one secondary figure.
 *
 * The cards' available limit is deliberately not a summary figure: each card
 * row already says how much of its own limit is left, and a limit summed
 * across issuers is not a pool anyone can spend from. The total limit sits in
 * the cards section's header, next to the cards it actually limits.
 */
export function AccountsSummary({ accounts }: { accounts: Account[] }) {
  const { t } = useTranslation();
  const bank = accounts.filter((account) => account.account_type !== "CREDIT");
  const credit = accounts.filter((account) => account.account_type === "CREDIT");
  const hasForeignAccounts = bank.some((account) => !isBRL(account));

  return (
    <SummaryStrip
      label={t("accountBalance")}
      value={<Money value={sumAccounts(bank, (account) => account.balance)} tone="balance" size="hero" />}
      note={hasForeignAccounts ? t("brlAccountsOnly") : undefined}
      items={
        credit.length > 0
          ? [
              {
                label: t("currentCardBills"),
                value: <Money value={sumAccounts(credit, (account) => account.balance)} tone="balance" />,
              },
            ]
          : undefined
      }
    />
  );
}
