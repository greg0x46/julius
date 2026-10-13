import type { FXRate } from "../api/contracts/valuation";

function parts(value: string): { coefficient: bigint; scale: bigint } {
  const [whole, fraction = ""] = value.split(".");
  const negative = whole.startsWith("-");
  const magnitude = BigInt(whole.replace("-", "") + fraction);
  return {
    coefficient: negative ? -magnitude : magnitude,
    scale: 10n ** BigInt(fraction.length),
  };
}

/** Exact conversion of BRL aggregates computed from native investment records. */
export function valueInReferenceCurrency(value: string, fx?: FXRate): string {
  if (!fx || fx.base === fx.quote) return value;
  const amount = parts(value);
  const rate = parts(fx.value);
  const numerator = fx.inverted ? amount.coefficient * rate.scale : amount.coefficient * rate.coefficient;
  const denominator = fx.inverted ? amount.scale * rate.coefficient : amount.scale * rate.scale;
  const precision = 10n ** 12n;
  const negative = numerator < 0n;
  const absolute = negative ? -numerator : numerator;
  const rounded = (absolute * precision + denominator / 2n) / denominator;
  const digits = rounded.toString().padStart(13, "0");
  return `${negative && rounded !== 0n ? "-" : ""}${digits.slice(0, -12)}.${digits.slice(-12)}`;
}
