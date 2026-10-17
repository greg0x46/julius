import { Button, Radio } from "antd";
import { useState } from "react";

import type { ImportRow } from "../../api/statementImports";
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
function StatementPreviewRow({ row, decision, onDecision }: {
  row: ImportRow;
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
        </span>
      </div>
      <div className="statement-row-figures">
        <span className="statement-row-amount">{row.amount ? formatMoney(row.amount, currency) : "—"}</span>
        <span className="statement-row-balance">Saldo após {row.balance ? formatMoney(row.balance, currency) : "—"}</span>
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
          {row.status === "ambiguous" && <Radio.Group aria-label={`Decisão da linha ${row.line_number}`} value={decision}
            onChange={(event) => onDecision?.(row.line_number, event.target.value)}>
            <Radio value="import">Importar como outra compra</Radio>
            <Radio value="ignore">Ignorar como repetição</Radio>
          </Radio.Group>}
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
export function StatementPreviewRows({ rows, ambiguousDecisions, onAmbiguousDecision }: {
  rows: ImportRow[];
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
        {visible.map((row) => <StatementPreviewRow key={row.line_number} row={row}
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
