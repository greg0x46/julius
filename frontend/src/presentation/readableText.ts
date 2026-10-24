/** Black or white, whichever contrasts more with a user-chosen `#rrggbb` fill. */
export function readableTextOn(hex: string): "#000000" | "#ffffff" {
  const m = /^#([0-9a-f]{6})$/i.exec(hex.trim());
  if (!m) return "#ffffff";
  const [r, g, b] = [0, 2, 4]
    .map((i) => parseInt(m[1].slice(i, i + 2), 16) / 255)
    .map((v) => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
  const luminance = 0.2126 * r + 0.7152 * g + 0.0722 * b;
  // Black beats white once luminance passes ~0.179.
  return luminance > 0.179 ? "#000000" : "#ffffff";
}
