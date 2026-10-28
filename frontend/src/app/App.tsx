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

const navItems: { path: string; name: string; icon: ReactNode }[] = [
  { path: "/", name: "Início", icon: <HomeOutlined /> },
  { path: "/transacoes", name: "Transações", icon: <TransactionOutlined /> },
  {
    path: "/contas-e-cartoes",
    name: "Contas e cartões",
    icon: <BankOutlined />,
  },
  { path: "/pendencias", name: "Pendências", icon: <WalletOutlined /> },
  { path: "/recorrencias", name: "Recorrências", icon: <RedoOutlined /> },
  { path: "/investimentos", name: "Investimentos", icon: <RiseOutlined /> },
  { path: "/metas", name: "Metas", icon: <AimOutlined /> },
  {
    path: "/patrimonio",
    name: "Patrimônio",
    icon: <FundOutlined />,
  },
  { path: "/configuracoes", name: "Configurações", icon: <SettingOutlined /> },
];
export function App() {
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
    <nav aria-label="Navegação principal">
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
                <span>{item.name}</span>
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
            aria-label="Abrir navegação"
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
              <span className="app-brand-subtitle">De graça !?</span>
            )}
          </span>
        </Link>
        <Switch
          className="app-theme-switch"
          aria-label="Tema escuro"
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
