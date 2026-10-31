import { dateFormatter } from "../../i18n/dateFormats";
import { useTranslation } from "../../i18n/context";
import { translate, type Language } from "../../i18n/messages";
import type { NetWorthBreakdown, NetWorthPoint } from "../../api/contracts";
import { SummaryStrip } from "../layout";
import { Money } from "../shared/Money";

// Wealth is what has already been realized: cash plus investments. A card, a
// debt or a bill to come is not wealth and has no line here (see
// internal/networth). The page title already names the figure, so the hero is
// labelled "Hoje"; the two parts read as plain balances (red only if negative).
function valuationNote(point: NetWorthPoint, language: Language): string {
  const rates = Object.keys(point.valuation?.rates ?? {});
  const parts: string[] = [];
  if (rates.length > 0) {
    parts.push(translate(language, "netWorthRatesNote", { currencies: rates.join(", "), date: dateFormatter(language, { timeZone: "UTC" }).format(new Date(point.valuation!.valued_at)) }));
  }
  if (point.valuation_status === "stale") parts.push(translate(language, "netWorthStaleNote"));
  return parts.join(" ");
}

export function NetWorthBreakdownCard({ point }: { point: NetWorthPoint & NetWorthBreakdown }) {
  const { t, language } = useTranslation();
  const extra = valuationNote(point, language);
  return (
    <SummaryStrip
      label={t("today")}
      value={<Money value={point.total} tone="balance" size="hero" />}
      note={`${t("cashPlusInvestments")}${extra ? ` ${extra}` : ""}`}
      items={[
        { label: t("cash"), value: <Money value={point.cash_balance} tone="balance" /> },
        { label: t("investmentsLabel"), value: <Money value={point.investment_balance} tone="balance" /> },
      ]}
    />
  );
}
