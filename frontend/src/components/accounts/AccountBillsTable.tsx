import { useTranslation } from "../../i18n/context";
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
  const { t, language } = useTranslation();
  const failed = error !== null && error !== undefined;
  return (
    <Section title={t("closedBills")}>
      <p className="accounts-section-note">
        {t("closedBillsNote")}
      </p>
      {failed && (
        <Alert type="error" showIcon message={t("billsLoadError")} />
      )}
      {isLoading ? (
        <div role="status" aria-label={t("loadingClosedBills")}>
          <Skeleton active paragraph={{ rows: 2 }} title={false} />
        </div>
      ) : bills.length === 0 ? (
        failed ? null : (
          <EmptyState
            title={t("noClosedBills")}
            hint={t("noClosedBillsHint")}
          />
        )
      ) : (
        <ul className="list-rows" aria-label={t("closedBills")}>
          {bills.map((bill) => (
            <ListRow
              key={bill.id}
              title={bill.closing_date === null ? t("closingNotProvided") : t("closedOn", { date: formatOptionalDay(bill.closing_date, language) })}
              meta={t("dueOnMin", { date: formatOptionalDay(bill.due_date, language), amount: formatAccountMoney(bill.minimum_payment_amount, bill.currency_code) })}
              trailing={<AccountMoney value={bill.total_amount} currencyCode={bill.currency_code} tone="neutral" />}
            />
          ))}
        </ul>
      )}
    </Section>
  );
}
