import { useEffect, useState, type ReactNode } from "react";
import { Link, Outlet, useLocation, useNavigationType } from "react-router-dom";
import {
  AimOutlined,
  BankOutlined,
  FundOutlined,
  HomeOutlined,
  MoonOutlined,
  RedoOutlined,
  RiseOutlined,
  SettingOutlined,
  SunOutlined,
  TransactionOutlined,
  WalletOutlined,
} from "../components/ui/icons";
import { Button, Drawer, Switch } from "../components/ui";
import { BuildFooter } from "./BuildFooter";
import { DemoNotice } from "./DemoNotice";
import { useTheme } from "./theme";
import julius from "../assets/julius.png";
import { SkipLink } from "../components/SkipLink";
import { useCompactScreen } from "../components/shared/useCompactScreen";
import { useTranslation } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";

const navItems: { path: string; name: MessageKey; icon: ReactNode }[] = [
  { path: "/", name: "home", icon: <HomeOutlined /> },
  { path: "/transacoes", name: "transactions", icon: <TransactionOutlined /> },
  {
    path: "/contas-e-cartoes",
    name: "accounts",
    icon: <BankOutlined />,
  },
  { path: "/pendencias", name: "payables", icon: <WalletOutlined /> },
  { path: "/recorrencias", name: "recurrences", icon: <RedoOutlined /> },
  { path: "/investimentos", name: "investments", icon: <RiseOutlined /> },
  { path: "/metas", name: "goals", icon: <AimOutlined /> },
  {
    path: "/patrimonio",
    name: "netWorth",
    icon: <FundOutlined />,
  },
  { path: "/configuracoes", name: "settings", icon: <SettingOutlined /> },
];
export function App() {
  const { t, language, setLanguage } = useTranslation();
  const { pathname } = useLocation();
  const navigationType = useNavigationType();
  const compact = useCompactScreen();
  const [menuOpen, setMenuOpen] = useState(false);
  const [theme, setTheme] = useTheme();
  // pushState keeps the old scroll offset: a new page must open at its top.
  useEffect(() => {
    if (navigationType !== "POP") window.scrollTo?.(0, 0);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pathname]);
  const navigation = (
    <nav aria-label={t("navigation")}>
      <ul className="app-navigation">
        {navItems.map((item) => {
          const active =
            item.path === "/"
              ? pathname === "/"
              : pathname === item.path || pathname.startsWith(`${item.path}/`);
          return (
            <li key={item.path}>
              <Link
                to={item.path}
                aria-current={active ? "page" : undefined}
                onClick={() => setMenuOpen(false)}
              >
                {item.icon}
                <span>{t(item.name)}</span>
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
  return (
    <div className="app-layout">
      <SkipLink />
      <header className="app-header">
        {compact && (
          <Button
            type="text"
            aria-label={t("openNavigation")}
            aria-expanded={menuOpen}
            onClick={() => setMenuOpen(true)}
          >
            ☰
          </Button>
        )}
        <Link to="/" className="app-brand">
          <img src={julius} alt="" />
          <span className="app-brand-text">
            <span className="app-brand-title">Julius</span>
            {!compact && (
              <span className="app-brand-subtitle">{t("brandSubtitle")}</span>
            )}
          </span>
        </Link>
        <select
          className="app-language-select"
          aria-label={t("language")}
          value={language}
          onChange={(event) => setLanguage(event.target.value === "en" ? "en" : "pt-BR")}
        >
          <option value="pt-BR" lang="pt-BR">Português (Brasil)</option>
          <option value="en" lang="en">English</option>
        </select>
        <Switch
          className="app-theme-switch"
          aria-label={t("darkTheme")}
          checked={theme === "dark"}
          onChange={(dark) => setTheme(dark ? "dark" : "light")}
          checkedChildren={<MoonOutlined />}
          unCheckedChildren={<SunOutlined />}
        />
      </header>
      {compact ? (
        <Drawer
          open={menuOpen}
          onClose={() => setMenuOpen(false)}
          placement="left"
          width={280}
          title="Julius"
        >
          {navigation}
        </Drawer>
      ) : (
        <aside className="app-sidebar">{navigation}</aside>
      )}
      <main className="app-main" id="conteudo-principal" tabIndex={-1}>
        <DemoNotice />
        <Outlet />
        <BuildFooter />
      </main>
    </div>
  );
}
