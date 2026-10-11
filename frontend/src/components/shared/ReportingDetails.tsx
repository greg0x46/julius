import type { CurrencyTotals, ReportingSummary } from "../../api/contracts";
import { Money } from "./Money";

export function ReportingDetails({ report, totals = [] }: { report?: ReportingSummary; totals?: CurrencyTotals[] }) {
  const foreign = totals.some((t) => t.currency_code !== "BRL");
  return <>
    {report?.missing ? <p role="status">Faltam cotações históricas para {report.missing} {report.missing === 1 ? "transação" : "transações"}.</p> : null}
    {report?.stale ? <p role="status">Totais com cotações históricas desatualizadas.</p> : null}
    {foreign && <details>
      <summary>Totais por moeda</summary>
      {totals.map((t) => <dl key={t.currency_code} aria-label={`Totais em ${t.currency_code}`}>
        <div><dt>Entradas ({t.currency_code})</dt><dd><Money value={t.inflow} currencyCode={t.currency_code} tone="neutral" /></dd></div>
        <div><dt>Saídas ({t.currency_code})</dt><dd><Money value={t.outflow} currencyCode={t.currency_code} tone="neutral" /></dd></div>
        <div><dt>Resultado ({t.currency_code})</dt><dd><Money value={t.balance} currencyCode={t.currency_code} tone="result" /></dd></div>
      </dl>)}
    </details>}
  </>;
}
