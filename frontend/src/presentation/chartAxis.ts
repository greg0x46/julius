import type { Language } from "../i18n/messages";

// The Y axis exists to give the curve a scale, not to be read to the cent —
// so its ticks are rounded to thousands and carry no "R$": the currency is
// named by the figures above the chart and by the tooltip, and dropping it
// keeps the axis narrow enough that a tick never wraps on a phone.

type AxisScale = { thousand: string; million: string; decimal: string };
const scales: Record<Language, AxisScale> = {
  "pt-BR": { thousand: " mil", million: " mi", decimal: "," },
  en: { thousand: "k", million: "M", decimal: "." },
};

/** Number with at most one decimal: 1.5 -> "1,5" (pt-BR) / "1.5" (en), 2 -> "2". */
function oneDecimal(value: number, decimal: string): string {
  return value.toFixed(1).replace(/\.0$/, "").replace(".", decimal);
}

export function formatAxisMoney(value: number, language: Language = "pt-BR"): string {
  const { thousand, million, decimal } = scales[language];
  const abs = Math.abs(value);
  // 999_500 is the first value that rounds to "1000 mil": from there it is "mi".
  if (abs >= 999_500) return `${value < 0 ? "-" : ""}${oneDecimal(abs / 1_000_000, decimal)}${million}`;
  if (abs >= 1_000) {
    // Below 10 mil one decimal when it is not a whole thousand, so 1_500 and
    // 2_000 are not both "2 mil"; from 10 mil up whole thousands are enough.
    const text = abs < 10_000 ? oneDecimal(Math.round(abs / 100) / 10, decimal) : String(Math.round(abs / 1_000));
    return `${value < 0 ? "-" : ""}${text}${thousand}`;
  }
  const rounded = Math.round(abs);
  // A value that rounds to zero is "0", never "-0".
  return `${value < 0 && rounded !== 0 ? "-" : ""}${rounded}`;
}
