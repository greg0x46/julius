import { Grid } from "../ui";

/**
 * How a transaction row is laid out:
 * - `phone` (< 768px): two lines, the date and the category fold into the meta line;
 * - `narrow` (768–1199px): one line with a date column, but no category column —
 *   the category joins the meta line so the description keeps its room;
 * - `wide` (≥ 1200px): date, description, category and amount are all columns.
 */
export type RowLayout = "phone" | "narrow" | "wide";

/** Read once per list (not once per row) and pass the result down. */
export function useRowLayout(): RowLayout {
  const screens = Grid.useBreakpoint();
  if (!screens.md) return "phone";
  return screens.xl ? "wide" : "narrow";
}
