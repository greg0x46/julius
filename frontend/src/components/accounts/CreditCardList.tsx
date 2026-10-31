import { dateFormatter } from "../../i18n/dateFormats";
import { useTranslation } from "../../i18n/context";
import { Skeleton } from "../ui";

import type { Account } from "../../api/contracts";
import { formatOptionalDay } from "../../presentation/dates";
import {
  accountDisplayName,
  closingDaySourceHint,
  formatAccountMoney,
} from "../../presentation/accountLabels";
import { sumBRL } from "../../presentation/money";
import { EmptyState, Section } from "../layout";
import { AccountMoney } from "./AccountMoney";
import { CreditUsageMeter } from "./CreditUsageMeter";

function isBRL(account: Account): boolean {
  return account.currency_code === null || account.currency_code === "BRL";
}

// Limits of different currencies are not one number: only the cards in reais
// are summed, the same rule the page's summary strip applies to balances.
function totalLimit(accounts: Account[]): string {
  return sumBRL(accounts.filter(isBRL).map((account) => account.credit_limit ?? "0"));
}

// Due dates are near-future calendar days, so "10/10" is enough — the year is
// only spelled out when it is not the current one.
function dueDayLabel(value: string | null, language: "pt-BR" | "en"): string | null {
  if (value === null) return null;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return formatOptionalDay(value, language);
  const year = date.getUTCFullYear() === new Date().getFullYear() ? {} : { year: "numeric" as const };
  return dateFormatter(language, { day: "2-digit", month: "2-digit", ...year, timeZone: "UTC" }).format(date);
}

/**
 * Credit cards as three-line rows, the whole row opening the card:
 *   name ........................ fatura atual
 *   Vence 10/10 · Fecha dia 3 ... "fatura atual"
 *   [thin meter]
 *   R$ 8.157,44 disponível · 32% usado
 * The total limit lives in the section header, not on every row.
 */
export function CreditCardList({
  accounts,
  isLoading,
  failed = false,
  onOpen,
}: {
  accounts: Account[];
  isLoading: boolean;
  /** A load that went wrong: the page shows only its retry Alert, so this section stays out. */
  failed?: boolean;
  onOpen: (account: Account) => void;
}) {
  const { t, language } = useTranslation();
  if (failed) return null;
  const hasForeignCards = accounts.some((account) => !isBRL(account));
  const hasBRLCards = accounts.some(isBRL);
  return (
    <Section
      title={t("creditCards")}
      trailing={
        !isLoading && hasBRLCards ? (
          <>
            {hasForeignCards ? t("totalLimitBRL") : t("totalLimit")}{" "}
            {formatAccountMoney(totalLimit(accounts), "BRL")}
          </>
        ) : undefined
      }
    >
      {isLoading ? (
        <div role="status" aria-label={t("loadingCreditCards")}>
          <Skeleton active paragraph={{ rows: 2 }} title={false} />
        </div>
      ) : accounts.length === 0 ? (
        <EmptyState
          title={t("noCreditCards")}
          hint={t("noCreditCardsHint")}
        />
      ) : (
        <ul className="list-rows" aria-label={t("creditCards")}>
          {accounts.map((account) => {
            const due = dueDayLabel(account.balance_due_date, language);
            const dates = [
              due !== null ? t("cardDue").replace("{date}", due) : null,
              account.closing_day !== null ? t("cardCloses").replace("{day}", String(account.closing_day)) : null,
            ].filter((part): part is string => part !== null);
            const available =
              account.available_credit_limit !== null
                ? formatAccountMoney(account.available_credit_limit, account.currency_code)
                : undefined;
            return (
              <li key={account.id} className="list-row-item">
                {/* No aria-label: the button's own text (name, invoice, dates,
                    usage) is the most useful name a screen reader can get. */}
                <button type="button" className="card-row" onClick={() => onOpen(account)}>
                  <span className="card-row-line">
                    <span className="card-row-name">{accountDisplayName(account, language)}</span>
                    <AccountMoney
                      value={account.balance}
                      currencyCode={account.currency_code}
                      tone="balance"
                      size="row"
                    />
                  </span>
                  <span className="card-row-line card-row-meta">
                    <span title={closingDaySourceHint(account.closing_day_source, language)}>{dates.join(" · ")}</span>
                    <span>{t("currentBill")}</span>
                  </span>
                  <CreditUsageMeter ratio={account.credit_usage_ratio} available={available} />
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </Section>
  );
}
