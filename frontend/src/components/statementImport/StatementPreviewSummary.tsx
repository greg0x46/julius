import type { ImportPreview } from "../../api/statementImports";
import { formatDateOnly } from "../../presentation/dates";
import { formatMoney } from "../../presentation/money";
import { DataCardSummary } from "../layout/DataCard";

function period(value: string | null): string {
  return value ? formatDateOnly(value) : "—";
}

/** The balance of the file's newest movement, or null when the file does not settle on one. */
function latestBalance(preview: ImportPreview): string | null {
  const latest = preview.rows.filter((row) => row.status !== "invalid" && row.occurred_at && row.balance)
    .sort((a, b) => b.occurred_at!.localeCompare(a.occurred_at!));
  return latest.length > 0 && !latest.some((row) => row.occurred_at === latest[0].occurred_at && row.balance !== latest[0].balance)
    ? latest[0].balance : null;
}

/**
 * The summary strip of the import preview DataCard: what the file covers
 * (period, latest balance, format) and how its rows split into new, already
 * imported and invalid.
 */
export function StatementPreviewSummary({ preview }: { preview: ImportPreview }) {
  const balance = latestBalance(preview);

  return (
    <DataCardSummary label="Resumo da prévia" note={`Formato do arquivo: ${preview.format}`}>
      <div className="statement-import-summary">
        <dl className="statement-import-counts">
          <div>
            <dt>Novas</dt>
            <dd>{preview.counts.new}</dd>
          </div>
          <div>
            <dt>Já importadas</dt>
            <dd>{preview.counts.duplicate}</dd>
          </div>
          {(preview.counts.ambiguous ?? 0) > 0 && <div>
            <dt>Para decidir</dt>
            <dd>{preview.counts.ambiguous}</dd>
          </div>}
          <div className={preview.counts.invalid > 0 ? "is-invalid" : undefined}>
            <dt>Inválidas</dt>
            <dd>{preview.counts.invalid}</dd>
          </div>
        </dl>
        <dl className="statement-import-facts">
          <div>
            <dt>Período das movimentações</dt>
            <dd>{period(preview.period_start)} a {period(preview.period_end)}</dd>
          </div>
          {balance && (
            <div>
              <dt>Saldo mais recente no arquivo</dt>
              <dd>{formatMoney(balance, preview.currency)}</dd>
            </div>
          )}
        </dl>
      </div>
    </DataCardSummary>
  );
}
