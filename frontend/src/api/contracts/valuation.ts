import { decimal, isRecord, isValidDate, requiredRecord } from "./shared";

export interface FXRate {
  base: string;
  quote: string;
  value: string;
  inverted: boolean;
  as_of: string;
  effective_on: string;
  source: string;
  fetched_at: string;
  stale: boolean;
}

export interface ReportValuation {
  currency_code?: string;
  fx?: FXRate;
}

export function parseValuation(value: Record<string, unknown>): ReportValuation {
  const result: ReportValuation = {};
  if (value.currency_code !== undefined) {
    if (typeof value.currency_code !== "string" || !/^[A-Z]{3}$/.test(value.currency_code)) {
      throw new TypeError("Moeda de avaliação inválida.");
    }
    result.currency_code = value.currency_code;
  }
  if (value.fx !== undefined) {
    const fx = requiredRecord(
      value.fx,
      ["base", "quote", "value", "inverted", "as_of", "effective_on", "source", "fetched_at", "stale"],
      "Cotação inválida.",
    );
    if (
      !isRecord(fx) ||
      typeof fx.base !== "string" ||
      typeof fx.quote !== "string" ||
      typeof fx.source !== "string" ||
      typeof fx.inverted !== "boolean" ||
      typeof fx.stale !== "boolean" ||
      !isValidDate(fx.as_of) ||
      !isValidDate(fx.effective_on) ||
      !isValidDate(fx.fetched_at)
    ) {
      throw new TypeError("Cotação inválida.");
    }
    const rate = decimal(fx.value);
    if (/^-|^0(?:\.0*)?$/.test(rate) || fx.quote !== result.currency_code)
      throw new TypeError("Cotação inválida.");
    result.fx = {
      base: fx.base,
      quote: fx.quote,
      value: rate,
      inverted: fx.inverted,
      as_of: fx.as_of,
      effective_on: fx.effective_on,
      source: fx.source,
      fetched_at: fx.fetched_at,
      stale: fx.stale,
    };
  }
  return result;
}
