import { translate } from "../../i18n/messages";
import { useTranslation } from "../../i18n/context";
import { formatDateOnly } from "../../presentation/dates";
import type { CurrencyTotals, ReportingSummary } from "../../api/contracts";
import { Money } from "./Money";

export function ReportingDetails({ report, totals = [] }: { report?: ReportingSummary; totals?: CurrencyTotals[] }) {
  const { t, language } = useTranslation();
  const foreign = totals.some((t) => t.currency_code !== "BRL");
  return <>
    {report?.issues?.map((issue, index) => {
      const label = `${issue.currency}${issue.account ? ` (${issue.account})` : ""}`;
      const date = issue.date ? formatDateOnly(issue.date, language) : t("reportDate");
      const message = issue.reason === "unsupported"
        ? t("fxUnsupported", { label })
        : issue.reason === "historical_unavailable"
          ? t("fxHistoricalUnavailable", { label, date })
          : t("fxUnavailable", { label });
      return <p role="status" key={index}>{message}</p>;
    })}
    {report?.missing && !report.issues?.length ? <p role="status">{t(report.missing === 1 ? "missingOneRate" : "missingRates").replace("{count}", String(report.missing))}</p> : null}
    {report?.stale ? <p role="status">{t("staleHistoricalTotals")}</p> : null}
    {foreign && <details>
      <summary>{t("totalsByCurrency")}</summary>
      {totals.map((t) => <dl key={t.currency_code} aria-label={translate(language, "totalsInCurrency").replace("{currency}", t.currency_code)}>
        <div><dt>{translate(language, "inflows")} ({t.currency_code})</dt><dd><Money value={t.inflow} currencyCode={t.currency_code} tone="neutral" /></dd></div>
        <div><dt>{translate(language, "outflows")} ({t.currency_code})</dt><dd><Money value={t.outflow} currencyCode={t.currency_code} tone="neutral" /></dd></div>
        <div><dt>{translate(language, "result")} ({t.currency_code})</dt><dd><Money value={t.balance} currencyCode={t.currency_code} tone="result" /></dd></div>
      </dl>)}
    </details>}
  </>;
}
