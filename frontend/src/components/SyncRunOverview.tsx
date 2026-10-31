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
  const timezone = useTimezone();
  return (
    <Section title="Resumo da sincronização" trailing={<SyncStatusBadge status={run.status} />}>
      <div className="sync-run-overview-body">
        <dl className="sync-run-overview-meta">
          <div>
            <dt>Início</dt>
            <dd>
              <time dateTime={run.started_at}>{formatDate(run.started_at, timezone)}</time>
            </dd>
          </div>
          <div>
            <dt>Conclusão</dt>
            <dd>
              {run.finished_at === null ? (
                formatOptionalDate(null, timezone)
              ) : (
                <time dateTime={run.finished_at}>{formatOptionalDate(run.finished_at, timezone)}</time>
              )}
            </dd>
          </div>
        </dl>
        <SyncRunMetrics run={run} />
        <p className="sync-run-identifier">
          Identificador: <span>{run.id}</span>
        </p>
      </div>
    </Section>
  );
}
