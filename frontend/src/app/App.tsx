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
} from "@ant-design/icons";
import ProLayout from "@ant-design/pro-layout";
import type { ReactNode } from "react";
import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";

import { BuildFooter } from "./BuildFooter";
import { DemoNotice } from "./DemoNotice";

import julius from "../assets/julius.png";
import { SkipLink } from "../components/SkipLink";
import { useCompactScreen } from "../components/shared/useCompactScreen";
import { colors } from "../theme/tokens";

const headerBg = "#000000";
const subtitle = "De graça !?";

/** App bar heights; layout.css mirrors them as `--app-bar-height`. */
const appBarHeight = 64;
const compactAppBarHeight = 56;

interface NavItem {
  path: string;
  name: string;
  icon: ReactNode;
}

/**
 * The navigation, defined once and in the order it reads: what you look at
 * every day first, then what you set up. pro-layout derives both the menu and
 * the highlighted item from it.
 */
const navItems: NavItem[] = [
  { path: "/", name: "Início", icon: <HomeOutlined /> },
  { path: "/transacoes", name: "Transações", icon: <TransactionOutlined /> },
  { path: "/contas-e-cartoes", name: "Contas e cartões", icon: <BankOutlined /> },
  { path: "/pendencias", name: "Pendências", icon: <WalletOutlined /> },
  { path: "/recorrencias", name: "Recorrências", icon: <RedoOutlined /> },
  { path: "/investimentos", name: "Investimentos", icon: <RiseOutlined /> },
  { path: "/metas", name: "Metas", icon: <AimOutlined /> },
  { path: "/patrimonio", name: "Patrimônio", icon: <FundOutlined /> },
  { path: "/configuracoes", name: "Configurações", icon: <SettingOutlined /> },
];

export function App() {
  const location = useLocation();
  const navigate = useNavigate();
  const compact = useCompactScreen();
  // pro-layout works out the active item from `location` by itself. Only the
  // settings area needs help: its sub-pages live under /configuracoes/* and
  // pro-layout would not match them to the single "Configurações" item.
  // Passing `selectedKeys` for any other path (even as undefined) overrides
  // that computation and leaves nothing highlighted.
  const menuProps = location.pathname.startsWith("/configuracoes")
    ? { selectedKeys: ["/configuracoes"] }
    : undefined;

  return (
    <>
      <SkipLink />
      <ProLayout
        className="app-layout"
        style={{ minHeight: "100vh" }}
        title="Julius"
        logo={julius}
        token={{
          header: {
            heightLayoutHeader: compact ? compactAppBarHeight : appBarHeight,
            colorBgHeader: headerBg,
            colorBgScrollHeader: headerBg,
            colorHeaderTitle: colors.bgContainer,
          },
          sider: {
            // Defaults to a neutral gray/black highlight regardless of
            // colorPrimary, so the active nav item has to be told explicitly
            // to pick up the theme: the readable green for the label, a
            // tint of the brand green behind it.
            colorTextMenuSelected: colors.primary,
            colorBgMenuItemSelected: `${colors.accent}1f`,
            // Defaults to fully transparent, and pro-layout paints this
            // straight onto the fixed sider's own DOM node via its
            // CSS-in-JS, which wins the cascade over the opaque background
            // global.css sets on .ant-pro-sider — scrolling page content
            // then shows through the "solid" sider. Same class of bug the
            // header comment below already works around; fix it the same
            // way, via the token instead of CSS.
            colorMenuBackground: colors.bgContainer,
          },
        }}
        location={location}
        menuProps={menuProps}
        route={{ routes: navItems }}
        menuItemRender={(item, defaultDom) =>
          item.path === undefined ? defaultDom : <Link to={item.path}>{defaultDom}</Link>
        }
        // The desktop global header doesn't go through menuHeaderRender at
        // all — pro-layout renders it via this separate prop instead — so
        // the title+subtitle stack needs its own override here.
        // pro-layout's own titleDom is an <h1>; the page title is the one
        // h1 assistive tech should land on, so the brand renders as text.
        headerTitleRender={(logoDom) => (
          <Link to="/">
            {logoDom}
            <span className="app-brand-text">
              <span className="app-brand-title">Julius</span>
              <span className="app-brand-subtitle">{subtitle}</span>
            </span>
          </Link>
        )}
        menuHeaderRender={(logoDom, _title, siderProps) =>
          // Desktop keeps the brand only in the global header (mix layout's
          // default, rendered above via headerTitleRender). On mobile that
          // header collapses to just the menu button, so this renders the
          // logo + name both there and atop the nav drawer instead of
          // leaving the product with no identity.
          siderProps && !siderProps.isMobile ? (
            false
          ) : siderProps === undefined ? (
            // The mobile app bar calls this without siderProps and wraps the
            // result in its own (href-less) <a>, so a <Link> here would nest
            // one anchor inside another. Navigate by hand instead.
            <span
              className="app-mobile-brand"
              role="link"
              tabIndex={0}
              onClick={() => navigate("/")}
              onKeyDown={(event) => {
                if (event.key === "Enter") navigate("/");
              }}
            >
              {logoDom}
              <span className="app-brand-text">
                <span className="app-brand-title">Julius</span>
              </span>
            </span>
          ) : (
            <Link to="/" className="app-mobile-brand">
              {logoDom}
              <span className="app-brand-text">
                <span className="app-brand-title">Julius</span>
                <span className="app-brand-subtitle">{subtitle}</span>
              </span>
            </Link>
          )
        }
        layout="mix"
        splitMenus={false}
        footerRender={() => <BuildFooter />}
        fixedHeader
        fixSiderbar
      >
        <div id="conteudo-principal" tabIndex={-1}>
          <DemoNotice />
          <Outlet />
        </div>
      </ProLayout>
    </>
  );
}
