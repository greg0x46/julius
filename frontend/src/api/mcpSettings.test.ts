import { expect, it, vi } from "vitest";
import { getMCPSettings, parseMCPSettings, revokeMCP, setMCPEnabled, setMCPScope } from "./mcpSettings";
import { apiFetch } from "./transport";
vi.mock("./transport");

it("rejects malformed permissions, dates and unexpected credential fields", () => {
  const connection = { id: "a", client_id: "client", client_name: "Synthetic", scope: "mcp:read", approved_scope: "mcp:read", created_at: "2026-10-27T10:00:00Z", last_used_at: null, expires_at: "2026-10-27T11:00:00Z", revoked_at: null, expired: false };
  for (const change of [{ scope: "mcp:access" }, { created_at: "invalid" }, { scope: "mcp:read mcp:write" }, { token_hash: "secret" }]) {
    expect(() => parseMCPSettings({ available: true, enabled: true, connections: [{ ...connection, ...change }] })).toThrow();
  }
  expect(parseMCPSettings({ available: false, enabled: false, connections: [connection] }).connections).toHaveLength(1);
});
it("uses the central transport for settings and mutations", async () => {
  vi.mocked(apiFetch).mockImplementation(async () => new Response(JSON.stringify({ saved: true }), { status: 200 }));
  await setMCPEnabled(false); await setMCPScope("client/id", "mcp:read"); await revokeMCP("client/id");
  expect(apiFetch).toHaveBeenCalledWith("/api/settings/mcp", expect.objectContaining({ method: "PUT", body: '{"enabled":false}' }));
  expect(apiFetch).toHaveBeenCalledWith("/api/settings/mcp/connections/client%2Fid", expect.objectContaining({ method: "PATCH" }));
  expect(apiFetch).toHaveBeenCalledWith("/api/settings/mcp/connections/client%2Fid", expect.objectContaining({ method: "DELETE" }));
  vi.mocked(apiFetch).mockResolvedValueOnce(new Response(JSON.stringify({ title: "Indisponível" }), { status: 503 }));
  await expect(getMCPSettings()).rejects.toThrow("Indisponível");
});
