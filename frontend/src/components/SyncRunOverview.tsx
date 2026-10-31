import { useTranslation } from "../i18n/context";
import type { SyncRunDetail } from "../api/contracts";
import { useTimezone } from "../hooks/useTimezone";
import { formatDate, formatOptionalDate } from "../presentation/dates";
import { Section } from "./layout";
import { SyncRunMetrics } from "./SyncRunMetrics";
import { SyncStatusBadge } from "./SyncStatusBadge";

/**
 * The sync run detail page's opening block — status first, then when the run
 * started and finished and the volume it moved, with the run's raw
 * identifier last as quiet technical metadata. One flat section with
 * definition lists, not cards.
 */
export function SyncRunOverview({ run }: { run: SyncRunDetail }) {
  const { t, language } = useTranslation();
  const timezone = useTimezone();
  return (
    <Section title={t("syncOverview")} trailing={<SyncStatusBadge status={run.status} />}>
      <div className="sync-run-overview-body">
        <dl className="sync-run-overview-meta">
          <div>
            <dt>{t("start")}</dt>
            <dd>
              <time dateTime={run.started_at}>{formatDate(run.started_at, timezone, language)}</time>
            </dd>
          </div>
          <div>
            <dt>{t("completion")}</dt>
            <dd>
              {run.finished_at === null ? (
                formatOptionalDate(null, timezone, language)
              ) : (
                <time dateTime={run.finished_at}>{formatOptionalDate(run.finished_at, timezone, language)}</time>
              )}
            </dd>
          </div>
        </dl>
        <SyncRunMetrics run={run} />
        <p className="sync-run-identifier">
          {t("identifier")}: <span>{run.id}</span>
        </p>
      </div>
    </Section>
  );
}
