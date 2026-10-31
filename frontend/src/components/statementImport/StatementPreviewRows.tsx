import { useTranslation } from "../../i18n/context";
import type { MessageKey } from "../../i18n/messages";
import { Button, Radio } from "../ui";
import { useState } from "react";

import type { ImportConflict, ImportRow } from "../../api/statementImports";
import { useTimezone } from "../../hooks/useTimezone";
import { formatDate } from "../../presentation/dates";
import { formatMoney } from "../../presentation/money";
import { EmptyState } from "../layout";
import { StatusTag } from "../shared/StatusTag";

const pageSize = 20;

// A new row is the default and carries no tag. Rows needing a decision,
// already imported rows and invalid rows say so explicitly.
const statusTag = {
  duplicate: { tone: "neutral", label: "statusAlreadyImported" },
  ambiguous: { tone: "warning", label: "statusNeedsDecision" },
  invalid: { tone: "danger", label: "statusInvalid" },
} as const satisfies Record<string, { tone: string; label: MessageKey }>;

/**
 * One line of the file: description and where it came from on the left, the
 * amount and the running balance on the right, then its status and review
 * notes. Errors keep the error color; warnings stay plain but visible.
 */
function StatementPreviewRow({ row, conflict, filename, decision, onDecision }: {
  row: ImportRow;
  conflict?: ImportConflict;
  filename?: string;
  decision?: "import" | "ignore";
  onDecision?: (line: number, decision: "import" | "ignore") => void;
}) {
  const { t, language } = useTranslation();
  const timezone = useTimezone();
  const currency = row.currency ?? "BRL";
  const tag = row.status === "new" ? null : statusTag[row.status];
  const hasNotes = row.errors.length > 0 || row.warnings.length > 0;

  return (
    <li className={`statement-row is-${row.status}`}>
      <div className="statement-row-identity">
        <span className="statement-row-description">{row.description || "—"}</span>
        <span className="statement-row-meta">
          <span>{t("lineNumber", { number: row.line_number })}</span>
          <span>{row.occurred_at ? formatDate(row.occurred_at, timezone, language) : "—"}</span>
          {row.payment_method && <span>{row.payment_method}</span>}
        </span>
      </div>
      <div className="statement-row-figures">
        <span className="statement-row-amount">{row.amount ? formatMoney(row.amount, currency) : "—"}</span>
        {row.status !== "ambiguous" && <span className="statement-row-balance">{t("balanceAfterValue", { value: row.balance ? formatMoney(row.balance, currency) : "—" })}</span>}
      </div>
      {(tag !== null || hasNotes) && (
        <div className="statement-row-review">
          {tag !== null && <StatusTag tone={tag.tone}>{t(tag.label)}</StatusTag>}
          {hasNotes && (
            <ul className="statement-row-notes">
              {row.errors.map((value, index) => <li key={`e-${index}`} className="statement-row-error">{value}</li>)}
              {row.warnings.map((value, index) => <li key={`w-${index}`}>{value}</li>)}
            </ul>
          )}
        </div>
      )}
      {row.status === "ambiguous" && (
        <div className="statement-row-conflict">
          <span className="statement-row-conflict-label">{t("differentBalances")}</span>
          <div className="statement-conflict-comparison">
            <section aria-label={t("existingTransactionOfLine", { number: row.line_number })}>
              <strong>{t("existingTransaction")}</strong>
              {conflict ? <>
                {conflict.description !== row.description && <span>{conflict.description}</span>}
                {conflict.occurred_at !== row.occurred_at && <span>{formatDate(conflict.occurred_at, timezone, language)}</span>}
                {formatMoney(conflict.amount, conflict.currency) !== formatMoney(row.amount ?? "0", currency) && <span>{formatMoney(conflict.amount, conflict.currency)}</span>}
                {conflict.payment_method !== row.payment_method && <span>{conflict.payment_method}</span>}
                <dl><dt>{t("balanceAfter")}</dt><dd>{conflict.balance === null ? t("notReported") : formatMoney(conflict.balance, conflict.currency)}</dd></dl>
                {conflict.filename && <span className="statement-row-meta">{t("fileLabel", { name: conflict.filename })}</span>}
              </> : <span>{t("existingTransactionUnavailable")}</span>}
            </section>
            <section aria-label={t("fileLineOf", { number: row.line_number })}>
              <strong>{t("fileLine")}</strong>
              <dl><dt>{t("balanceAfter")}</dt><dd>{row.balance === null ? t("notReported") : formatMoney(row.balance, currency)}</dd></dl>
              {filename && <span className="statement-row-meta">{t("fileLabel", { name: filename })}</span>}
            </section>
          </div>
          <Radio.Group aria-label={t("lineDecision", { number: row.line_number })} value={decision}
            onChange={(event) => onDecision?.(row.line_number, event.target.value as "ignore" | "import")}>
            <Radio value="ignore">{t("keepExistingIgnore")}</Radio>
            <Radio value="import">{t("importAsAnother")}</Radio>
          </Radio.Group>
        </div>
      )}
    </li>
  );
}

/**
 * The preview's rows as a mobile-first list (a table would scroll sideways on
 * a phone), 20 to a page: a file can carry thousands of lines. The footer is
 * the same Exibindo/Anterior/Próxima one the transactions list uses.
 */
export function StatementPreviewRows({ rows, conflicts, filename, ambiguousDecisions, onAmbiguousDecision }: {
  rows: ImportRow[];
  conflicts?: Record<number, ImportConflict>;
  filename?: string;
  ambiguousDecisions?: Record<number, "import" | "ignore">;
  onAmbiguousDecision?: (line: number, decision: "import" | "ignore") => void;
}) {
  const { t, language } = useTranslation();
  const [page, setPage] = useState(1);

  if (rows.length === 0) return <EmptyState title={t("noRowsToShow")} />;

  const totalPages = Math.ceil(rows.length / pageSize);
  const current = Math.min(page, totalPages);
  const start = (current - 1) * pageSize;
  const visible = rows.slice(start, start + pageSize);

  return (
    <>
      <ul className="statement-row-list" aria-label={t("fileRows")}>
        {visible.map((row) => <StatementPreviewRow key={row.line_number} row={row} conflict={conflicts?.[row.line_number]} filename={filename}
          decision={ambiguousDecisions?.[row.line_number]} onDecision={onAmbiguousDecision} />)}
      </ul>
      {totalPages > 1 && (
        <footer className="statement-import-pagination">
          <span>
            {t("rowsDisplayed", { start: (start + 1).toLocaleString(language), end: (start + visible.length).toLocaleString(language), total: rows.length.toLocaleString(language) })}
          </span>
          <nav aria-label={t("rowsPagination")}>
            <Button disabled={current <= 1} onClick={() => setPage(current - 1)}>{t("previous")}</Button>
            <span>{t("pageOf", { page: current, total: totalPages })}</span>
            <Button disabled={current >= totalPages} onClick={() => setPage(current + 1)}>{t("next")}</Button>
          </nav>
        </footer>
      )}
    </>
  );
}
