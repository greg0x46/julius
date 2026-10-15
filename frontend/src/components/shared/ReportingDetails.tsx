import type { CurrencyTotals, ReportingSummary } from "../../api/contracts";
import { Money } from "./Money";

export function ReportingDetails({ report, totals = [] }: { report?: ReportingSummary; totals?: CurrencyTotals[] }) {
  const foreign = totals.some((t) => t.currency_code !== "BRL");
  return <>
    {report?.issues?.map((issue, index) => {
      const label = `${issue.currency}${issue.account ? ` (${issue.account})` : ""}`;
      const date = issue.date?.split("-").reverse().join("/") ?? "a data do relatório";
      const message = issue.reason === "unsupported"
        ? `${label}: o provedor não oferece conversão para BRL. Sincronizar não resolve essa limitação.`
        : issue.reason === "historical_unavailable"
          ? `${label}: não há cotação para ${date}. Escolha outro período ou consulte os valores na moeda original.`
          : `${label}: não foi possível consultar a cotação agora. Tente novamente mais tarde.`;
      return <p role="status" key={index}>{message}</p>;
    })}
    {report?.missing && !report.issues?.length ? <p role="status">Faltam cotações históricas para {report.missing} {report.missing === 1 ? "transação" : "transações"}.</p> : null}
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
