import { useTranslation } from "../../i18n/context";
import { Alert, Button, Popconfirm, Skeleton, Typography } from "../ui";
import { useState } from "react";

import type { ReconciliationOption, TransactionItem } from "../../api/contracts";
import { useTransactionReconciliation } from "../../hooks/useTransactionReconciliation";
import { errorMessage } from "../../presentation/errors";
import { formatDay } from "../../presentation/dates";
import { formatBRL } from "../../presentation/money";
import { TransactionAmount } from "./TransactionAmount";
import { TransactionPicker } from "../shared/TransactionPicker";
import { PanelDisclosure, PanelFooter, PanelSection } from "../shared/PanelStack";

// The option's value has to carry both halves of an occurrence's address —
// the commitment and the calendar day — because occurrences have no id.
function optionValue(option: ReconciliationOption): string {
  return `${option.commitment_id}|${option.occurrence_date}`;
}


/**
 * Reconciliation seen from a transaction: what recurring occurrence it
 * settles, and which nearby occurrences it could settle instead.
 *
 * Only occurrences flowing the same direction as the transaction are offered
 * — an outflow can never settle an income commitment — so an empty list
 * usually means there is simply no matching commitment near this date.
 *
 * The writes are the same occurrence-scoped endpoints the Recorrências page
 * calls; this screen must not grow a second set of rules.
 */
export function TransactionReconcileScreen({
  item,
  onDone,
}: {
  item: TransactionItem;
  /** Called after a successful write so the stack can pop back. */
  onDone: () => void;
}) {
  const { t, language } = useTranslation();
  const reconciliation = useTransactionReconciliation(item.id);
  const [selected, setSelected] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const run = async (action: () => Promise<unknown>, fallback: string) => {
    setError(null);
    try {
      await action();
      onDone();
    } catch (caught) {
      setError(errorMessage(caught, fallback));
    }
  };

  const reconcile = () => {
    if (selected === null) {
      setError(t("selectOccurrenceToReconcile"));
      return;
    }
    const [commitmentId, occurrenceDate] = selected.split("|") as [string, string];
    void run(
      () => reconciliation.reconcile({ commitmentId, occurrenceDate }),
      t("reconcileTransactionError"),
    );
  };

  const current = reconciliation.current;

  return (
    <>
      <header className="transaction-screen-summary">
        <TransactionAmount item={item} />
        <span>{item.description ?? t("descriptionNotProvided")}</span>
      </header>

      {error && (
        <Alert type="error" showIcon closable onClose={() => setError(null)} message={error} />
      )}

      {reconciliation.isLoading ? (
        <div role="status" aria-label={t("loadingReconciliation")}>
          <Skeleton active paragraph={{ rows: 2 }} title={{ width: "40%" }} />
        </div>
      ) : reconciliation.isError ? (
        // A failed lookup is not "no recurrence nearby": say so, and offer the retry.
        <Alert
          type="error"
          showIcon
          message={t("recurrencesLookupError")}
          description={<Button onClick={reconciliation.refetch}>{t("retry")}</Button>}
        />
      ) : current !== null ? (
        <PanelSection title={t("reconciledWith")}>
          <div className="transaction-reconciled">
            <strong>{current.commitment_name}</strong>
            <span>
              {formatDay(current.occurrence_date, language)} · {formatBRL(current.expected_amount)}
            </span>
            <Typography.Text type="secondary">
              {t(current.origin === "rule" ? "reconciliationAutomatic" : "reconciliationManual")}
            </Typography.Text>
          </div>
          <PanelFooter>
            <Popconfirm
              title={t("unreconcile")}
              description={t("unreconcileHint")}
              onConfirm={() =>
                run(
                  () =>
                    reconciliation.detach({
                      commitmentId: current.commitment_id,
                      occurrenceDate: current.occurrence_date,
                    }),
                  t("unreconcileError"),
                )
              }
              okText={t("unreconcile")}
              cancelText={t("cancel")}
            >
              <Button danger block loading={reconciliation.isWriting}>
                {t("unreconcile")}
              </Button>
            </Popconfirm>
          </PanelFooter>
        </PanelSection>
      ) : (
        <>
          <PanelSection title={t("occurrence")}>
            <TransactionPicker
              id="transaction-reconcile-occurrence"
              label={t("reconcileWithRecurrence")}
              value={selected}
              placeholder={t("selectOccurrence")}
              disabled={reconciliation.options.length === 0}
              emptyText={t("noCompatibleOccurrence")}
              transactions={reconciliation.options.map((option) => ({
                id: optionValue(option),
                description: option.commitment_name,
                amount: option.expected_amount,
                direction: option.kind === "expense" ? "outflow" : "inflow",
                date: option.occurrence_date,
                category: t(option.kind === "expense" ? "recurringExpense" : "recurringIncome"),
              }))}
              onSelect={setSelected}
            />
            {reconciliation.options.length === 0 && (
              <Typography.Text type="secondary" className="transaction-field-hint">
                {t("noCompatibleOccurrenceHint")}
              </Typography.Text>
            )}
            <PanelDisclosure summary={t("occurrenceProjectionRemoved")}>
              {t("reconciliationExplanation")}
            </PanelDisclosure>
          </PanelSection>
          <PanelFooter>
            <Button
              type="primary"
              block
              loading={reconciliation.isWriting}
              disabled={reconciliation.options.length === 0}
              onClick={reconcile}
            >
              {t("reconcile")}
            </Button>
          </PanelFooter>
        </>
      )}
    </>
  );
}
