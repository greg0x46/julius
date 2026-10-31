import { lazy, Suspense } from "react";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";

import { HomePage } from "../pages/HomePage";
import { NotFoundPage } from "../pages/NotFoundPage";
import { App } from "./App";
import { LegacySettingsRedirect } from "./LegacySettingsRedirect";
import { AuthGate } from "./AuthGate";
import { TimezoneProvider } from "./TimezoneProvider";
import { AppLoadingScreen } from "./AppLoadingScreen";

const SyncRunListPage = lazy(() =>
  import("../pages/SyncRunListPage").then((module) => ({
    default: module.SyncRunListPage,
  })),
);
const SyncRunDetailPage = lazy(() =>
  import("../pages/SyncRunDetailPage").then((module) => ({
    default: module.SyncRunDetailPage,
  })),
);
const StatementImportPage = lazy(() =>
  import("../pages/StatementImportPage").then((module) => ({ default: module.StatementImportPage })),
);
const AccountsPage = lazy(() =>
  import("../pages/AccountsPage").then((module) => ({
    default: module.AccountsPage,
  })),
);
const AccountDetailPage = lazy(() =>
  import("../pages/AccountDetailPage").then((module) => ({
    default: module.AccountDetailPage,
  })),
);
const TransactionsPage = lazy(() =>
  import("../pages/TransactionsPage").then((module) => ({
    default: module.TransactionsPage,
  })),
);
const AutomationRulesPage = lazy(() =>
  import("../pages/AutomationRulesPage").then((module) => ({
    default: module.AutomationRulesPage,
  })),
);
const PayablesPage = lazy(() =>
  import("../pages/PayablesPage").then((module) => ({
    default: module.PayablesPage,
  })),
);
const PayableDetailPage = lazy(() =>
  import("../pages/PayableDetailPage").then((module) => ({
    default: module.PayableDetailPage,
  })),
);
const CategoriesPage = lazy(() =>
  import("../pages/CategoriesPage").then((module) => ({
    default: module.CategoriesPage,
  })),
);
const GoalsPage = lazy(() =>
  import("../pages/GoalsPage").then((module) => ({
    default: module.GoalsPage,
  })),
);
const InvestmentsPage = lazy(() =>
  import("../pages/InvestmentsPage").then((module) => ({
    default: module.InvestmentsPage,
  })),
);
const InvestmentDetailPage = lazy(() =>
  import("../pages/InvestmentDetailPage").then((module) => ({
    default: module.InvestmentDetailPage,
  })),
);
const SettingsPage = lazy(() =>
  import("../pages/SettingsPage").then((module) => ({
    default: module.SettingsPage,
  })),
);
const RecurringCommitmentsPage = lazy(() =>
  import("../pages/RecurringCommitmentsPage").then((module) => ({
    default: module.RecurringCommitmentsPage,
  })),
);
const ScenariosPage = lazy(() =>
  import("../pages/ScenariosPage").then((module) => ({
    default: module.ScenariosPage,
  })),
);
const NetWorthPage = lazy(() =>
  import("../pages/NetWorthPage").then((module) => ({
    default: module.NetWorthPage,
  })),
);

const GeneralSettingsPage = lazy(() => import("../pages/GeneralSettingsPage").then((module) => ({ default: module.GeneralSettingsPage })));

const SecuritySettingsPage = lazy(() => import("../pages/SecuritySettingsPage").then((module) => ({ default: module.SecuritySettingsPage })));

const InvestmentAssetsSettingsPage = lazy(() => import("../pages/InvestmentAssetsSettingsPage").then((module) => ({ default: module.InvestmentAssetsSettingsPage })));

export function AppRouter() {
  return (
    <BrowserRouter>
      <Suspense fallback={<AppLoadingScreen />}>
        <Routes>
          <Route
            element={
              <AuthGate>
                <TimezoneProvider>
                  <App />
                </TimezoneProvider>
              </AuthGate>
            }
          >
            <Route index element={<HomePage />} />
            <Route path="configuracoes/open-banking" element={<SyncRunListPage />} />
            <Route path="configuracoes/open-banking/sync-runs/:id" element={<SyncRunDetailPage />} />
            <Route path="contas-e-cartoes" element={<AccountsPage />} />
            <Route path="contas-e-cartoes/importar" element={<StatementImportPage />} />
            <Route path="contas-e-cartoes/:id" element={<AccountDetailPage />} />
            <Route path="transacoes" element={<TransactionsPage />} />
            <Route path="configuracoes/automacoes" element={<AutomationRulesPage />} />
            <Route path="pendencias" element={<PayablesPage />} />
            <Route path="pendencias/:id" element={<PayableDetailPage />} />
            <Route path="recorrencias" element={<RecurringCommitmentsPage />} />
            <Route path="configuracoes/cenarios" element={<ScenariosPage />} />
            <Route path="patrimonio" element={<NetWorthPage />} />
            <Route path="patrimonio-liquido" element={<Navigate to="/patrimonio" replace />} />
            <Route path="metas" element={<GoalsPage />} />
            <Route path="investimentos" element={<InvestmentsPage />} />
            <Route path="investimentos/:id" element={<InvestmentDetailPage />} />
            <Route path="configuracoes/categorias" element={<CategoriesPage />} />
            {['open-banking', 'open-banking/sync-runs/:id', 'automacoes', 'cenarios', 'categorias'].map((path) => (
              <Route key={path} path={path} element={<LegacySettingsRedirect />} />
            ))}
            <Route path="configuracoes/geral" element={<GeneralSettingsPage />} />
            <Route path="configuracoes/seguranca" element={<SecuritySettingsPage />} />
            <Route path="configuracoes/ativos-de-investimento" element={<InvestmentAssetsSettingsPage />} />
            <Route path="configuracoes" element={<SettingsPage />} />
            <Route path="*" element={<NotFoundPage />} />
          </Route>
        </Routes>
      </Suspense>
    </BrowserRouter>
  );
}
