import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { investmentPeriodFixture } from "../test/investmentWorkspaceFixtures";
import { PortfolioApp } from "./PortfolioApp";
import { connectPortfolioHost } from "./bridge";

const host = (message: object, source: Window | null = window) => act(() => {
  window.dispatchEvent(new MessageEvent("message", { source, data: { jsonrpc: "2.0", ...message } }));
});
const initialize = () => host({ id: "julius-init", result: { protocolVersion: "2026-01-26", hostContext: { theme: "dark" } } });
afterEach(() => { delete document.documentElement.dataset.theme; vi.useRealTimers(); });

it("uses the shared portfolio view and only filters visible rows", async () => {
  const user = userEvent.setup();
  const { container } = render(<PortfolioApp state={{ data: investmentPeriodFixture({ group_by: "none" }) }} />);
  const hero = container.querySelector(".money-size-hero")!;
  expect(hero).toHaveTextContent("1.500,00");
  expect(screen.queryByText("LCI Banco Antigo")).not.toBeInTheDocument();
  await user.click(screen.getByRole("switch", { name: "Mostrar encerradas" }));
  expect(screen.getByText("LCI Banco Antigo")).toBeVisible();
  await user.type(screen.getByRole("textbox", { name: "Buscar ativo" }), "Tesouro");
  expect(screen.getByText("Tesouro Selic 2029")).toBeVisible();
  expect(screen.queryByText("LCI Banco Antigo")).not.toBeInTheDocument();
  expect(hero).toHaveTextContent("1.500,00");
  expect(screen.queryByRole("link")).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: /nova|editar|excluir/i })).not.toBeInTheDocument();
});

it("negotiates the portable bridge and validates shared structured results", () => {
  const send = vi.spyOn(window, "postMessage").mockImplementation(() => {});
  const { rerender } = render(<PortfolioApp state={null} />);
  const close = connectPortfolioHost((state) => rerender(<PortfolioApp state={state} />));
  try {
    expect(send).toHaveBeenCalledWith(expect.objectContaining({ method: "ui/initialize", params: expect.objectContaining({ appInfo: expect.any(Object), protocolVersion: "2026-01-26" }) }), "*");
    const result = { method: "ui/notifications/tool-result", params: { structuredContent: investmentPeriodFixture({ group_by: "none" }) } };
    host(result);
    expect(screen.getByRole("status")).toHaveTextContent("Aguardando");
    initialize();
    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(send).toHaveBeenCalledWith(expect.objectContaining({ method: "ui/notifications/initialized" }), "*");
    host(result, null);
    expect(screen.getByRole("status")).toHaveTextContent("Aguardando");
    host(result);
    expect(screen.getByText("Tesouro Selic 2029")).toBeVisible();
    expect(screen.getByRole("button", { name: /Parcial: 1 de 4 posições/ })).toBeVisible();
    host({ method: "ui/notifications/tool-result", params: { structuredContent: { totals: {} } } });
    expect(screen.getByText("Não foi possível exibir a carteira.")).toBeVisible();
    expect(screen.queryByText("Tesouro Selic 2029")).not.toBeInTheDocument();
    host(result);
    host({ method: "ui/notifications/tool-result", params: { isError: true } });
    expect(screen.getByText("Não foi possível exibir a carteira.")).toBeVisible();
    host({ id: 42, method: "ui/resource-teardown", params: {} });
    expect(send).toHaveBeenCalledWith({ jsonrpc: "2.0", id: 42, result: {} }, "*");
    host(result);
    expect(screen.queryByText("Tesouro Selic 2029")).not.toBeInTheDocument();
    expect(send.mock.calls.some(([message]) => message.method === "tools/call")).toBe(false);
  } finally { close(); }
});

it("shows empty portfolios without creation actions", () => {
  render(<PortfolioApp state={{ data: investmentPeriodFixture({ groups: [] }) }} />);
  expect(screen.getByText("Nenhuma posição neste período")).toBeVisible();
  expect(screen.queryByRole("button", { name: "Nova posição" })).not.toBeInTheDocument();
});

it("leaves a readable error when initialization times out", () => {
  vi.useFakeTimers();
  const update = vi.fn();
  const close = connectPortfolioHost(update);
  try {
    vi.advanceTimersByTime(15_000);
    expect(update).toHaveBeenCalledWith({ error: true });
  } finally { close(); }
});
