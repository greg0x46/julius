import { translate, type Language } from "../i18n/messages";
import type {
  InvestmentPeriodAssetClass,
  InvestmentPeriodPosition,
  InvestmentPeriodReason,
} from "../api/contracts";
import type { Period } from "../hooks/usePeriod";
import { multiplyDecimals, roundDecimal } from "./decimal";

/** Display only: the backend states the figures; these choose words and place the decimal point. */

/**
 * A ratio string ("0.0123456789" = 1,23%) as a pt-BR percentage, rounded on
 * the decimal text so nothing passes through a float. `signed` adds an
 * explicit "+" to positive values (a yield); zero is never signed.
 */
export function formatRatioPercent(ratio: string, { signed = false, language = "pt-BR" }: { signed?: boolean; language?: Language } = {}): string {
  const rounded = roundDecimal(multiplyDecimals(ratio, "100"), 2);
  const negative = rounded.startsWith("-");
  const [integer, fraction] = rounded.replace(/^-/, "").split(".");
  const zero = /^0+$/.test(`${integer}${fraction}`);
  const sign = zero ? "" : negative ? "-" : signed ? "+" : "";
  return `${sign}${integer.replace(/\B(?=(\d{3})+(?!\d))/g, language === "en" ? "," : ".")}${language === "en" ? "." : ","}${fraction}%`;
}

/** Sign of a decimal/ratio string for tone and wording; "0" when it rounds to nothing at 2 places. */
export function ratioDirection(ratio: string): "positive" | "negative" | "zero" {
  const rounded = roundDecimal(multiplyDecimals(ratio, "100"), 2);
  if (/^-?0\.00$/.test(rounded)) return "zero";
  return rounded.startsWith("-") ? "negative" : "positive";
}

export type PeriodRequest = { from: string | null; to: string };

/**
 * The request window for a page period. "Todo o período" omits `from`. A
 * window that starts after today has nothing to show and yields null (no
 * request; the server would answer 400). A future end is sent as is: the
 * server cuts it at today and says so (`to_clamped`).
 */
export function periodRequest(period: Period, today: string): PeriodRequest | null {
  if (period.from === null) return { from: null, to: today };
  if (period.from > today) return null;
  return { from: period.from, to: period.to };
}

/** What "N positions are partial" is made of, for the summary's tooltip. */
export function partialReasons(positions: InvestmentPeriodPosition[], language: Language = "pt-BR"): string[] {
  const counts = new Map<InvestmentPeriodReason, number>();
  for (const position of positions) {
    if (position.unavailable_reason !== null) {
      counts.set(position.unavailable_reason, (counts.get(position.unavailable_reason) ?? 0) + 1);
    }
  }
  const keys = { sem_historico: "noInvestmentHistory", historico_incompleto: "incompleteInvestmentHistory", saldo_indisponivel: "noInvestmentBalance", cambio_indisponivel: "noExchangeRate" } as const;
  return [...counts].map(([reason, count]) => `${count}× ${translate(language, keys[reason])}`);
}

const normalize = (value: string) =>
  value
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .toLowerCase();

/** Search by asset name or ticker, accent- and case-insensitive. Hides rows only. */
export function matchesAssetSearch(
  position: Pick<InvestmentPeriodPosition, "name" | "ticker">,
  search: string,
): boolean {
  const needle = normalize(search.trim());
  if (needle === "") return true;
  return normalize(`${position.name} ${position.ticker ?? ""}`).includes(needle);
}

export function investmentClassName(value: InvestmentPeriodAssetClass, language: Language = "pt-BR"): string {
  const keys = { fixed_income: "assetClassFixedIncome", variable_income: "assetClassVariableIncome", multimarket: "assetClassMultimarket", currency: "assetClassCurrency", crypto: "assetClassCrypto", other: "assetClassOther", unclassified: "assetClassUnclassified" } as const;
  return translate(language, keys[value]);
}

export function investmentUnavailable(reason: InvestmentPeriodReason, language: Language = "pt-BR") {
  const shortKeys = { sem_historico: "noInvestmentHistory", historico_incompleto: "incompleteInvestmentHistory", saldo_indisponivel: "noInvestmentBalance", cambio_indisponivel: "noExchangeRate" } as const;
  const hintKeys = { sem_historico: "investmentHistoryMissingHint", historico_incompleto: "investmentHistoryIncompleteHint", saldo_indisponivel: "investmentBalanceMissingHint", cambio_indisponivel: "investmentExchangeMissingHint" } as const;
  const investedKeys = { sem_historico: "investedNoHistory", historico_incompleto: "investedIncompleteHistory", saldo_indisponivel: "investedNoBalance", cambio_indisponivel: "investedNoExchange" } as const;
  return { short: translate(language, shortKeys[reason]), hint: translate(language, hintKeys[reason]), investedHint: translate(language, investedKeys[reason]) };
}
