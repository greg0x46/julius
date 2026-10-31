import { useTranslation } from "../../i18n/context";
import type { Language } from "../../i18n/messages";
import type { ImportPreview } from "../../api/statementImports";
import { formatDateOnly } from "../../presentation/dates";
import { formatMoney } from "../../presentation/money";
import { DataCardSummary } from "../layout/DataCard";

function period(value: string | null, language: Language): string {
  return value ? formatDateOnly(value, language) : "—";
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
  const { t, language } = useTranslation();
  const balance = latestBalance(preview);

  return (
    <DataCardSummary label={t("previewSummary")} note={t("fileFormat", { format: preview.format })}>
      <div className="statement-import-summary">
        <dl className="statement-import-counts">
          <div>
            <dt>{t("countNew")}</dt>
            <dd>{preview.counts.new}</dd>
          </div>
          <div>
            <dt>{t("countAlreadyImported")}</dt>
            <dd>{preview.counts.duplicate}</dd>
          </div>
          {(preview.counts.ambiguous ?? 0) > 0 && <div>
            <dt>{t("countToDecide")}</dt>
            <dd>{preview.counts.ambiguous}</dd>
          </div>}
          <div className={preview.counts.invalid > 0 ? "is-invalid" : undefined}>
            <dt>{t("countInvalid")}</dt>
            <dd>{preview.counts.invalid}</dd>
          </div>
        </dl>
        <dl className="statement-import-facts">
          <div>
            <dt>{t("movementsPeriod")}</dt>
            <dd>{t("dateRange", { start: period(preview.period_start, language), end: period(preview.period_end, language) })}</dd>
          </div>
          {balance && (
            <div>
              <dt>{t("latestBalanceInFile")}</dt>
              <dd>{formatMoney(balance, preview.currency)}</dd>
            </div>
          )}
        </dl>
      </div>
    </DataCardSummary>
  );
}
