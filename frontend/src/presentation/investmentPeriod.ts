import type {
  InvestmentPeriodAssetClass,
  InvestmentPeriodGrouping,
  InvestmentPeriodPosition,
  InvestmentPeriodReason,
} from "../api/contracts";
import type { Period } from "../hooks/usePeriod";
import { multiplyDecimals, roundDecimal } from "./decimal";

/** Display only: the backend states the figures; these choose words and place the decimal point. */

export const investmentGroupingLabel: Record<InvestmentPeriodGrouping, string> = {
  class: "Classe de ativo",
  account: "Custódia",
  none: "Sem agrupamento",
};

export const investmentAssetClassLabel: Record<InvestmentPeriodAssetClass, string> = {
  fixed_income: "Renda fixa",
  variable_income: "Renda variável",
  multimarket: "Multimercado",
  currency: "Moeda",
  crypto: "Criptoativos",
  other: "Outros",
  unclassified: "Sem classe",
};

/** Why a position has no figures for the period. */
export const periodUnavailableLabel: Record<InvestmentPeriodReason, { short: string; hint: string }> = {
  sem_historico: {
    short: "Sem histórico",
    hint: "Nenhuma movimentação registrada para esta posição, então não há como medir o período.",
  },
  historico_incompleto: {
    short: "Histórico incompleto",
    hint: "As movimentações não cobrem a posição inteira (a instituição não enviou as aplicações anteriores à conexão). Calcular o período contaria o principal como lucro.",
  },
  saldo_indisponivel: {
    short: "Sem saldo",
    hint: "Não há saldo ou cotação para o início ou o fim do período.",
  },
  cambio_indisponivel: {
    short: "Sem câmbio",
    hint: "Não há cotação de câmbio para converter esta posição para a moeda de referência.",
  },
};

/** Why the invested amount ("valor investido") is unknown. */
export const investedUnavailableHint: Record<InvestmentPeriodReason, string> = {
  sem_historico: "Sem movimentações para saber quanto foi investido.",
  historico_incompleto: "O histórico não inclui a primeira aplicação, então o valor investido é desconhecido.",
  saldo_indisponivel: "Sem saldo para calcular o valor investido.",
  cambio_indisponivel: "Sem câmbio para converter o valor investido.",
};

/**
 * A ratio string ("0.0123456789" = 1,23%) as a pt-BR percentage, rounded on
 * the decimal text so nothing passes through a float. `signed` adds an
 * explicit "+" to positive values (a yield); zero is never signed.
 */
export function formatRatioPercent(ratio: string, { signed = false }: { signed?: boolean } = {}): string {
  const rounded = roundDecimal(multiplyDecimals(ratio, "100"), 2);
  const negative = rounded.startsWith("-");
  const [integer, fraction] = rounded.replace(/^-/, "").split(".");
  const zero = /^0+$/.test(`${integer}${fraction}`);
  const sign = zero ? "" : negative ? "-" : signed ? "+" : "";
  return `${sign}${integer.replace(/\B(?=(\d{3})+(?!\d))/g, ".")},${fraction}%`;
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
export function partialReasons(positions: InvestmentPeriodPosition[]): string[] {
  const counts = new Map<InvestmentPeriodReason, number>();
  for (const position of positions) {
    if (position.unavailable_reason !== null) {
      counts.set(position.unavailable_reason, (counts.get(position.unavailable_reason) ?? 0) + 1);
    }
  }
  return [...counts].map(([reason, count]) => `${count}× ${periodUnavailableLabel[reason].short}`);
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
