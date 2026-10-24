import { Tooltip } from "../ui";

import type { Account } from "../../api/contracts";
import { formatOptionalDateTime, formatOptionalDay } from "../../presentation/dates";
import {
  accountMetaLine,
  creditUsagePercent,
  shortInstitutionName,
} from "../../presentation/accountLabels";
import { SummaryStrip, type SummaryStripItem } from "../layout";
import { AccountMoney } from "./AccountMoney";
import { ClosingDayField } from "./ClosingDayField";

/**
 * The account detail page's opening block — one summary for both a bank
 * account and a credit card, built on the shared `SummaryStrip`: the balance
 * (the invoice, for a card) as the hero, and — for a card only — the limit
 * figures and the closing/due dates as its definition strip. Identity and
 * freshness are two quiet lines under the hero ("Atualizado em" is meta, not a
 * figure). The account's name is the page's h1 (see AccountDetailPage), so it
 * is not repeated here. The legal institution name (when different from the
 * short one shown as the title) rides along as a tooltip on the identity line,
 * and the provider's own plumbing (MeuPluggy or otherwise) never surfaces
 * here — see Account.institution_name.
 */
export function AccountSummary({
  account,
  onSaveClosingDay,
  savingClosingDay,
}: {
  account: Account;
  onSaveClosingDay: (closingDay: number | null) => Promise<unknown>;
  savingClosingDay: boolean;
}) {
  const isCredit = account.account_type === "CREDIT";
  // The heading is the account's own name, so the institution lives on this line.
  const legalName =
    account.institution_name !== null && account.institution_name !== shortInstitutionName(account.institution_name)
      ? account.institution_name
      : undefined;
  const identityLine = accountMetaLine(account);

  const items: SummaryStripItem[] = isCredit
    ? [
        {
          label: "Limite disponível",
          value: (
            <AccountMoney
              value={account.available_credit_limit}
              currencyCode={account.currency_code}
              tone="balance"
            />
          ),
        },
        {
          label: "Limite total",
          value: <AccountMoney value={account.credit_limit} currencyCode={account.currency_code} tone="balance" />,
        },
        // Its own row, not a second line under the available limit: every
        // definition row is one label and one value, so the column stays even.
        ...(account.credit_usage_ratio !== null
          ? [{ label: "Limite usado", value: creditUsagePercent(account.credit_usage_ratio) }]
          : []),
        { label: "Vence", value: formatOptionalDay(account.balance_due_date) },
        {
          label: "Fechamento",
          value: <ClosingDayField account={account} onSave={onSaveClosingDay} saving={savingClosingDay} />,
        },
      ]
    : account.origin === "manual" ? [{ label: "Saldo inicial", value: <AccountMoney value={account.opening_balance} currencyCode={account.currency_code} tone="balance" /> }] : [];

  return (
    <SummaryStrip
      className="account-summary"
      label={isCredit ? "Fatura atual" : "Saldo disponível"}
      value={<AccountMoney value={account.balance} currencyCode={account.currency_code} tone="balance" size="hero" />}
      note={
        <>
          {identityLine !== "" && (
            <Tooltip title={legalName}>
              <span className="account-summary-meta">{identityLine}</span>
            </Tooltip>
          )}
          <span className="account-summary-meta">
            Atualizado em {formatOptionalDateTime(account.provider_updated_at ?? account.updated_at)}
          </span>
        </>
      }
      items={items}
    />
  );
}
