import { useTranslation } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { getSession } from "../api/auth";
import { getMCPSettings, revokeMCP, setMCPEnabled, setMCPScope, type MCPConnection, type MCPScope } from "../api/mcpSettings";
import { queryKeys } from "../api/queryKeys";
import { formatOptionalDateTime } from "../presentation/dates";
import { UnavailableState } from "./AsyncState";
import { FormField } from "./forms/FormField";
import { Section } from "./layout";
import { Alert, Button, Popconfirm, Select, Skeleton, Switch } from "./ui";

const settingsKey = ["mcp-settings"];
const permissions: { value: MCPScope; label: MessageKey }[] = [
  { value: "mcp:access", label: "mcpReadOnly" },
  { value: "mcp:access mcp:write", label: "mcpReadWrite" },
  { value: "mcp:access mcp:write mcp:operations", label: "mcpOperations" },
];
const rank = (scope: MCPScope) => permissions.findIndex((item) => item.value === scope);


export function MCPSettings() {
  const { t } = useTranslation();
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
      setNotice(enabled ? t("mcpEnabled") : t("mcpDisabled"));
    } catch (error) {
      setError(error instanceof Error ? error.message : t("mcpChangeError"));
    } finally { setSaving(false); }
  };
  return (
    <Section title={t("mcpClients")}>
      {session.isLoading ? <Skeleton active /> : session.isError ? (
        <UnavailableState onRetry={() => void session.refetch()}>{t("accessLoadError")}</UnavailableState>
      ) : !allowed ? (
        <p className="settings-radio-hint">{session.data?.demo ? t("mcpDemo") : t("mcpSignIn")}</p>
      ) : settings.isLoading ? <Skeleton active /> : settings.isError || !settings.data ? (
        <UnavailableState onRetry={() => void settings.refetch()}>{t("mcpLoadError")}</UnavailableState>
      ) : (
        <div className="settings-form">
          {settings.data.available ? (
            <FormField label={t("mcpAllow")} htmlFor="mcp-enabled" hint={t("mcpAllowHint")}>
              <div><Switch id="mcp-enabled" checked={settings.data.enabled} disabled={saving} onChange={(value) => void toggle(value)} aria-describedby="mcp-enabled-hint" /></div>
            </FormField>
          ) : <Alert type="info" showIcon message={t("mcpInstanceDisabled")} description={t("mcpInstanceHint")} />}
          {error && <Alert type="error" showIcon message={error} />}
          <div role="status" aria-live="polite">{notice}</div>
          <p className="settings-radio-hint">{t("mcpScopeHint")}</p>
          {settings.data.connections.length === 0 ? <p>{t("mcpNoClients")}</p> : (
            <ul className="mcp-connections" aria-label={t("mcpAuthorizedClients")}>
              {settings.data.connections.map((connection) => <Connection key={`${connection.id}:${connection.scope}:${connection.revoked_at}`} connection={connection} onSaved={async (message) => { await reload(); setNotice(message); }} />)}
            </ul>
          )}
          <p className="settings-radio-hint">{t("mcpPasswordHint")}</p>
        </div>
      )}
    </Section>
  );
}

function Connection({ connection, onSaved }: { connection: MCPConnection; onSaved: (message: string) => Promise<void> }) {
  const { t, language } = useTranslation();
  const [scope, setScope] = useState<MCPScope>(connection.scope);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const name = connection.client_name || t("unnamedApplication");
  const active = !connection.revoked_at && !connection.expired;
  const editable = rank(connection.approved_scope) > 0;
  const scopeLabel = permissions.find((item) => item.value === connection.scope)?.label;
  const run = async (action: () => Promise<void>, message: string) => {
    setBusy(true); setError(null);
    try { await action(); await onSaved(message); }
    catch (error) { setError(error instanceof Error ? error.message : t("mcpClientError")); }
    finally { setBusy(false); }
  };
  return (
    <li className="mcp-connection">
      <strong>{name}</strong>
      <span className="settings-radio-hint">{t("identifier")}:{" "}{connection.client_id}</span>
      <span className="settings-radio-hint">{t("authorizedAt")}:{" "}{formatOptionalDateTime(connection.created_at, language)}</span>
      <span className="settings-radio-hint">{t("lastUsed")}:{" "}{connection.last_used_at ? formatOptionalDateTime(connection.last_used_at, language) : t("neverUsed")}</span>
      <span className="settings-radio-hint">{t("expiresAt")}:{" "}{formatOptionalDateTime(connection.expires_at, language)}</span>
      {connection.revoked_at ? <span>{t("revokedAt")}{" "}{formatOptionalDateTime(connection.revoked_at, language)}</span> : connection.expired ? <span>{t("expired")}</span> : null}
      {active && editable ? (
        <FormField label={t("permissionsFor", { name: name })} htmlFor={`mcp-scope-${connection.id}`}>
          <Select id={`mcp-scope-${connection.id}`} value={scope} options={permissions.filter((item) => rank(item.value) <= rank(connection.approved_scope)).map((item) => ({ ...item, label: t(item.label) }))} disabled={busy} onChange={(value: MCPScope) => setScope(value)} />
        </FormField>
      ) : <span>{t("permissions")}:{" "}{scopeLabel ? t(scopeLabel) : connection.scope}</span>}
      {error && <Alert type="error" showIcon message={error} />}
      {active && <div className="settings-account-actions">
        {editable && <Button disabled={busy || scope === connection.scope} loading={busy} onClick={() => void run(() => setMCPScope(connection.id, scope), t("permissionsUpdated"))}>{t("savePermissions")}</Button>}
        <Popconfirm title={t("revokeAccessQuestion", { name: name })} description={t("revokeDescription")} okText={t("revokeAccess")} cancelText={t("cancel")} onConfirm={() => run(() => revokeMCP(connection.id), t("accessRevoked"))}>
          <Button danger disabled={busy} aria-label={t("revokeAccessFor", { name: name })}>{t("revokeAccess")}</Button>
        </Popconfirm>
      </div>}
    </li>
  );
}
