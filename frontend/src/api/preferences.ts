import { apiMessage } from "../i18n/apiMessage";
import { apiFetch } from "./transport";
import { parsePreferences, parseProblem, type Preferences, type Problem } from "./contracts";
import { ApiError, isAbortError } from "./problems";

const defaultMessage = () => apiMessage("preferencesSaveError");

async function send(input: RequestInfo | URL, init: RequestInit, expectedStatus: number): Promise<Preferences> {
  let response: Response;
  try {
    response = await apiFetch(input, init);
  } catch (error) {
    if (isAbortError(error)) throw error;
    throw new ApiError("transport", defaultMessage());
  }

  const contentType = response.headers.get("content-type")?.split(";")[0].trim();

  if (response.status !== expectedStatus) {
    let problem: Problem | undefined;
    if (contentType === "application/problem+json") {
      try {
        problem = parseProblem(await response.json());
      } catch {
        problem = undefined;
      }
    }
    throw new ApiError("response", problem?.detail ?? problem?.title ?? defaultMessage(), problem);
  }

  if (contentType !== "application/json") {
    throw new ApiError("response", defaultMessage());
  }
  try {
    return parsePreferences(await response.json());
  } catch {
    throw new ApiError("response", defaultMessage());
  }
}

export function getPreferences(signal?: AbortSignal): Promise<Preferences> {
  return send("/api/preferences", { method: "GET", headers: { Accept: "application/json" }, signal }, 200);
}

export function updatePreferences(preferences: Preferences): Promise<Preferences> {
  return send(
    "/api/preferences",
    {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(preferences),
    },
    200,
  );
}
