import { Alert, Skeleton } from "../ui";

import type { AccountBill } from "../../api/contracts";
import { formatOptionalDay } from "../../presentation/dates";
import { formatAccountMoney } from "../../presentation/accountLabels";
import { EmptyState, ListRow, Section } from "../layout";
import { AccountMoney } from "./AccountMoney";

/** Closed invoices as flat rows: when it closed on the left (due date and the
 *  minimum payment as its one quiet line), the total on the right. Rows are
 *  plain, not tappable — there is no bill detail to open. */
export function AccountBillsTable({
  bills,
  isLoading,
  error,
}: {
  bills: AccountBill[];
  isLoading: boolean;
  error: unknown;
}) {
  return (
    <Section title="Faturas fechadas">
      <p className="accounts-section-note">
        A instituição disponibiliza apenas faturas já fechadas — a fatura em aberto aparece no saldo do cartão.
      </p>
      {error !== null && error !== undefined && (
        <Alert type="error" showIcon message="Não foi possível carregar as faturas desta conta." />
      )}
      {isLoading ? (
        <div role="status" aria-label="Carregando faturas fechadas">
          <Skeleton active paragraph={{ rows: 2 }} title={false} />
        </div>
      ) : bills.length === 0 ? (
        <EmptyState
          title="Nenhuma fatura fechada"
          hint="As faturas fechadas aparecem aqui quando a instituição as informa."
        />
      ) : (
        <ul className="list-rows" aria-label="Faturas fechadas">
          {bills.map((bill) => (
            <ListRow
              key={bill.id}
              title={`Fechou em ${formatOptionalDay(bill.closing_date)}`}
              meta={`Vence em ${formatOptionalDay(bill.due_date)} · Mín. ${formatAccountMoney(bill.minimum_payment_amount, bill.currency_code)}`}
              trailing={<AccountMoney value={bill.total_amount} currencyCode={bill.currency_code} tone="neutral" />}
            />
          ))}
        </ul>
      )}
    </Section>
  );
}
