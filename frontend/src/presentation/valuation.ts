import type { FXRate } from "../api/contracts/valuation";
import { multiplyDecimals } from "./decimal";

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
  if (!fx.inverted) return multiplyDecimals(value, fx.value);
  const amount = parts(value);
  const rate = parts(fx.value);
  const numerator = amount.coefficient * rate.scale;
  const denominator = amount.scale * rate.coefficient;
  const precision = 10n ** 12n;
  const negative = numerator < 0n;
  const absolute = negative ? -numerator : numerator;
  const rounded = (absolute * precision + denominator / 2n) / denominator;
  const digits = rounded.toString().padStart(13, "0");
  return `${negative && rounded !== 0n ? "-" : ""}${digits.slice(0, -12)}.${digits.slice(-12)}`;
}
