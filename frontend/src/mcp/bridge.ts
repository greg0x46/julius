import { parseInvestmentPeriod } from "../api/contracts/investmentPeriod";
import type { PortfolioState } from "./PortfolioApp";

const version = "2026-01-26";

// The parent is the host (or its sandbox proxy); no credentials enter the view.
export function connectPortfolioHost(update: (state: PortfolioState) => void): () => void {
  let ready = false;
  const send = (message: object) => window.parent.postMessage({ jsonrpc: "2.0", ...message }, "*");
  const theme = (context: { theme?: unknown } | undefined) => {
    if (context?.theme === "light" || context?.theme === "dark") document.documentElement.dataset.theme = context.theme;
  };
  const size = () => {
    if (ready) send({ method: "ui/notifications/size-changed", params: { height: document.body.scrollHeight, width: document.documentElement.clientWidth } });
  };
  const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(size);
  const timer = window.setTimeout(() => update({ error: true }), 15_000);
  const receive = (event: MessageEvent) => {
    if (event.source !== window.parent || event.data?.jsonrpc !== "2.0") return;
    const message = event.data;
    if (message.id === "julius-init" && !ready) {
      window.clearTimeout(timer);
      if (message.error || message.result?.protocolVersion !== version) { update({ error: true }); return; }
      ready = true;
      theme(message.result.hostContext);
      send({ method: "ui/notifications/initialized", params: {} });
      observer?.observe(document.body);
      size();
    }
    if (!ready) return;
    switch (message.method) {
      case "ui/notifications/tool-result":
        try {
          if (message.params?.isError) throw new Error("Tool failed");
          update({ data: parseInvestmentPeriod(message.params?.structuredContent) });
        } catch { update({ error: true }); }
        break;
      case "ui/notifications/tool-cancelled": update({ error: true }); break;
      case "ui/notifications/host-context-changed": theme(message.params); break;
      case "ping": send({ id: message.id, result: {} }); break;
      case "ui/resource-teardown":
        cleanup();
        send({ id: message.id, result: {} });
        break;
    }
  };
  const cleanup = () => {
    ready = false;
    window.clearTimeout(timer);
    observer?.disconnect();
    window.removeEventListener("message", receive);
  };
  window.addEventListener("message", receive);
  send({ id: "julius-init", method: "ui/initialize", params: { appInfo: { name: "Julius portfolio", version: "1" }, appCapabilities: { availableDisplayModes: ["inline"] }, protocolVersion: version } });
  return cleanup;
}
