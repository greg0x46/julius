import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { getSession } from "../api/auth";
import { getMCPSettings, revokeMCP, setMCPEnabled, setMCPScope, type MCPConnection, type MCPScope } from "../api/mcpSettings";
import { queryKeys } from "../api/queryKeys";
import { useTimezone } from "../hooks/useTimezone";
import { formatOptionalDateTime } from "../presentation/dates";
import { UnavailableState } from "./AsyncState";
import { FormField } from "./forms/FormField";
import { Section } from "./layout";
import { Alert, Button, Popconfirm, Select, Skeleton, Switch } from "./ui";

const settingsKey = ["mcp-settings"];
const permissions: { value: MCPScope; label: string }[] = [
  { value: "mcp:access", label: "Somente leitura" },
  { value: "mcp:access mcp:write", label: "Leitura e escrita" },
  { value: "mcp:access mcp:write mcp:operations", label: "Leitura, escrita e operações" },
];
const rank = (scope: MCPScope) => permissions.findIndex((item) => item.value === scope);
const label = (scope: MCPScope) => permissions.find((item) => item.value === scope)?.label ?? scope;

export function MCPSettings() {
  const client = useQueryClient();
  const session = useQuery({ queryKey: queryKeys.authSession, queryFn: ({ signal }) => getSession(signal) });
  const allowed = session.data?.authenticated && session.data.authentication_enabled && !session.data.demo;
  const settings = useQuery({ queryKey: settingsKey, queryFn: ({ signal }) => getMCPSettings(signal), enabled: Boolean(allowed) });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState("");
  const reload = async () => { await client.invalidateQueries({ queryKey: settingsKey }); };
  const toggle = async (enabled: boolean) => {
    setSaving(true); setError(null); setNotice("");
    try {
      await setMCPEnabled(enabled);
      await reload();
      setNotice(enabled ? "Acesso MCP ativado." : "Acesso MCP desativado.");
    } catch (error) {
      setError(error instanceof Error ? error.message : "Não foi possível alterar o acesso MCP.");
    } finally { setSaving(false); }
  };
  return (
    <Section title="Clientes MCP">
      {session.isLoading ? <Skeleton active /> : session.isError ? (
        <UnavailableState onRetry={() => void session.refetch()}>Não foi possível consultar o acesso agora.</UnavailableState>
      ) : !allowed ? (
        <p className="settings-radio-hint">{session.data?.demo ? "O acesso MCP não está disponível na demonstração." : "Ative a autenticação e entre na conta para gerenciar clientes MCP."}</p>
      ) : settings.isLoading ? <Skeleton active /> : settings.isError || !settings.data ? (
        <UnavailableState onRetry={() => void settings.refetch()}>Não foi possível carregar os clientes MCP.</UnavailableState>
      ) : (
        <div className="settings-form">
          {settings.data.available ? (
            <FormField label="Permitir acesso MCP" htmlFor="mcp-enabled" hint="Clientes autorizados podem acessar os dados conforme suas permissões. Desativar bloqueia novas chamadas imediatamente.">
              <div><Switch id="mcp-enabled" checked={settings.data.enabled} disabled={saving} onChange={(value) => void toggle(value)} aria-describedby="mcp-enabled-hint" /></div>
            </FormField>
          ) : <Alert type="info" showIcon message="MCP desativado nesta instância" description="O acesso por clientes MCP depende da configuração do servidor." />}
          {error && <Alert type="error" showIcon message={error} />}
          <div role="status" aria-live="polite">{notice}</div>
          <p className="settings-radio-hint">Somente leitura permite consultar dados. Leitura e escrita também permite solicitar alterações, e operações inclui sincronizações e atualização de cotações.</p>
          {settings.data.connections.length === 0 ? <p>Nenhum cliente MCP autorizado.</p> : (
            <ul className="mcp-connections" aria-label="Clientes MCP autorizados">
              {settings.data.connections.map((connection) => <Connection key={`${connection.id}:${connection.scope}:${connection.revoked_at}`} connection={connection} onSaved={async (message) => { await reload(); setNotice(message); }} />)}
            </ul>
          )}
          <p className="settings-radio-hint">Alterar ou redefinir a senha revoga todos os acessos MCP. Sair encerra apenas a sessão do navegador. Para ampliar as permissões além do consentimento original, autorize o cliente novamente.</p>
        </div>
      )}
    </Section>
  );
}

function Connection({ connection, onSaved }: { connection: MCPConnection; onSaved: (message: string) => Promise<void> }) {
  const [scope, setScope] = useState<MCPScope>(connection.scope);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const timezone = useTimezone();
  const name = connection.client_name || "Aplicativo sem nome";
  const active = !connection.revoked_at && !connection.expired;
  const editable = rank(connection.approved_scope) > 0;
  const run = async (action: () => Promise<void>, message: string) => {
    setBusy(true); setError(null);
    try { await action(); await onSaved(message); }
    catch (error) { setError(error instanceof Error ? error.message : "Não foi possível alterar este cliente."); }
    finally { setBusy(false); }
  };
  return (
    <li className="mcp-connection">
      <strong>{name}</strong>
      <span className="settings-radio-hint">Identificador: {connection.client_id}</span>
      <span className="settings-radio-hint">Autorizado em: {formatOptionalDateTime(connection.created_at, timezone)}</span>
      <span className="settings-radio-hint">Último uso: {connection.last_used_at ? formatOptionalDateTime(connection.last_used_at, timezone) : "Ainda não utilizado"}</span>
      <span className="settings-radio-hint">Expira em: {formatOptionalDateTime(connection.expires_at, timezone)}</span>
      {connection.revoked_at ? <span>Revogado em {formatOptionalDateTime(connection.revoked_at, timezone)}</span> : connection.expired ? <span>Expirado</span> : null}
      {active && editable ? (
        <FormField label={`Permissões de ${name}`} htmlFor={`mcp-scope-${connection.id}`}>
          <Select id={`mcp-scope-${connection.id}`} value={scope} options={permissions.filter((item) => rank(item.value) <= rank(connection.approved_scope))} disabled={busy} onChange={(value: MCPScope) => setScope(value)} />
        </FormField>
      ) : <span>Permissões: {label(connection.scope)}</span>}
      {error && <Alert type="error" showIcon message={error} />}
      {active && <div className="settings-account-actions">
        {editable && <Button disabled={busy || scope === connection.scope} loading={busy} onClick={() => void run(() => setMCPScope(connection.id, scope), "Permissões atualizadas.")}>Salvar permissões</Button>}
        <Popconfirm title={`Revogar o acesso de ${name}?`} description="Este cliente deixará de acessar seus dados imediatamente." okText="Revogar acesso" cancelText="Cancelar" onConfirm={() => run(() => revokeMCP(connection.id), "Acesso revogado.")}>
          <Button danger disabled={busy} aria-label={`Revogar acesso de ${name}`}>Revogar acesso</Button>
        </Popconfirm>
      </div>}
    </li>
  );
}
