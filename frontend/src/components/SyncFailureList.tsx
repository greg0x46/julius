import { useTranslation } from "../i18n/context";
import type { SyncFailure } from "../api/contracts";
import { useTimezone } from "../hooks/useTimezone";
import { formatDate } from "../presentation/dates";
import { getFailureStageLabel } from "../presentation/syncStatus";
import { Section } from "./layout";

/**
 * Every failure the run recorded, as flat rows: the failing stage (an error,
 * so in the error colour), its message, and the when and which account or
 * transaction as quiet meta lines. No Alert per failure — a list of errors
 * does not need a list of red boxes.
 */
export function SyncFailureList({ failures }: { failures: SyncFailure[] }) {
  const { t, language } = useTranslation();
  const timezone = useTimezone();
  if (failures.length === 0) {
    return null;
  }
  return (
    <Section title={t("recordedFailures")}>
      <ul className="sync-failure-list" aria-label={t("recordedFailures")}>
        {failures.map((failure, index) => (
          <li key={`${failure.code}-${failure.occurred_at}-${index}`} className="sync-failure-row">
            <span className="sync-failure-stage">{getFailureStageLabel(failure.stage, language)}</span>
            <span>{failure.message}</span>
            <span className="sync-failure-meta">
              {t("occurredAt")} <time dateTime={failure.occurred_at}>{formatDate(failure.occurred_at, timezone, language)}</time>
            </span>
            {failure.external_account_id !== null && (
              <span className="sync-failure-meta">{t("relatedAccount")}: {failure.external_account_id}</span>
            )}
            {failure.external_transaction_id !== null && (
              <span className="sync-failure-meta">{t("relatedTransaction")}: {failure.external_transaction_id}</span>
            )}
          </li>
        ))}
      </ul>
    </Section>
  );
}
