import { apiFetch, expireSession } from "./transport";

export type AuthSession = { authenticated: boolean; authentication_enabled: boolean; email?: string; demo?: boolean };
async function send(path: string, method: string, payload?: unknown, signal?: AbortSignal): Promise<AuthSession> {
  const response = await apiFetch(path, {
    method, signal, headers: { "Content-Type": "application/json" },
    ...(payload === undefined ? {} : { body: JSON.stringify(payload) }),
  });
  if (!response.ok) {
    let message = "Não foi possível concluir a autenticação.";
    try {
      const problem = await response.json() as { detail?: string; title?: string };
      message = problem.detail || problem.title || message;
    } catch { /* Keep a useful message if the server response is not JSON. */ }
    throw new Error(message);
  }
  const value: unknown = await response.json();
  if (!value || typeof value !== "object" || !("authenticated" in value) || typeof value.authenticated !== "boolean" ||
      !("authentication_enabled" in value) || typeof value.authentication_enabled !== "boolean") {
    throw new Error("Resposta de autenticação inválida.");
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
