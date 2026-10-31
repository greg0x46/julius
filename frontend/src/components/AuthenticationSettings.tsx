import { useTranslation } from "../i18n/context";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Alert, Button, Input, Skeleton } from "./ui";
import { useState } from "react";

import { authErrorMessage, changePassword, getSession, logout, setAuthenticationEnabled } from "../api/auth";
import { UnavailableState } from "./AsyncState";
import { FormField } from "./forms/FormField";
import { Section } from "./layout";
import { queryKeys } from "../api/queryKeys";

export function AuthenticationSettings() {
  const { t, language } = useTranslation();
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
      setError(authErrorMessage(err, language, t("unableToComplete")));
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
      setError(authErrorMessage(err, language, t("authenticationChangeError")));
    } finally {
      setBusy(false);
    }
  };
  if (isError && !session)
    return (
      <Section title={t("access")}>
        <UnavailableState onRetry={() => void refetch()}>{t("accessLoadError")}</UnavailableState>
      </Section>
    );
  if (isLoading || !session)
    return (
      <Section title={t("access")}>
        <Skeleton active />
      </Section>
    );
  if (session.demo)
    return (
      <Section title={t("access")}>
        <Alert
          type="info"
          showIcon
          message={t("demoEnvironment")}
          description={t("demoAccessHint")}
        />
      </Section>
    );
  if (!session.authentication_enabled)
    return (
      <Section title={t("access")}>
        <div className="settings-form">
          {error && <Alert type="error" message={error} showIcon />}
          <Alert
            type="warning"
            showIcon
            message={t("authenticationDisabled")}
            description={t("authenticationDisabledHint")}
          />
          <Button type="primary" onClick={() => void toggleAuthentication(true)} loading={busy}>
            {t("enableAuthentication")}
          </Button>
        </div>
      </Section>
    );
  return (
    <Section title={t("access")}>
      <form
        className="settings-form"
        onSubmit={(event) => {
          event.preventDefault();
          if (password !== confirmation) {
            setError(t("passwordMismatch"));
            return;
          }
          void run(() => changePassword(current, password));
        }}
      >
        {error && <Alert type="error" message={error} showIcon />}
        <p className="settings-radio-hint">{t("authenticationEnabledHint")}</p>
        <FormField label={t("currentPassword")} htmlFor="current-password">
          <Input.Password
            id="current-password"
            autoComplete="current-password"
            required
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
          />
        </FormField>
        <FormField label={t("newPassword")} htmlFor="new-password">
          <Input.Password
            id="new-password"
            autoComplete="new-password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </FormField>
        <FormField label={t("repeatNewPassword")} htmlFor="confirm-password">
          <Input.Password
            id="confirm-password"
            autoComplete="new-password"
            required
            value={confirmation}
            onChange={(e) => setConfirmation(e.target.value)}
          />
        </FormField>
        <Button type="primary" htmlType="submit" loading={busy}>
          {t("changePassword")}
        </Button>
        <div className="settings-account-actions">
          <Button onClick={() => void run(logout)} loading={busy}>
            {t("signOut")}
          </Button>
          <Button danger onClick={() => void toggleAuthentication(false)} loading={busy}>
            {t("disableAuthentication")}
          </Button>
        </div>
      </form>
    </Section>
  );
}
