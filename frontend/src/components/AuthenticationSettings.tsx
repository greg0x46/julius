import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Alert, Button, Input, Skeleton } from "antd";
import { useState } from "react";

import { changePassword, getSession, logout, setAuthenticationEnabled } from "../api/auth";
import { UnavailableState } from "./AsyncState";
import { FormField } from "./forms/FormField";
import { Section } from "./layout";
import { queryKeys } from "../api/queryKeys";

export function AuthenticationSettings() {
  const client = useQueryClient();
  const { data: session, isLoading, isError, refetch } = useQuery({
    queryKey: queryKeys.authSession,
    queryFn: ({ signal }) => getSession(signal),
  });
  const [current, setCurrent] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const run = async (action: () => Promise<void>) => {
    setBusy(true);
    setError(null);
    try {
      await action();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Não foi possível concluir.");
    } finally {
      setBusy(false);
    }
  };
  const toggleAuthentication = async (enabled: boolean) => {
    setBusy(true);
    setError(null);
    try {
      const next = await setAuthenticationEnabled(enabled);
      client.setQueryData(queryKeys.authSession, next);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Não foi possível alterar a autenticação.");
    } finally {
      setBusy(false);
    }
  };
  if (isError && !session)
    return (
      <Section title="Acesso">
        <UnavailableState onRetry={() => void refetch()}>Não foi possível consultar o acesso agora.</UnavailableState>
      </Section>
    );
  if (isLoading || !session)
    return (
      <Section title="Acesso">
        <Skeleton active />
      </Section>
    );
  if (session.demo)
    return (
      <Section title="Acesso">
        <Alert
          type="info"
          showIcon
          message="Ambiente de demonstração"
          description="O acesso é aberto e não pode ser alterado aqui. Os dados são fictícios e redefinidos todos os dias."
        />
      </Section>
    );
  if (!session.authentication_enabled)
    return (
      <Section title="Acesso">
        <div className="settings-form">
          {error && <Alert type="error" message={error} showIcon />}
          <Alert
            type="warning"
            showIcon
            message="Autenticação desativada"
            description="Qualquer pessoa com acesso a esta instância pode consultar e alterar os dados."
          />
          <Button type="primary" onClick={() => void toggleAuthentication(true)} loading={busy}>
            Ativar autenticação
          </Button>
        </div>
      </Section>
    );
  return (
    <Section title="Acesso">
      <form
        className="settings-form"
        onSubmit={(event) => {
          event.preventDefault();
          if (password !== confirmation) {
            setError("As senhas não coincidem.");
            return;
          }
          void run(() => changePassword(current, password));
        }}
      >
        {error && <Alert type="error" message={error} showIcon />}
        <p className="settings-radio-hint">A autenticação está ativada. Somente sessões válidas acessam os dados.</p>
        <FormField label="Senha atual" htmlFor="current-password">
          <Input.Password
            id="current-password"
            autoComplete="current-password"
            required
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
          />
        </FormField>
        <FormField label="Nova senha" htmlFor="new-password">
          <Input.Password
            id="new-password"
            autoComplete="new-password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </FormField>
        <FormField label="Repita a nova senha" htmlFor="confirm-password">
          <Input.Password
            id="confirm-password"
            autoComplete="new-password"
            required
            value={confirmation}
            onChange={(e) => setConfirmation(e.target.value)}
          />
        </FormField>
        <Button type="primary" htmlType="submit" loading={busy}>
          Alterar senha e encerrar sessões
        </Button>
        <div className="settings-account-actions">
          <Button onClick={() => void run(logout)} loading={busy}>
            Sair
          </Button>
          <Button danger onClick={() => void toggleAuthentication(false)} loading={busy}>
            Desativar autenticação
          </Button>
        </div>
      </form>
    </Section>
  );
}
