import { useTranslation } from "../../i18n/context";
import { Descriptions, Tag } from "../ui";

import type { TransactionItem } from "../../api/contracts";
import { internalCategoryOriginLabel } from "../../presentation/categoryLabels";
import { formatBRL, formatMoney, moneySourceLabel } from "../../presentation/money";
import { dateTime, installmentLabel, investmentSplit, remainderLabel } from "../../presentation/transactionDetail";
import { movementTypeLabel } from "../../presentation/transactionLabels";
import { exclusionReason, inclusionOriginLabel } from "../../presentation/transactionStatus";
import { PanelSection } from "../shared/PanelStack";

/** Where and when, plus the history behind the current decisions — read-only. */
export function TransactionDetailsScreen({ item }: { item: TransactionItem }) {
  const { t, language } = useTranslation();
  const split = investmentSplit(item);
  return (
    <>
      <PanelSection title={t("whenWhere")}>
        <Descriptions column={1} size="small" colon={false}>
          <Descriptions.Item label={t("dateAndTime")}>{dateTime(item.occurred_at, language)}</Descriptions.Item>
          <Descriptions.Item label={t("account")}>{item.account.name ?? t("accountNotProvided")}</Descriptions.Item>
          <Descriptions.Item label={t("institution")}>
            {item.account.institution ?? t("institutionNotProvided")}
          </Descriptions.Item>
          {item.card && (
            <Descriptions.Item label={t("card")}>
              {item.card.number}
              {installmentLabel(item.card, language) && (
                <Tag className="transaction-card-installment" color="blue">
                  {installmentLabel(item.card, language)}
                </Tag>
              )}
            </Descriptions.Item>
          )}
        </Descriptions>
      </PanelSection>

      <PanelSection title={t("decisions")}>
        <Descriptions column={1} size="small" colon={false}>
          <Descriptions.Item label={t("category")}>
            {item.internal_category
              ? `${item.internal_category.name} · ${internalCategoryOriginLabel(item.internal_category, language)}`
              : t("uncategorized")}
          </Descriptions.Item>
          <Descriptions.Item label={t("bankSuggestion")}>{item.source_category ?? t("notProvidedFeminine")}</Descriptions.Item>
          <Descriptions.Item label={t("inTotals")}>
            {item.inclusion.state === "ignored" ? t("no") : t("yes")}
            {item.inclusion.changed_at &&
              ` · ${inclusionOriginLabel(item.inclusion, language)} ${t("atDate")} ${dateTime(item.inclusion.changed_at, language)}`}
          </Descriptions.Item>
          {split && (
            <Descriptions.Item label={t("investment")}>
              {item.classification === "inflow" ? t("redemption") : t("contribution")} {t("linked")}: {formatBRL(split.transferred)} ·{" "}
              {remainderLabel(item, split.remaining, language)}
            </Descriptions.Item>
          )}
        </Descriptions>
      </PanelSection>
    </>
  );
}

/** The provider's raw view of the line, for when a figure needs to be traced back. */
export function TransactionTechnicalScreen({ item }: { item: TransactionItem }) {
  const { t, language } = useTranslation();
  const reason = item.totals_eligibility.reason;
  return (
    <PanelSection>
      <Descriptions column={1} size="small" colon={false}>
        <Descriptions.Item label={t("origin")}>{item.origin === "manual" ? "Manual" : item.source_provider === "file" ? t("importedFile") : t("automaticConnection")}</Descriptions.Item>
        <Descriptions.Item label={t("externalIdentifier")}>{item.external_id}</Descriptions.Item>
        <Descriptions.Item label={t("originalType")}>{movementTypeLabel(item.movement_type, language)}</Descriptions.Item>
        <Descriptions.Item label={t("originalAmount")}>
          {item.amount !== null && item.currency_code ? formatMoney(item.amount, item.currency_code) : t("notProvided")}
        </Descriptions.Item>
        <Descriptions.Item label={t("originalCurrency")}>{item.currency_code ?? t("notProvidedFeminine")}</Descriptions.Item>
        <Descriptions.Item label={t("originalProviderStatus")}>
          {item.provider_status ?? t("notProvided")}
        </Descriptions.Item>
        {item.effective_money && (
          <Descriptions.Item label={t("displayedAmountSource")}>
            {moneySourceLabel(item.effective_money.source, language)}
          </Descriptions.Item>
        )}
        <Descriptions.Item label={t("totalization")}>
          {item.totals_eligibility.included
            ? t("includedInTotals")
            : reason
              ? exclusionReason(reason, language)
              : t("notIncluded")}
        </Descriptions.Item>
      </Descriptions>
    </PanelSection>
  );
}
