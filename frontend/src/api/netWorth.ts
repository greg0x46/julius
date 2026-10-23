import { apiFetch } from "./transport";
import { parseNetWorthSeries, parseProblem, type NetWorthSeries, type Problem } from "./contracts";
import { ApiError, isAbortError } from "./problems";

const defaultMessage = "Não foi possível carregar o patrimônio.";

export async function getNetWorth(signal?: AbortSignal): Promise<NetWorthSeries> {
  let response: Response;
  try {
    response = await apiFetch("/api/net-worth", {
      method: "GET",
      headers: { Accept: "application/json" },
      signal,
    });
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
    throw new ApiError("response", problem?.detail ?? problem?.title ?? defaultMessage, problem);
  }

  if (contentType !== "application/json") {
    throw new ApiError("response", defaultMessage);
  }
  try {
    return parseNetWorthSeries(await response.json());
  } catch {
    throw new ApiError("response", defaultMessage);
  }
}
