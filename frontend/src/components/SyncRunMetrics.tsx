import { useTranslation } from "../i18n/context";
type Metrics = {
  accounts_processed: number;
  transactions_inserted: number;
  transactions_updated: number;
};

/**
 * What a run moved, as a flat definition strip (label small, figure 500)
 * instead of three statistic cards — the same shape the summary strips use.
 */
export function SyncRunMetrics({ run }: { run: Metrics }) {
  const { t } = useTranslation();
  const items = [
    { label: t("processedAccounts"), value: run.accounts_processed },
    { label: t("insertedTransactions"), value: run.transactions_inserted },
    { label: t("updatedTransactions"), value: run.transactions_updated },
  ];
  return (
    <dl className="sync-run-metrics">
      {items.map((item) => (
        <div key={item.label}>
          <dt>{item.label}</dt>
          <dd>{item.value}</dd>
        </div>
      ))}
    </dl>
  );
}
