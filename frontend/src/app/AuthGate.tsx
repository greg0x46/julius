import { useTranslation } from "../i18n/context";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, type ReactNode } from "react";
import { getSession } from "../api/auth";
import { expireSession, onSessionExpired } from "../api/transport";
import { UnavailableState } from "../components/AsyncState";
import { LoginPage } from "../pages/LoginPage";
import { AppLoadingScreen } from "./AppLoadingScreen";
import { invalidateAuthSession, queryKeys } from "../api/queryKeys";

export function AuthGate({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const wasAuthenticated = useRef(false);
  const { data, isLoading, refetch } = useQuery({
    queryKey: queryKeys.authSession, queryFn: ({ signal }) => getSession(signal),
    retry: false, refetchInterval: 60_000,
  });
  useEffect(() => onSessionExpired(() => {
    void client.cancelQueries();
    wasAuthenticated.current = false;
    client.removeQueries({ predicate: (query) => query.queryKey[0] !== queryKeys.authSession[0] });
    client.getMutationCache().clear();
    client.setQueryData(queryKeys.authSession, { authenticated: false, authentication_enabled: true });
  }), [client]);
  // Another browser/tab can revoke the cookie's server-side session.
  useEffect(() => {
    if (data?.authentication_enabled !== false && data?.authenticated === false && wasAuthenticated.current) {
      wasAuthenticated.current = false;
      expireSession();
    }
    if (data?.authentication_enabled === false) wasAuthenticated.current = false;
    if (data?.authentication_enabled !== false && data?.authenticated === true) wasAuthenticated.current = true;
    if (data?.authentication_enabled !== false && data?.authenticated === false) {
      void client.cancelQueries({ predicate: (query) => query.queryKey[0] !== queryKeys.authSession[0] });
      client.removeQueries({ predicate: (query) => query.queryKey[0] !== queryKeys.authSession[0] });
    }
  }, [client, data?.authenticated, data?.authentication_enabled]);
  // A failed background refetch keeps the last known session; only a missing one is an error.
  if (isLoading) return <AppLoadingScreen />;
  if (!data) return <UnavailableState onRetry={() => refetch()}>{t("sessionUnavailable")}</UnavailableState>;
  if (data.authentication_enabled && !data.authenticated) return <LoginPage onDone={() => { void invalidateAuthSession(client); }} />;
  return <>{children}</>;
}
