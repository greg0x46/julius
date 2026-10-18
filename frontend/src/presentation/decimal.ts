// Exact arithmetic for investment quantities, prices and reconciliation parcels.
// Formatting is separate: rounding a funding amount before recording its trade
// can leave a fractional purchase without enough cash.
function scaled(value: string): { digits: bigint; scale: number } {
  const negative = value.startsWith("-");
  const [whole = "0", fraction = ""] = (negative ? value.slice(1) : value).split(".");
  const digits = BigInt(`${whole || "0"}${fraction}`);
  return { digits: negative ? -digits : digits, scale: fraction.length };
}
function text(digits: bigint, scale: number, minimumScale = 2): string {
  if (scale < minimumScale) { digits *= 10n ** BigInt(minimumScale - scale); scale = minimumScale; }
  if (scale === 0) return digits.toString();
  const negative = digits < 0n;
  const raw = (negative ? -digits : digits).toString().padStart(scale + 1, "0");
  return `${negative ? "-" : ""}${raw.slice(0, -scale)}.${raw.slice(-scale)}`;
}
export function sumDecimals(values: string[]): string {
  const parts = values.map(scaled);
  const scale = Math.max(2, ...parts.map((part) => part.scale));
  return text(parts.reduce((total, part) => total + part.digits * 10n ** BigInt(scale - part.scale), 0n), scale);
}
export function subtractDecimals(left: string, right: string): string {
  return sumDecimals([left, right.startsWith("-") ? right.slice(1) : `-${right}`]);
}
export function multiplyDecimals(left: string, right: string): string {
  const a = scaled(left), b = scaled(right);
  return text(a.digits * b.digits, a.scale + b.scale);
}
export function isPositiveDecimal(value: string): boolean { return scaled(value).digits > 0n; }
export function isZeroDecimal(value: string): boolean { return scaled(value).digits === 0n; }

/** Nearest decimal, with ties away from zero; only for explicit boundaries. */
export function roundDecimal(value: string, places: number): string {
  if (!Number.isInteger(places) || places < 0) throw new RangeError("places must be a non-negative integer");
  const part = scaled(value.trim());
  if (part.scale <= places) return text(part.digits, part.scale, places);
  const factor = 10n ** BigInt(part.scale - places);
  const magnitude = part.digits < 0n ? -part.digits : part.digits;
  const rounded = magnitude / factor + (magnitude % factor * 2n >= factor ? 1n : 0n);
  return text(part.digits < 0n ? -rounded : rounded, places, 0);
}

export function compareDecimals(left: string, right: string): number {
  const difference = scaled(subtractDecimals(left, right)).digits;
  return difference < 0n ? -1 : difference > 0n ? 1 : 0;
}

/** `part` as a share of `total`, floored to an integer in 0–100; 0 when `total` is not positive. */
export function floorPercent(part: string, total: string): number {
  const a = scaled(part), b = scaled(total);
  const scale = Math.max(a.scale, b.scale);
  const numerator = a.digits * 10n ** BigInt(scale - a.scale);
  const denominator = b.digits * 10n ** BigInt(scale - b.scale);
  if (denominator <= 0n || numerator <= 0n) return 0;
  return numerator >= denominator ? 100 : Number(numerator * 100n / denominator);
}

/** Non-negative shares floored to cents without a rounded intermediate. */
export function divideDecimalFloor(value: string, divisor: number): string {
  if (!Number.isInteger(divisor) || divisor <= 0) throw new RangeError("divisor must be a positive integer");
  const part = scaled(value);
  if (part.digits < 0n) throw new RangeError("value must not be negative");
  return text(part.digits * 100n / (10n ** BigInt(part.scale) * BigInt(divisor)), 2);
}
