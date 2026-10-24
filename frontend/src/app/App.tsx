import { useState, type ReactNode } from "react";
import { Link, Outlet, useLocation } from "react-router-dom";
import {
  AimOutlined,
  BankOutlined,
  FundOutlined,
  HomeOutlined,
  RedoOutlined,
  RiseOutlined,
  SettingOutlined,
  TransactionOutlined,
  WalletOutlined,
} from "../components/ui/icons";
import { Button, Drawer } from "../components/ui";
import { BuildFooter } from "./BuildFooter";
import { DemoNotice } from "./DemoNotice";
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
  const compact = useCompactScreen();
  const [menuOpen, setMenuOpen] = useState(false);
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
