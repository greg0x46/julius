import { Button, Radio } from "antd";
import { useState } from "react";

import type { ImportConflict, ImportRow } from "../../api/statementImports";
import { formatDate } from "../../presentation/dates";
import { formatMoney } from "../../presentation/money";
import { EmptyState } from "../layout";
import { StatusTag } from "../shared/StatusTag";

const pageSize = 20;

// A new row is the default and carries no tag. Rows needing a decision,
// already imported rows and invalid rows say so explicitly.
const statusTag = {
  duplicate: { tone: "neutral", label: "Já importada" },
  ambiguous: { tone: "warning", label: "Precisa de decisão" },
  invalid: { tone: "danger", label: "Inválida" },
} as const;

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
  const currency = row.currency ?? "BRL";
  const tag = row.status === "new" ? null : statusTag[row.status];
  const hasNotes = row.errors.length > 0 || row.warnings.length > 0;

  return (
    <li className={`statement-row is-${row.status}`}>
      <div className="statement-row-identity">
        <span className="statement-row-description">{row.description || "—"}</span>
        <span className="statement-row-meta">
          <span>Linha {row.line_number}</span>
          <span>{row.occurred_at ? formatDate(row.occurred_at) : "—"}</span>
          {row.payment_method && <span>{row.payment_method}</span>}
        </span>
      </div>
      <div className="statement-row-figures">
        <span className="statement-row-amount">{row.amount ? formatMoney(row.amount, currency) : "—"}</span>
        {row.status !== "ambiguous" && <span className="statement-row-balance">Saldo após {row.balance ? formatMoney(row.balance, currency) : "—"}</span>}
      </div>
      {(tag !== null || hasNotes) && (
        <div className="statement-row-review">
          {tag !== null && <StatusTag tone={tag.tone}>{tag.label}</StatusTag>}
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
          <span className="statement-row-conflict-label">Saldos diferentes</span>
          <div className="statement-conflict-comparison">
            <section aria-label={`Transação existente da linha ${row.line_number}`}>
              <strong>Transação existente</strong>
              {conflict ? <>
                {conflict.description !== row.description && <span>{conflict.description}</span>}
                {conflict.occurred_at !== row.occurred_at && <span>{formatDate(conflict.occurred_at)}</span>}
                {formatMoney(conflict.amount, conflict.currency) !== formatMoney(row.amount ?? "0", currency) && <span>{formatMoney(conflict.amount, conflict.currency)}</span>}
                {conflict.payment_method !== row.payment_method && <span>{conflict.payment_method}</span>}
                <dl><dt>Saldo após</dt><dd>{conflict.balance === null ? "Não informado" : formatMoney(conflict.balance, conflict.currency)}</dd></dl>
                {conflict.filename && <span className="statement-row-meta">Arquivo: {conflict.filename}</span>}
              </> : <span>Dados da transação existente indisponíveis</span>}
            </section>
            <section aria-label={`Linha do arquivo ${row.line_number}`}>
              <strong>Linha do arquivo</strong>
              <dl><dt>Saldo após</dt><dd>{row.balance === null ? "Não informado" : formatMoney(row.balance, currency)}</dd></dl>
              {filename && <span className="statement-row-meta">Arquivo: {filename}</span>}
            </section>
          </div>
          <Radio.Group aria-label={`Decisão da linha ${row.line_number}`} value={decision}
            onChange={(event) => onDecision?.(row.line_number, event.target.value)}>
            <Radio value="ignore">Manter existente / ignorar esta linha</Radio>
            <Radio value="import">Importar como outra transação</Radio>
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
  const [page, setPage] = useState(1);

  if (rows.length === 0) return <EmptyState title="Nenhuma linha para mostrar" />;

  const totalPages = Math.ceil(rows.length / pageSize);
  const current = Math.min(page, totalPages);
  const start = (current - 1) * pageSize;
  const visible = rows.slice(start, start + pageSize);

  return (
    <>
      <ul className="statement-row-list" aria-label="Linhas do arquivo">
        {visible.map((row) => <StatementPreviewRow key={row.line_number} row={row} conflict={conflicts?.[row.line_number]} filename={filename}
          decision={ambiguousDecisions?.[row.line_number]} onDecision={onAmbiguousDecision} />)}
      </ul>
      {totalPages > 1 && (
        <footer className="statement-import-pagination">
          <span>
            Exibindo {(start + 1).toLocaleString("pt-BR")}–{(start + visible.length).toLocaleString("pt-BR")} de{" "}
            {rows.length.toLocaleString("pt-BR")} linhas
          </span>
          <nav aria-label="Paginação das linhas">
            <Button disabled={current <= 1} onClick={() => setPage(current - 1)}>Anterior</Button>
            <span>Página {current} de {totalPages}</span>
            <Button disabled={current >= totalPages} onClick={() => setPage(current + 1)}>Próxima</Button>
          </nav>
        </footer>
      )}
    </>
  );
}
