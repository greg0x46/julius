import { translate, type Language, type MessageKey } from "../i18n/messages";
import { apiFetch, expireSession } from "./transport";

export class AuthError extends Error {
  constructor(message: string, readonly type?: string) {
    super(message);
    this.name = "AuthError";
  }
}

export function authErrorMessage(error: unknown, language: Language, fallback: string): string {
  if (!(error instanceof Error)) return fallback;
  if (!(error instanceof AuthError) || language === "pt-BR") return error.message;
  const keys: Record<string, MessageKey> = {
    "invalid-response": "invalidAuthResponse",
    "/problems/unauthorized": "authRequired",
    "/problems/auth-unavailable": "authUnavailable",
    "/problems/auth-config-unavailable": "authUnavailable",
    "/problems/too-many-attempts": "authTooManyAttempts",
    "/problems/auth-disabled": "authDisabledLogin",
    "/problems/invalid-login": "invalidLogin",
    "/problems/invalid-password": "invalidPassword",
    "/problems/invalid-auth-config": "invalidAuthConfig",
    "/problems/invalid-origin": "invalidOrigin",
    "/problems/demo-mode": "authDemoMode",
  };
  const key = error.type ? keys[error.type] : undefined;
  return translate(language, key ?? "authFailure");
}

export type AuthSession = { authenticated: boolean; authentication_enabled: boolean; email?: string; demo?: boolean };
async function send(path: string, method: string, payload?: unknown, signal?: AbortSignal): Promise<AuthSession> {
  const response = await apiFetch(path, {
    method, signal, headers: { "Content-Type": "application/json" },
    ...(payload === undefined ? {} : { body: JSON.stringify(payload) }),
  });
  if (!response.ok) {
    let message = "Não foi possível concluir a autenticação.";
    let type: string | undefined;
    try {
      const problem = await response.json() as { detail?: string; title?: string; type?: string };
      message = problem.detail || problem.title || message;
      type = problem.type;
    } catch { /* Keep a useful message if the server response is not JSON. */ }
    throw new AuthError(message, type);
  }
  const value: unknown = await response.json();
  if (!value || typeof value !== "object" || !("authenticated" in value) || typeof value.authenticated !== "boolean" ||
      !("authentication_enabled" in value) || typeof value.authentication_enabled !== "boolean") {
    throw new AuthError("Resposta de autenticação inválida.", "invalid-response");
  }
  return value as AuthSession;
}
export const getSession = (signal?: AbortSignal) => send("/api/auth/session", "GET", undefined, signal);
export const login = (email: string, password: string) => send("/api/auth/login", "POST", { email, password });
export const setAuthenticationEnabled = (enabled: boolean) => send("/api/auth/config", "PUT", { enabled });
export async function logout() {
  await send("/api/auth/logout", "POST");
  expireSession();
}
export async function changePassword(current_password: string, password: string) {
  await send("/api/auth/password", "PUT", { current_password, password });
  expireSession();
}
