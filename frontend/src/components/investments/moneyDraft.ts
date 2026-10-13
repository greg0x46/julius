/** Drafts and MoneyInput both retain exact decimal strings. */
export function toMoneyInput(value: string | null): string | null {
  return value === "" ? null : value;
}

export function fromMoneyInput(value: string | null): string | null {
  if (value === null) return null;
  const [whole, fraction = ""] = value.split(".");
  return `${whole}.${fraction.padEnd(2, "0")}`;
}
