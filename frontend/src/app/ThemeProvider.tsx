import type { ReactNode } from "react";
import { App } from "../components/ui";

export function ThemeProvider({ children }: { children: ReactNode }) {
  return <App>{children}</App>;
}
