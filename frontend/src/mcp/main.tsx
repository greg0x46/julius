import { createRoot } from "react-dom/client";
import { PortfolioApp } from "./PortfolioApp";
import { connectPortfolioHost } from "./bridge";
import ui from "../styles/ui.css?inline";
import global from "../styles/global.css?inline";
import layout from "../styles/layout.css?inline";
import investments from "../styles/investments.css?inline";
import app from "./portfolio.css?inline";

const style = document.createElement("style");
style.textContent = [ui, global, layout, investments, app].join("\n");
document.head.append(style);
const root = createRoot(document.getElementById("root")!);
root.render(<PortfolioApp state={null} />);
connectPortfolioHost((state) => root.render(<PortfolioApp state={state} />));
