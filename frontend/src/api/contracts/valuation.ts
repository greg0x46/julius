import { decimal, isRecord, isValidDate, requiredRecord } from "./shared";

export interface Valuation {
  reference_currency: "BRL";
  valued_at: string;
  rate_policy: string;
  rates: Record<string, { value: string; observed_at: string; source: string }>;
}

export function parseValuation(value: unknown): Valuation {
  const item = requiredRecord(value, ["reference_currency", "valued_at", "rate_policy", "rates"], "Avaliação inválida.");
  if (item.reference_currency !== "BRL" || !isValidDate(item.valued_at) ||
      item.rate_policy !== "latest_observed_at_or_before_v1" || !isRecord(item.rates)) {
    throw new TypeError("Avaliação inválida.");
  }
  const rates: Valuation["rates"] = {};
  for (const [currency, value] of Object.entries(item.rates)) {
    const rate = requiredRecord(value, ["value", "observed_at", "source"], "Câmbio inválido.");
    const amount = decimal(rate.value);
    if (!/^[A-Z]{3}$/.test(currency) || Number(amount) <= 0 || !isValidDate(rate.observed_at) ||
        typeof rate.source !== "string" || rate.source.trim() === "" || Date.parse(rate.observed_at) > Date.parse(item.valued_at)) {
      throw new TypeError("Câmbio inválido.");
    }
    rates[currency] = { value: amount, observed_at: rate.observed_at, source: rate.source };
  }
  return { reference_currency: "BRL", valued_at: item.valued_at, rate_policy: item.rate_policy, rates };
}
