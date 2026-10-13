/**
 * The text side of `MoneyInput`: what the field shows for a number, and the
 * number a shown or pasted text means. Kept apart from the component so the
 * rules can be tested as plain functions.
 */

/**
 * What the field shows for the number string rc-input-number hands over
 * ("12850.5" with a point, however it was typed): thousands grouped with "."
 * and the decimal comma — "12.850,5" while typing (a trailing comma is kept,
 * so "12," survives the keystroke), "12.850,50" once settled.
 */
export function formatMoney(value: string | number | undefined, { userTyping }: { userTyping: boolean }): string {
  if (value === undefined || value === "") return "";
  const [integer = "", decimals] = String(value).split(".");
  const sign = integer.startsWith("-") ? "-" : "";
  const whole = integer.replace(/\D/g, "").replace(/\B(?=(\d{3})+(?!\d))/g, ".");
  if (userTyping) return decimals === undefined ? `${sign}${whole}` : `${sign}${whole},${decimals}`;
  return `${sign}${whole === "" ? "0" : whole},${decimals?.padEnd(2, "0") ?? "00"}`;
}

function isSingleDeletion(previous: string, next: string): boolean {
  if (next.length !== previous.length - 1) return false;
  for (let i = 0; i < previous.length; i += 1) {
    if (previous.slice(0, i) + previous.slice(i + 1) === next) return true;
  }
  return false;
}

/**
 * The inverse: text (typed, pasted or as the field itself shows it) back to a
 * point-decimal string.
 *
 * - With a comma, the comma is the decimal mark and every "." is a thousands
 *   separator: "1.234,56" is 1234.56.
 * - Without a comma, ONE "." followed by one, two or more than three digits at the very end is
 *   a decimal point ("1234.56", "12.5" — what a US keyboard, a calculator or a
 *   copied figure gives); any other "." is a thousands separator ("12.850" is
 *   12850, "1.234" is 1234, "1.234.567" is 1234567).
 * - A "." typed at the end of a whole number ("1234.", "1.234.") is a decimal
 *   point on its way: the digits that follow are decimals.
 * - Deleting one character of what the field showed never creates a decimal
 *   point: the field itself put its dots there as thousands separators, so
 *   backspacing "12.850" gives 1.285, not 12.85. `previousShown` is the text
 *   the field last displayed.
 */
export function parseMoney(text: string | undefined, previousShown = ""): string {
  const clean = (text ?? "").replace(/[^\d.,-]/g, "");
  if (clean.includes(",")) return clean.replace(/\./g, "").replace(",", ".");
  if (!clean.includes(".")) return clean;
  if (/^-?\d{1,3}(\.\d{3})+$/.test(previousShown) && clean.startsWith(previousShown) && /^\d+$/.test(clean.slice(previousShown.length))) return clean.replace(/\./g, "");
  if (previousShown.includes(".") && isSingleDeletion(previousShown, clean)) return clean.replace(/\./g, "");
  // A point typed after the grouped whole part ("1.234" + "." stays on screen
  // as "1.234." until a digit follows) and then the decimals: "1.234.56".
  if (previousShown !== "" && clean.startsWith(`${previousShown}.`)) {
    const decimals = clean.slice(previousShown.length + 1);
    if (/^\d+$/.test(decimals)) return `${previousShown.replace(/\./g, "")}.${decimals}`;
  }
  if (/^-?\d{1,3}(\.\d{3})*\.$/.test(clean) || /^-?\d+\.$/.test(clean)) return clean.replace(/\.(?=.)/g, "");
  if (/^-?\d+\.(\d{1,2}|\d{4,})$/.test(clean)) return clean;
  return clean.replace(/\./g, "");
}
