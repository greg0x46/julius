import { DownOutlined } from "@ant-design/icons";
import { useId, useState } from "react";

import type { CurrencyTotals } from "../../api/contracts";
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
  reportTotals,
  totalItems,
  busy,
}: {
  totals: CurrencyTotals[];
  reportTotals?: CurrencyTotals;
  totalItems: number;
  busy?: boolean;
}) {
  const compact = useCompactScreen();
  const [expanded, setExpanded] = useState(false);
  const total = reportTotals ?? totals.find((total) => total.currency_code === "BRL") ?? {
    currency_code: "BRL",
    inflow: "0",
    outflow: "0",
    balance: "0",
  };
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
            <Money value={total.balance} currencyCode={total.currency_code} tone="result" className="transaction-summary-value" />
          </span>
          {!compact && (
            <>
              <span className="transaction-summary-figure">
                <span className="transaction-summary-label">Entradas</span>
                <span className="transaction-summary-value">{formatReportMoney(total.inflow, total.currency_code)}</span>
              </span>
              <span className="transaction-summary-figure">
                <span className="transaction-summary-label">Saídas</span>
                <span className="transaction-summary-value">{formatReportMoney(total.outflow, total.currency_code)}</span>
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

        {compact && expanded && (
          <dl id={detailsId} className="transaction-summary-details">
            <div>
              <dt>Entradas</dt>
              <dd>{formatReportMoney(total.inflow, total.currency_code)}</dd>
            </div>
            <div>
              <dt>Saídas</dt>
              <dd>{formatReportMoney(total.outflow, total.currency_code)}</dd>
            </div>
          </dl>
        )}
      </div>
    </DataCardSummary>
  );
}
