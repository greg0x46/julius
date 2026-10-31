import { Skeleton } from "../ui";

import type { Account } from "../../api/contracts";
import { useTimezone } from "../../hooks/useTimezone";
import { formatOptionalDay } from "../../presentation/dates";
import { zonedToday } from "../../presentation/timezone";
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
function dueDayLabel(value: string | null, currentYear: string): string | null {
  if (value === null) return null;
  const full = formatOptionalDay(value);
  return full.endsWith(`/${currentYear}`) ? full.slice(0, 5) : full;
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
  const timezone = useTimezone();
  if (failed) return null;
  const currentYear = zonedToday(timezone).slice(0, 4);
  const hasForeignCards = accounts.some((account) => !isBRL(account));
  const hasBRLCards = accounts.some(isBRL);
  return (
    <Section
      title="Cartões de crédito"
      trailing={
        !isLoading && hasBRLCards ? (
          <>
            {hasForeignCards ? "Limite total em reais" : "Limite total"}{" "}
            {formatAccountMoney(totalLimit(accounts), "BRL")}
          </>
        ) : undefined
      }
    >
      {isLoading ? (
        <div role="status" aria-label="Carregando cartões de crédito">
          <Skeleton active paragraph={{ rows: 2 }} title={false} />
        </div>
      ) : accounts.length === 0 ? (
        <EmptyState
          title="Nenhum cartão de crédito"
          hint="Os cartões aparecem aqui quando você conecta um banco em Configurações."
        />
      ) : (
        <ul className="list-rows" aria-label="Cartões de crédito">
          {accounts.map((account) => {
            const due = dueDayLabel(account.balance_due_date, currentYear);
            const dates = [
              due !== null ? `Vence ${due}` : null,
              account.closing_day !== null ? `Fecha dia ${account.closing_day}` : null,
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
                    <span className="card-row-name">{accountDisplayName(account)}</span>
                    <AccountMoney
                      value={account.balance}
                      currencyCode={account.currency_code}
                      tone="balance"
                      size="row"
                    />
                  </span>
                  <span className="card-row-line card-row-meta">
                    <span title={closingDaySourceHint(account.closing_day_source)}>{dates.join(" · ")}</span>
                    <span>fatura atual</span>
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
