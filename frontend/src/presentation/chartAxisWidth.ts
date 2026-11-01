import type { Language } from "../i18n/messages";
import { formatAxisMoney } from "./chartAxis";

/**
 * The Y axis is as wide as its widest tick: a fixed width wrapped "-500 mil"
 * onto two lines and wasted room on charts whose labels are "0" and "2 mil".
 * 12px digits are about 7px wide; one character of slack covers the rounded
 * ticks ("-500 mil" beside an extreme of "-1,5 mi") and 8px is the gap to the plot. The
 * widest label is one of the data's extremes (the axis rounds outward, which
 * keeps the same number of characters), or "0".
 */
export function yAxisWidth(values: number[], language: Language = "pt-BR"): number {
  const labels = [0, ...values].filter(Number.isFinite).map((value) => formatAxisMoney(value, language));
  const longest = Math.max(...labels.map((label) => label.length));
  return Math.max(32, Math.ceil((longest + 1) * 7) + 8);
}
