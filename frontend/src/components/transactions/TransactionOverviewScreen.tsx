import { useTranslation } from "../../i18n/context";
import { LinkOutlined, RedoOutlined, SwapOutlined } from "../ui/icons";
import { Alert, Button, Switch, Typography } from "../ui";
import { useQuery } from "@tanstack/react-query";
import { getTransactionTransfer } from "../../api/transfers";
import { queryKeys } from "../../api/queryKeys";
import { useMemo } from "react";

import type { Category, TransactionInclusionState, TransactionItem } from "../../api/contracts";
import { useTimezone } from "../../hooks/useTimezone";
import { useTransactionReconciliation } from "../../hooks/useTransactionReconciliation";
import { internalCategoryOriginLabel } from "../../presentation/categoryLabels";
import { formatDay } from "../../presentation/dates";
import { exclusionReason } from "../../presentation/transactionStatus";
import { formatBRL, formatMoney } from "../../presentation/money";
import {
  allowsInvestmentLink,
  categoryChoices,
  installmentLabel,
  investmentSplit,
  recurringCommitmentPrefill,
  remainderLabel,
  shortDateTime,
  suggestedCategory,
} from "../../presentation/transactionDetail";
import { CategoryField } from "../categories/CategoryField";
import { PanelNavRow, PanelSection } from "../shared/PanelStack";
import { TransactionAmount } from "./TransactionAmount";
import type { TransactionScreen } from "./TransactionPanel";
import type { WriteIssue } from "./useTransactionPanelWrites";

/**
 * The root screen: who the transaction is and the decisions people take on
 * almost every one (category, whether it counts). Everything else is one
 * tap away behind a row, never open by default.
 *
 * A failed write is shown here, next to the control that caused it — the
 * panel covers the list, so an error on the page would never be seen.
 */
export function TransactionOverviewScreen({
  item,
  categories,
  onCategory,
  categoryPending,
  categoryError = null,
  onInclusion,
  inclusionPending,
  inclusionError = null,
  deleteError,
  navigate,
}: {
  item: TransactionItem;
  categories: Category[];
  onCategory?: (id: string, categoryId: string) => void;
  categoryPending: boolean;
  categoryError?: WriteIssue | null;
  onInclusion?: (id: string, state: TransactionInclusionState) => void;
  inclusionPending: boolean;
  inclusionError?: WriteIssue | null;
  deleteError: string | null;
  navigate: (screen: TransactionScreen) => void;
}) {
  const { t, language } = useTranslation();
  const timezone = useTimezone();
  const ignored = item.inclusion.state === "ignored";
  const transfer = useQuery({ queryKey: queryKeys.transactionTransfer(item.id), queryFn: ({ signal }) => getTransactionTransfer(item.id, signal), retry: false });
  const linked = !!transfer.data || item.totals_eligibility.reason === "linked_transfer";
  const linkedPrincipal = linked && transfer.data?.fee?.transaction_id !== item.id;
  const otherMovement = transfer.data
    ? (transfer.data.source.transaction_id === item.id ? transfer.data.destination : transfer.data.source)
    : null;
  const transferHint = otherMovement
    ? `${otherMovement.account_name} · ${(otherMovement.amount === null ? t("transferAmountUnavailable") : formatMoney(otherMovement.amount.replace(/^-/, ""), otherMovement.currency_code))}`
    : transfer.isLoading ? t("loading") : transfer.isError ? t("queryUnavailable") : linked ? t("linkedTransfer") : t("noLink");
  const choices = useMemo(() => categoryChoices(categories, item, language), [categories, item, language]);
  const suggestion = suggestedCategory(item, choices);
  const split = investmentSplit(item);
  const prefill = recurringCommitmentPrefill(item, timezone);
  // The row only summarises; the reconcile screen owns the writes.
  const reconciliation = useTransactionReconciliation(item.id, !ignored);

  const where = item.card
    ? [item.card.number, installmentLabel(item.card, language)].filter(Boolean).join(" · ")
    : [item.account.name, item.account.institution].filter(Boolean).join(" · ");

  const reconciliationHint = ignored
    ? t("ignoredCannotReconcile")
    : reconciliation.isLoading
      ? t("loading")
      : reconciliation.isError
        ? t("queryUnavailable")
        : reconciliation.current
        ? `${reconciliation.current.commitment_name} · ${formatDay(reconciliation.current.occurrence_date, language)}`
        : reconciliation.options.length > 0
          ? t(reconciliation.options.length === 1 ? "oneNearbyOccurrence" : "manyNearbyOccurrences", { count: reconciliation.options.length })
          : t("noNearbyRecurrences");

  const investmentHint = split
    ? t("amountLinked", { amount: formatBRL(split.transferred) })
    : t("noLink");

  // Origin and "out of the totals" are facts about the line, so they read as
  // plain text beside the date rather than as coloured tags.
  const when = [
    shortDateTime(item.occurred_at, timezone, language),
    item.origin === "manual" ? "Manual" : null,
    item.source_provider === "file" ? t("file") : null,
    ignored ? t("excludedFromTotals") : null,
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <>
      <header className="transaction-identity">
        <strong className="transaction-identity-description">
          {item.description ?? t("descriptionNotProvided")}
        </strong>
        {where && <span className="transaction-identity-where">{where}</span>}
        <TransactionAmount item={item} size="hero" className="transaction-identity-amount" />
        <span className="transaction-identity-when">{when}</span>
        {split && (
          <span className="transaction-identity-split">
            {item.classification === "inflow" ? t("redemptionLower") : t("contributionLower")} {t("linkedLower")}:{" "}
            {formatBRL(split.transferred)} · {remainderLabel(item, split.remaining, language)}
          </span>
        )}
      </header>

      <PanelSection title={t("category")}>
        <CategoryField
          id="transaction-category"
          value={item.internal_category?.id ?? null}
          options={choices}
          suggested={suggestion}
          loading={categoryPending}
          disabled={!onCategory}
          onChange={(value) => onCategory?.(item.id, value)}
        />
        {item.internal_category && (
          <Typography.Text type="secondary" className="transaction-field-hint">
            {internalCategoryOriginLabel(item.internal_category, language)}
          </Typography.Text>
        )}
        {categoryError && <WriteFailure issue={categoryError} title={t("categorySaveError")} />}
        {item.internal_category && !item.internal_category.is_active && (
          <Alert
            type="warning"
            showIcon
            message={t("inactiveCategoryWarning")}
          />
        )}
      </PanelSection>

      <PanelSection>
        <div className="transaction-toggle-row">
          <label htmlFor="transaction-considered">{t("includeInTotals")}</label>
          <Switch
            id="transaction-considered"
            checked={!ignored}
            loading={inclusionPending}
            disabled={!onInclusion || linkedPrincipal}
            onChange={(checked) => onInclusion?.(item.id, checked ? "considered" : "ignored")}
          />
        </div>
        {linkedPrincipal && <Typography.Text type="secondary">{t("linkedTransferWarning")}</Typography.Text>}
        {ignored && (
          <Typography.Text type="secondary" className="transaction-field-hint">
            {t("ignoredTransactionHint")}
          </Typography.Text>
        )}
        {!ignored && !linkedPrincipal && !item.totals_eligibility.included && item.totals_eligibility.reason && (
          <Typography.Text type="secondary" className="transaction-field-hint">
            {exclusionReason(item.totals_eligibility.reason, language)}
          </Typography.Text>
        )}
        {inclusionError && <WriteFailure issue={inclusionError} title={t("decisionSaveError")} />}
      </PanelSection>

      <PanelSection title={t("actions")} className="panel-section-rows">
        <PanelNavRow icon={<SwapOutlined />} label={t("transferBetweenAccounts")} hint={transferHint} disabled={!linked && (ignored || item.classification === "unclassified")} onClick={() => navigate({ kind: "transfer" })} />
        {prefill && (
          <PanelNavRow
            icon={<RedoOutlined />}
            label={t("createRecurrence")}
            onClick={() => navigate({ kind: "recurrence" })}
          />
        )}
        <PanelNavRow
          icon={<SwapOutlined />}
          label={t("reconcileRecurrence")}
          hint={reconciliationHint}
          disabled={ignored}
          onClick={() => navigate({ kind: "reconcile" })}
        />
        {allowsInvestmentLink(item) && (
          <PanelNavRow
            icon={<LinkOutlined />}
            label={t("linkInvestment")}
            hint={investmentHint}
            onClick={() => navigate({ kind: "investment" })}
          />
        )}
      </PanelSection>

      <PanelSection title={t("moreInformation")} className="panel-section-rows">
        <PanelNavRow label={t("transactionDetails")} onClick={() => navigate({ kind: "details" })} />
        <PanelNavRow label={t("technicalInformation")} onClick={() => navigate({ kind: "technical" })} />
      </PanelSection>

      {item.origin === "manual" && split && (
        <Alert
          type="warning"
          showIcon
          message={t("investmentLinkedTransaction")}
          description={t("investmentLinkedWarning")}
        />
      )}
      {deleteError && <Alert type="error" showIcon message={deleteError} />}
    </>
  );
}

/** A failed write, in the panel and next to the control that caused it: what happened, and a retry. */
function WriteFailure({ issue, title }: { issue: WriteIssue; title: string }) {
  const { t } = useTranslation();
  return (
    <Alert
      type="error"
      showIcon
      message={title}
      description={
        <>
          <p>{issue.message}</p>
          <Button size="small" onClick={issue.retry}>
            {t("retry")}
          </Button>
        </>
      }
    />
  );
}
