/** Source currency units per destination unit; same 16-place rounding as the API. */
export function transferRate(sourceAmount: string, destinationAmount: string): string {
  const parts = (value: string) => {
    const [whole, fraction = ""] = value.replace(/^-/, "").split(".");
    return { digits: BigInt(`${whole}${fraction}`), scale: fraction.length };
  };
  const source = parts(sourceAmount), destination = parts(destinationAmount);
  if (destination.digits === 0n) return "";
  const numerator = source.digits * 10n ** BigInt(destination.scale + 16);
  const denominator = destination.digits * 10n ** BigInt(source.scale);
  const rounded = (numerator + denominator / 2n) / denominator;
  const text = rounded.toString().padStart(17, "0");
  return `${text.slice(0, -16)}.${text.slice(-16)}`.replace(/0+$/, "").replace(/\.$/, "");
}
