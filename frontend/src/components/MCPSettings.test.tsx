import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import * as authApi from "../api/auth";
import * as api from "../api/mcpSettings";
import type { MCPConnection, MCPSettings as Settings } from "../api/mcpSettings";
import { MCPSettings } from "./MCPSettings";

vi.mock("../api/auth");
vi.mock("../api/mcpSettings");
const connection = (overrides: Partial<MCPConnection> = {}): MCPConnection => ({
  id: "a", client_id: "synthetic-client", client_name: "Assistente", scope: "mcp:access mcp:write", approved_scope: "mcp:access mcp:write",
  created_at: "2026-10-27T10:00:00Z", last_used_at: null, expires_at: "2026-10-27T11:05:00Z", revoked_at: null, expired: false, ...overrides,
});
let state: Settings;
beforeEach(() => {
  vi.resetAllMocks();
  state = { available: true, enabled: true, connections: [connection()] };
  vi.mocked(authApi.getSession).mockResolvedValue({ authenticated: true, authentication_enabled: true });
  vi.mocked(api.getMCPSettings).mockImplementation(async () => structuredClone(state));
  vi.mocked(api.setMCPScope).mockImplementation(async (id, scope) => { state.connections = state.connections.map((item) => item.id === id ? { ...item, scope } : item); });
  vi.mocked(api.revokeMCP).mockImplementation(async (id) => { state.connections = state.connections.map((item) => item.id === id ? { ...item, revoked_at: "2026-10-27T10:30:00Z" } : item); });
  vi.mocked(api.setMCPEnabled).mockImplementation(async (enabled) => { state.enabled = enabled; });
});
function show() {
  return render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><MCPSettings /></QueryClientProvider>);
}
it("shows client identity and dates and limits permissions before saving", async () => {
  show();
  expect(await screen.findByText("Assistente")).toBeInTheDocument();
  expect(screen.getByText("Identificador: synthetic-client")).toBeInTheDocument();
  expect(screen.getByText(/Último uso: Ainda não utilizado/)).toBeInTheDocument();
  expect(screen.getByText(/Autorizado em:/)).toBeInTheDocument();
  expect(screen.getByText(/Expira em:/)).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Salvar permissões" })).toBeDisabled();
  await userEvent.click(screen.getByRole("combobox", { name: "Permissões de Assistente" }));
  await userEvent.click(screen.getByRole("option", { name: "Somente leitura" }));
  await userEvent.click(screen.getByRole("button", { name: "Salvar permissões" }));
  await waitFor(() => expect(api.setMCPScope).toHaveBeenCalledWith("a", "mcp:access"));
  expect(await screen.findByText("Permissões atualizadas.")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Salvar permissões" })).toBeDisabled();
});
it("confirms revocation for one client and retains the other", async () => {
  state.connections.push(connection({ id: "b", client_id: "other-client", client_name: "Outro cliente", scope: "mcp:access", approved_scope: "mcp:access" }));
  show();
  await userEvent.click(await screen.findByRole("button", { name: "Revogar acesso de Assistente" }));
  const dialog = screen.getByRole("dialog");
  expect(within(dialog).getByText("Revogar o acesso de Assistente?")).toBeInTheDocument();
  await userEvent.click(within(dialog).getByRole("button", { name: "Revogar acesso" }));
  expect(await screen.findByText(/Revogado em/)).toBeInTheDocument();
  expect(api.revokeMCP).toHaveBeenCalledWith("a");
  expect(screen.getByRole("button", { name: "Revogar acesso de Outro cliente" })).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Revogar acesso de Assistente" })).not.toBeInTheDocument();
  expect(screen.getByText("Acesso revogado.")).toBeInTheDocument();
});
it("keeps connections visible when MCP is disabled and handles errors without losing controls", async () => {
  state.enabled = false;
  vi.mocked(api.revokeMCP).mockRejectedValue(new Error("Tente novamente em instantes."));
  show();
  const button = await screen.findByRole("button", { name: "Revogar acesso de Assistente" });
  await userEvent.click(button);
  await userEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Revogar acesso" }));
  expect(await screen.findByText("Tente novamente em instantes.")).toBeInTheDocument();
  expect(button).toBeEnabled();
});
it("changes the runtime switch independently of browser authentication", async () => {
  show();
  await userEvent.click(await screen.findByRole("switch", { name: "Permitir acesso MCP" }));
  expect(await screen.findByText("Acesso MCP desativado.")).toBeInTheDocument();
  expect(api.setMCPEnabled).toHaveBeenCalledWith(false);
  expect(screen.getByText("Assistente")).toBeInTheDocument();
});
it("shows empty and server-disabled states", async () => {
  state.available = false; state.connections = [];
  show();
  expect(await screen.findByText("MCP desativado nesta instância")).toBeInTheDocument();
  expect(screen.getByText("Nenhum cliente MCP autorizado.")).toBeInTheDocument();
  expect(screen.queryByRole("switch")).not.toBeInTheDocument();
});
it("does not offer editing for expired, revoked or read-only consent", async () => {
  state.connections = [connection({ expired: true }), connection({ id: "b", revoked_at: "2026-10-27T10:30:00Z" }), connection({ id: "c", scope: "mcp:access", approved_scope: "mcp:access" })];
  show();
  expect(await screen.findByText("Expirado")).toBeInTheDocument();
  expect(screen.getByText(/Revogado em/)).toBeInTheDocument();
  expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
  expect(screen.getAllByRole("button", { name: "Revogar acesso de Assistente" })).toHaveLength(1);
});
it("retries a failed load", async () => {
  vi.mocked(api.getMCPSettings).mockRejectedValueOnce(new Error("unavailable"));
  show();
  await userEvent.click(await screen.findByRole("button", { name: "Tentar novamente" }));
  expect(await screen.findByText("Assistente")).toBeInTheDocument();
});
it.each([
  { authenticated: false, authentication_enabled: false, demo: true },
  { authenticated: false, authentication_enabled: false },
])("does not query connections in demo or open mode: %o", async (session) => {
  vi.mocked(authApi.getSession).mockResolvedValue(session);
  show();
  expect(await screen.findByText(session.demo ? "O acesso MCP não está disponível na demonstração." : "Ative a autenticação e entre na conta para gerenciar clientes MCP.")).toBeInTheDocument();
  expect(api.getMCPSettings).not.toHaveBeenCalled();
});
it("shows loading state without action controls", () => {
  vi.mocked(authApi.getSession).mockReturnValue(new Promise(() => {}));
  const { container } = show();
  expect(container.querySelector(".ui-skeleton")).toBeInTheDocument();
  expect(screen.queryByRole("switch")).not.toBeInTheDocument();
});
