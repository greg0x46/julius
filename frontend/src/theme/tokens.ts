/**
 * Design tokens for Julius.
 *
 * These formalize the palette that had already emerged organically across
 * the app's CSS files (navy text, slate-gray secondary text, green primary,
 * green/red for positive/negative money, orange for warnings) into a single
 * source of truth for local components. New CSS and components
 * should reference these instead of hardcoding hex values.
 */
export const colors = {
  textPrimary: "#172b4d",
  // 5.2:1 on the page background — the floor for any meaningful secondary text.
  textSecondary: "#5d697c",
  // Only on white and only for non-essential hints; fails AA on the grey bg.
  textTertiary: "#6b778c",
  // Brand green, as a fill: marks, meters, chart, active indicator. Never
  // use it for text on white (2.8:1) — use `primary` for that.
  accent: "#1baf7a",
  // Interactive green (buttons, links, active tab, focus): 5.1:1 on white.
  primary: "#0f7d58",
  primaryHover: "#0b6a4a",
  // Money coming in and "Quitada"-style success text.
  success: "#1f6f46",
  // The single red; replaces the older #a62920 / #e34948 / #f5222d.
  error: "#c4302b",
  // Warning as a fill (meters, focus ring) and as text on white.
  warning: "#f2a900",
  warningText: "#8a5a00",
  // Informational, deliberately not green so it never reads as "success".
  info: "#3b5b8c",
  // Light tint of the brand green for "selected" rows in menus and selects;
  // paired with the normal text colour, never with green text.
  selectedTint: "#eef6f2",
  selectedTintHover: "#e4f1ea",
  // Backgrounds of the StatusTag tones; each pairs with its text colour at
  // >= 4.5:1 (neutral 4.9, info 5.9, success 5.4, warning 5.3, danger 4.7).
  tagNeutralBg: "#eef0f4",
  tagInfoBg: "#e9eff8",
  tagSuccessBg: "#e5f3ec",
  tagWarningBg: "#fcf1d6",
  tagDangerBg: "#fbe8e7",
  // Row hover on a white surface (mirrors --color-hover).
  hover: "rgba(23, 43, 77, 0.035)",
  border: "#d9d9d9",
  borderSecondary: "#e5e9f0",
  bgLayout: "#f5f7fa",
  bgContainer: "#ffffff",
} as const;

export const fontFamily =
  'Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif';


export const spacing = {
  xs: 4,
  sm: 8,
  md: 16,
  lg: 24,
  xl: 32,
  xxl: 48,
} as const;
