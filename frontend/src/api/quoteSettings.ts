import { apiFetch } from "./transport";
import {
  parseProblem,
  parseQuoteLastRefresh,
  parseQuoteSettings,
  type Problem,
  type QuoteLastRefresh,
  type QuoteSchedule,
  type QuoteSettings,
} from "./contracts";
import { ApiError, isAbortError } from "./problems";

async function send<T>(
  input: string,
  init: RequestInit,
  parse: (value: unknown) => T,
  defaultMessage: string,
): Promise<T> {
  let response: Response;
  try {
    response = await apiFetch(input, init);
  } catch (error) {
    if (isAbortError(error)) throw error;
    throw new ApiError("transport", defaultMessage);
  }
  const contentType = response.headers.get("content-type")?.split(";")[0].trim();
  if (response.status !== 200) {
    let problem: Problem | undefined;
    if (contentType === "application/problem+json") {
      try {
        problem = parseProblem(await response.json());
      } catch {
        problem = undefined;
      }
    }
    const kind = response.status === 409 ? "conflict" : "response";
    throw new ApiError(kind, defaultMessage, problem);
  }
  try {
    return parse(await response.json());
  } catch (error) {
    if (isAbortError(error)) throw error;
    throw new ApiError("response", defaultMessage);
  }
}

export function getQuoteSettings(signal?: AbortSignal): Promise<QuoteSettings> {
  return send(
    "/api/settings/quotes",
    { method: "GET", headers: { Accept: "application/json" }, signal },
    parseQuoteSettings,
    "Não foi possível carregar as configurações de cotação.",
  );
}

export function updateQuoteSchedule(schedule: QuoteSchedule): Promise<void> {
  return send(
    "/api/settings/quotes",
    { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(schedule) },
    () => undefined,
    "Não foi possível salvar o agendamento.",
  );
}

export function refreshQuotesNow(): Promise<QuoteLastRefresh> {
  return send(
    "/api/settings/quotes/refresh",
    { method: "POST" },
    parseQuoteLastRefresh,
    "Não foi possível atualizar as cotações agora.",
  );
}
