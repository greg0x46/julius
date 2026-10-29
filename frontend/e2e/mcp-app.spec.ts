import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, test } from "@playwright/test";
import { investmentPeriodFixture } from "../src/test/investmentWorkspaceFixtures";

for (const width of [390, 1440]) {
  test(`MCP Apps portfolio renders through the sandbox bridge at ${width}px`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width, height: 900 });
    const errors: string[] = [];
    const requests: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    page.on("request", (request) => requests.push(request.url()));
    await page.setContent('<!doctype html><html><body style="margin:0"></body></html>');
    const html = readFileSync(resolve("dist/mcp-investments.html"), "utf8");
    await page.evaluate(({ html, data }) => {
      const frame = document.createElement("iframe");
      frame.title = "Julius portfolio";
      frame.sandbox.add("allow-scripts");
      frame.style.cssText = "width:100%;height:850px;border:0";
      const methods: string[] = [];
      document.body.dataset.methods = "[]";
      window.addEventListener("message", (event) => {
        if (event.source !== frame.contentWindow) return;
        const message = event.data;
        methods.push(message.method);
        document.body.dataset.methods = JSON.stringify(methods);
        const send = (payload: object) => frame.contentWindow!.postMessage({ jsonrpc: "2.0", ...payload }, "*");
        if (message.method === "ui/initialize") send({ id: message.id, result: { protocolVersion: "2026-01-26", hostInfo: { name: "Protocol test host", version: "1" }, hostCapabilities: {}, hostContext: { theme: "light" } } });
        if (message.method === "ui/notifications/initialized") {
          send({ method: "ui/notifications/tool-input", params: { arguments: { to: data.to } } });
          send({ method: "ui/notifications/tool-result", params: { content: [{ type: "text", text: JSON.stringify(data) }], structuredContent: data } });
        }
      });
      frame.srcdoc = html;
      document.body.append(frame);
    }, { html, data: investmentPeriodFixture({ group_by: "none" }) });
    const view = page.frameLocator('iframe[title="Julius portfolio"]');
    await expect(view.getByRole("heading", { name: "Carteira de investimentos" })).toBeVisible();
    await expect(view.getByText("Tesouro Selic 2029")).toBeVisible();
    await expect(view.getByRole("button", { name: /Parcial: 1 de 4 posições/ })).toBeVisible();
    await view.getByText("Detalhes do cálculo", { exact: true }).click();
    await expect(view.getByText(/Resultado do período = mantidas/)).toBeVisible();
    await view.getByRole("switch", { name: "Mostrar encerradas" }).click();
    await expect(view.getByText("LCI Banco Antigo")).toBeVisible();
    await view.getByRole("textbox", { name: "Buscar ativo" }).fill("Tesouro");
    await expect(view.getByText("LCI Banco Antigo")).toHaveCount(0);
    await expect(view.locator(".money-size-hero")).toContainText("1.500,00");
    expect(await view.locator("html").evaluate((html) => html.scrollWidth <= html.clientWidth)).toBe(true);
    const methods = JSON.parse(await page.locator("body").getAttribute("data-methods") ?? "[]");
    expect(methods).toContain("ui/notifications/initialized");
    expect(methods).toContain("ui/notifications/size-changed");
    expect(methods.every((method: string) => method.startsWith("ui/"))).toBe(true);
    expect(requests).toEqual([]);
    expect(errors).toEqual([]);
    await page.screenshot({ path: testInfo.outputPath("mcp-portfolio.png"), fullPage: true });
  });
}
