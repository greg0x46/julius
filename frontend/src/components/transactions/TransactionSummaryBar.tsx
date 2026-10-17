import { DownOutlined } from "@ant-design/icons";
import { useId, useState } from "react";

import { ReportingDetails } from "../shared/ReportingDetails";
import { reportingLabel, reportingTotals } from "../../presentation/reporting";
import type { CurrencyTotals, ReportingSummary } from "../../api/contracts";
import { formatReportMoney } from "../../presentation/money";
import { DataCardSummary } from "../layout/DataCard";
import { Money } from "../shared/Money";
import { useCompactScreen } from "../shared/useCompactScreen";

/**
 * The summary strip of the transactions DataCard: how many rows and the
 * result of everything the filters matched, with the inflow / outflow
 * figures and the note about what the totals include.
 *
 * On a phone the strip stays one ~40px line — "19 transações · Resultado
 * -R$ 2.656,18" — with a chevron that unfolds Entradas, Saídas and the note;
 * the figures that explain the result are not worth a permanent half
 * screen of a sticky bar. On a wide screen everything shows, in one quiet
 * line without icons or big figures.
 */
export function TransactionSummaryBar({
  totals,
  reporting,
  totalItems,
  busy,
}: {
  totals: CurrencyTotals[];
  reporting?: ReportingSummary;
  totalItems: number;
  busy?: boolean;
}) {
  const compact = useCompactScreen();
  const [expanded, setExpanded] = useState(false);
  const brl = reportingTotals(reporting, totals);
  const showDetails = !compact || expanded;
  const detailsId = useId();

  return (
    <DataCardSummary
      label="Resumo financeiro"
      busy={busy}
      note={showDetails ? "Totais dos filtros aplicados, sem transações ignoradas." : undefined}
    >
      <div className={`transaction-summary-bar ${expanded ? "is-expanded" : ""}`}>
        <p className="transaction-summary-line">
          <span className="transaction-summary-count">
            {totalItems.toLocaleString("pt-BR")} {totalItems === 1 ? "transação" : "transações"}
          </span>
          <span className="transaction-summary-figure transaction-summary-result">
            <span className="transaction-summary-label">Resultado</span>
            {brl ? <Money value={brl.balance} currencyCode={brl.currency_code} report tone="result" className="transaction-summary-value" /> : <span role="status">{reporting ? reportingLabel(reporting) : "Total indisponível"}</span>}
          </span>
          {!compact && brl && (
            <>
              <span className="transaction-summary-figure">
                <span className="transaction-summary-label">Entradas</span>
                <span className="transaction-summary-value">{formatReportMoney(brl.inflow, brl.currency_code)}</span>
              </span>
              <span className="transaction-summary-figure">
                <span className="transaction-summary-label">Saídas</span>
                <span className="transaction-summary-value">{formatReportMoney(brl.outflow, brl.currency_code)}</span>
              </span>
            </>
          )}
        </p>

        {compact && (
          <button
            type="button"
            className="transaction-summary-toggle"
            aria-expanded={expanded}
            aria-controls={expanded ? detailsId : undefined}
            aria-label={expanded ? "Ocultar resumo" : "Ver resumo"}
            onClick={() => setExpanded((value) => !value)}
          >
            <DownOutlined aria-hidden="true" />
          </button>
        )}

        {compact && expanded && brl && (
          <dl id={detailsId} className="transaction-summary-details">
            <div>
              <dt>Entradas</dt>
              <dd>{formatReportMoney(brl.inflow, brl.currency_code)}</dd>
            </div>
            <div>
              <dt>Saídas</dt>
              <dd>{formatReportMoney(brl.outflow, brl.currency_code)}</dd>
            </div>
          </dl>
        )}
      </div>
      <ReportingDetails report={reporting} totals={totals} />
    </DataCardSummary>
  );
}
