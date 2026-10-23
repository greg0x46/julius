import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";

import { createAppQueryClient } from "./app/queryClient";
import { AppRouter } from "./app/router";
import { ThemeProvider } from "./app/ThemeProvider";
import "./styles/ui.css";
import "./styles/global.css";
import "./styles/forms.css";
import "./styles/sync-runs.css";
import "./styles/period.css";
import "./styles/layout.css";
import "./styles/transactions.css";
import "./styles/filters.css";
import "./styles/panel.css";
import "./styles/categories.css";
import "./styles/home.css";
import "./styles/payables.css";
import "./styles/timeline.css";
import "./styles/accounts.css";
import "./styles/goals.css";
import "./styles/investments.css";
import "./styles/netWorth.css";
import "./styles/scenarios.css";
import "./styles/statement-imports.css";
import "./styles/settings.css";


const root = document.getElementById("root");

if (root === null) {
  throw new Error("Elemento raiz da aplicação não encontrado.");
}

const queryClient = createAppQueryClient();

createRoot(root).render(
  <StrictMode>
    <ThemeProvider>
      <QueryClientProvider client={queryClient}>
        <AppRouter />
      </QueryClientProvider>
    </ThemeProvider>
  </StrictMode>,
);
