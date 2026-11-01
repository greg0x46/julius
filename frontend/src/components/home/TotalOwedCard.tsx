import { useTranslation } from "../../i18n/context";
import { Skeleton } from "../ui";

import { usePayableTotalOwed } from "../../hooks/usePayableTotalOwed";
import { isZeroDecimal } from "../../presentation/decimal";
import { problemMessage } from "../../presentation/reporting";
import { UnavailableState } from "../AsyncState";
import { Money } from "../shared/Money";
import { HomeTile } from "./HomeTile";

const route = "/pendencias";

/**
 * What is still owed, as a link to Pendências: open debts plus the card bills
 * still accruing, as `/api/payables/total-owed` sums them. That endpoint is
 * BRL by construction (it declares `currency_code`), so the figure is shown in
 * the currency it names, not the reference currency.
 */
export function TotalOwedCard() {
  const { t } = useTranslation();
  const title = t("totalDebt");
  const owed = usePayableTotalOwed();

  if (owed.isLoading) {
    return (
      <HomeTile label={title}>
        <div role="status" aria-label={t("loadingTotalDebt")}>
          <Skeleton active title={false} paragraph={{ rows: 2 }} />
        </div>
      </HomeTile>
    );
  }
  if (owed.error || !owed.total) {
    return (
      <HomeTile label={title}>
        <UnavailableState onRetry={() => owed.refetch()}>
          {problemMessage(owed.error) ?? t("totalDebtError")}
        </UnavailableState>
      </HomeTile>
    );
  }

  const total = owed.total;
  return (
    <HomeTile label={title} to={route}>
      <span className="home-tile-line">
        <span className="home-tile-figure">
          <Money currencyCode={total.currency_code} value={total.total_owed} tone="neutral" />
        </span>
        <span className="home-tile-note">
          {isZeroDecimal(total.total_owed) ? (
            t("noOpenDebt")
          ) : (
            <>
              {t("payables")} <Money currencyCode={total.currency_code} value={total.remaining_debts_total} tone="neutral" />{" "}
              {t("openBills")}{" "}
              <Money currencyCode={total.currency_code} value={total.future_installments_total} tone="neutral" />
            </>
          )}
        </span>
      </span>
    </HomeTile>
  );
}
