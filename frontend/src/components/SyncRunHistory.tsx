import { useTranslation } from "../i18n/context";
import { translate, type Language } from "../i18n/messages";
import type { Column } from "./ui/Table";
import Table from "./ui/Table";
import { Link, useNavigate } from "react-router-dom";

import type { SyncRun } from "../api/contracts";
import { formatDate, formatLocalDay } from "../presentation/dates";
import { ResponsiveList } from "./layout";
import { SyncStatusBadge } from "./SyncStatusBadge";

const detailsPath = (run: SyncRun) => `/configuracoes/open-banking/sync-runs/${run.id}`;
const detailsLabel = (run: SyncRun, language: Language) => translate(language, "syncDetailsLabel").replace("{date}", formatDate(run.started_at, language));

/**
 * One lighter line for a run on a phone: when, and what it moved. Short on
 * purpose — a failed or partial run has a status tag beside the text, and a
 * line that wraps to three beside it reads as noise. Updates only when there
 * were some; the accounts count is in the run's detail.
 */
function runMeta(run: SyncRun, language: Language): string {
  return [
    formatLocalDay(run.started_at, language),
    translate(language, run.transactions_inserted === 1 ? "syncNewOne" : "syncNewMany").replace("{count}", String(run.transactions_inserted)),
    run.transactions_updated > 0
      ? translate(language, run.transactions_updated === 1 ? "syncUpdatedOne" : "syncUpdatedMany").replace("{count}", String(run.transactions_updated))
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
  const { t, language } = useTranslation();
  const navigate = useNavigate();
  const columns: Column<SyncRun>[] = [
    {
      title: t("start"),
      dataIndex: "started_at",
      render: (_, run) => (
        <Link aria-label={detailsLabel(run, language)} to={detailsPath(run)}>
          <time dateTime={run.started_at}>{formatDate(run.started_at, language)}</time>
        </Link>
      ),
    },
    { title: t("connection"), dataIndex: "source_name" },
    {
      title: t("status"),
      dataIndex: "status",
      // The common case is plain text; only the runs that need a look get a tag.
      render: (_, run) =>
        run.status === "completed" ? t("syncCompleted") : <SyncStatusBadge status={run.status} />,
    },
    { title: t("processedAccounts"), dataIndex: "accounts_processed", align: "right" },
    { title: t("insertedTransactions"), dataIndex: "transactions_inserted", align: "right" },
    { title: t("updatedTransactions"), dataIndex: "transactions_updated", align: "right" },
  ];

  return (
    <ResponsiveList<SyncRun>
      label={t("recentSyncs")}
      items={runs}
      getKey={(run) => run.id}
      row={(run) => ({
        title: run.source_name,
        meta: runMeta(run, language),
        status: run.status === "completed" ? undefined : <SyncStatusBadge status={run.status} />,
        href: detailsPath(run),
        ariaLabel: detailsLabel(run, language),
      })}
      wide={
        <Table<SyncRun>
          aria-label={t("recentSyncs")}
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
