import type { Column } from "./ui/Table";
import Table from "./ui/Table";
import { Link, useNavigate } from "react-router-dom";

import type { SyncRun } from "../api/contracts";
import { useTimezone } from "../hooks/useTimezone";
import { formatDate, formatLocalDay } from "../presentation/dates";
import { ResponsiveList } from "./layout";
import { SyncStatusBadge } from "./SyncStatusBadge";

const detailsPath = (run: SyncRun) => `/configuracoes/open-banking/sync-runs/${run.id}`;
const detailsLabel = (run: SyncRun, timezone: string) =>
  `Ver detalhes da sincronização iniciada em ${formatDate(run.started_at, timezone)}`;

/**
 * One lighter line for a run on a phone: when, and what it moved. Short on
 * purpose — a failed or partial run has a status tag beside the text, and a
 * line that wraps to three beside it reads as noise. Updates only when there
 * were some; the accounts count is in the run's detail.
 */
function runMeta(run: SyncRun, timezone: string): string {
  return [
    formatLocalDay(run.started_at, timezone),
    `${run.transactions_inserted} ${run.transactions_inserted === 1 ? "nova" : "novas"}`,
    run.transactions_updated > 0
      ? `${run.transactions_updated} ${run.transactions_updated === 1 ? "atualizada" : "atualizadas"}`
      : null,
  ]
    .filter(Boolean)
    .join(" · ");
}

/**
 * The sync history: a table from `md` up and, on a phone, rows — connection,
 * when and what it moved, with the status shown only when the run did not
 * simply complete — each one a link to the run's detail. In the table the row
 * opens the run too; the date is the link for the keyboard.
 */
export function SyncRunHistory({ runs }: { runs: SyncRun[] }) {
  const navigate = useNavigate();
  const timezone = useTimezone();
  const columns: Column<SyncRun>[] = [
    {
      title: "Início",
      dataIndex: "started_at",
      render: (_, run) => (
        <Link aria-label={detailsLabel(run, timezone)} to={detailsPath(run)}>
          <time dateTime={run.started_at}>{formatDate(run.started_at, timezone)}</time>
        </Link>
      ),
    },
    { title: "Conexão", dataIndex: "source_name" },
    {
      title: "Situação",
      dataIndex: "status",
      // The common case is plain text; only the runs that need a look get a tag.
      render: (_, run) =>
        run.status === "completed" ? "Concluída" : <SyncStatusBadge status={run.status} />,
    },
    { title: "Contas processadas", dataIndex: "accounts_processed", align: "right" },
    { title: "Transações incluídas", dataIndex: "transactions_inserted", align: "right" },
    { title: "Transações atualizadas", dataIndex: "transactions_updated", align: "right" },
  ];

  return (
    <ResponsiveList<SyncRun>
      label="Sincronizações recentes"
      items={runs}
      getKey={(run) => run.id}
      row={(run) => ({
        title: run.source_name,
        meta: runMeta(run, timezone),
        status: run.status === "completed" ? undefined : <SyncStatusBadge status={run.status} />,
        href: detailsPath(run),
        ariaLabel: detailsLabel(run, timezone),
      })}
      wide={
        <Table<SyncRun>
          aria-label="Sincronizações recentes"
          columns={columns}
          dataSource={runs}
          rowKey="id"
          pagination={false}
          onRow={(run) => ({ onClick: () => navigate(detailsPath(run)), style: { cursor: "pointer" } })}
        />
      }
    />
  );
}
