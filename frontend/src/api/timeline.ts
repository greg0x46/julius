import { apiFetch } from "./transport";
import {
  parseProblem,
  parseTimelineDataRange,
  parseTimelineResponse,
  type Problem,
  type TimelineDataRange,
  type TimelineParams,
  type TimelineResponse,
} from "./contracts";
import { ApiError, isAbortError } from "./problems";

const defaultMessage = "Não foi possível carregar a linha do tempo.";

function buildQuery(params: TimelineParams): string {
  const search = new URLSearchParams({
    reference_date: params.referenceDate,
    from: params.from,
    to: params.to,
  });
  if (params.timezone) search.set("timezone", params.timezone);
  if (params.accountIds && params.accountIds.length > 0) search.set("account_ids", params.accountIds.join(","));
  if (params.categoryIds && params.categoryIds.length > 0) search.set("category_ids", params.categoryIds.join(","));
  if (params.cardNumbers && params.cardNumbers.length > 0) search.set("card_numbers", params.cardNumbers.join(","));
  if (params.scenarioIds && params.scenarioIds.length > 0) search.set("scenario_ids", params.scenarioIds.join(","));
  return search.toString();
}

export async function getTimelineDataRange(timezone = "UTC", signal?: AbortSignal): Promise<TimelineDataRange> {
  const search = new URLSearchParams({ timezone });
  return readJSON(await request(`/api/timeline/range?${search}`, signal), parseTimelineDataRange);
}

export async function getTimeline(params: TimelineParams, signal?: AbortSignal): Promise<TimelineResponse> {
  return readJSON(await request(`/api/timeline?${buildQuery(params)}`, signal), parseTimelineResponse);
}

async function request(url: string, signal?: AbortSignal): Promise<Response> {
  let response: Response;
  try {
    response = await apiFetch(url, {
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
  return response;
}

// Parsing lives behind readJSON so a malformed body reads as the same
// "response" failure whether it broke at JSON.parse or at the contract.
async function readJSON<T>(response: Response, parse: (value: unknown) => T): Promise<T> {
  try {
    return parse(await response.json());
  } catch {
    throw new ApiError("response", defaultMessage);
  }
}
