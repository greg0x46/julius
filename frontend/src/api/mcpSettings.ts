import { apiMessage } from "../i18n/apiMessage";
import { apiFetch } from "./transport";
import { isValidDate, requiredRecord } from "./contracts/shared";

export type MCPScope = "mcp:access" | "mcp:access mcp:write" | "mcp:access mcp:write mcp:operations";
const scopeRank: Record<MCPScope, number> = { "mcp:access": 0, "mcp:access mcp:write": 1, "mcp:access mcp:write mcp:operations": 2 };
export interface MCPConnection {
  id: string;
  client_id: string;
  client_name: string;
  scope: MCPScope;
  approved_scope: MCPScope;
  created_at: string;
  last_used_at: string | null;
  expires_at: string;
  revoked_at: string | null;
  expired: boolean;
}
export interface MCPSettings {
  available: boolean;
  enabled: boolean;
  connections: MCPConnection[];
}
const invalid = () => apiMessage("apiMcpInvalid");
function isScope(value: unknown): value is MCPScope {
  return typeof value === "string" && Object.hasOwn(scopeRank, value);
}
export function parseMCPSettings(value: unknown): MCPSettings {
  const record = requiredRecord(value, ["available", "enabled", "connections"], invalid());
  if (typeof record.available !== "boolean" || typeof record.enabled !== "boolean" || !Array.isArray(record.connections)) {
    throw new TypeError(invalid());
  }
  const connections = record.connections.map((value): MCPConnection => {
    const item = requiredRecord(value, ["id", "client_id", "client_name", "scope", "approved_scope", "created_at", "last_used_at", "expires_at", "revoked_at", "expired"], invalid());
    const { id, client_id, client_name, scope, approved_scope, created_at, last_used_at, expires_at, revoked_at, expired } = item;
    if (typeof id !== "string" || typeof client_id !== "string" || typeof client_name !== "string" ||
      !isScope(scope) || !isScope(approved_scope) || (scope !== "mcp:access" && approved_scope !== scope && !(scope === "mcp:access mcp:write" && approved_scope === "mcp:access mcp:write mcp:operations")) ||
      !isValidDate(created_at) || !isValidDate(expires_at) ||
      (last_used_at !== null && !isValidDate(last_used_at)) || (revoked_at !== null && !isValidDate(revoked_at)) || typeof expired !== "boolean") {
      throw new TypeError(invalid());
    }
    return { id, client_id, client_name, scope, approved_scope, created_at, last_used_at, expires_at, revoked_at, expired };
  });
  return { available: record.available, enabled: record.enabled, connections };
}
async function send(path: string, method: string, body?: unknown, signal?: AbortSignal): Promise<unknown> {
  const response = await apiFetch(path, { method, signal, headers: { "Content-Type": "application/json" }, ...(body === undefined ? {} : { body: JSON.stringify(body) }) });
  if (!response.ok) {
    const problem = await response.json().catch(() => null) as { detail?: string; title?: string } | null;
    throw new Error(problem?.detail || problem?.title || apiMessage("mcpChangeError"));
  }
  return response.json();
}
export const getMCPSettings = async (signal?: AbortSignal) => parseMCPSettings(await send("/api/settings/mcp", "GET", undefined, signal));
export const setMCPEnabled = async (enabled: boolean) => { await send("/api/settings/mcp", "PUT", { enabled }); };
export const setMCPScope = async (id: string, scope: MCPScope) => { await send(`/api/settings/mcp/connections/${encodeURIComponent(id)}`, "PATCH", { scope }); };
export const revokeMCP = async (id: string) => { await send(`/api/settings/mcp/connections/${encodeURIComponent(id)}`, "DELETE"); };
