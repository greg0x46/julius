import { Grid } from "../ui";

/** Panels become full-screen and menus become sheets below 768px. */
export function useCompactScreen(): boolean {
  const screens = Grid.useBreakpoint();
  return !screens.md;
}
