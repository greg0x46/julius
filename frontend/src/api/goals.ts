import { apiFetch } from "./transport";
import { parseGoal, parseGoalList, parseProblem, type Goal, type GoalWrite, type Problem } from "./contracts";
import { ApiError, isAbortError } from "./problems";

const defaultMessage = "Não foi possível atualizar as metas.";

async function send<T>(
  input: RequestInfo | URL,
  init: RequestInit,
  expectedStatus: number,
  parser: ((value: unknown) => T) | null,
): Promise<T> {
  let response: Response;
  try {
    response = await apiFetch(input, init);
  } catch (error) {
    if (isAbortError(error)) throw error;
    throw new ApiError("transport", defaultMessage);
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
    throw new ApiError(
      response.status === 404 ? "not_found" : response.status === 409 ? "conflict" : "response",
      problem?.detail ?? problem?.title ?? defaultMessage,
      problem,
    );
  }

  if (parser === null) return undefined as T;
  if (contentType !== "application/json") throw new ApiError("response", defaultMessage);
  try {
    return parser(await response.json());
  } catch {
    throw new ApiError("response", defaultMessage);
  }
}

const json = { "content-type": "application/json", Accept: "application/json" } as const;

export function listGoals(signal?: AbortSignal): Promise<Goal[]> {
  return send("/api/goals", { method: "GET", headers: { Accept: "application/json" }, signal }, 200, parseGoalList);
}

export function createGoal(write: GoalWrite): Promise<Goal> {
  return send("/api/goals", { method: "POST", headers: json, body: JSON.stringify(write) }, 201, parseGoal);
}

export function updateGoal(goalId: string, write: GoalWrite): Promise<Goal> {
  return send(
    `/api/goals/${encodeURIComponent(goalId)}`,
    { method: "PUT", headers: json, body: JSON.stringify(write) },
    200,
    parseGoal,
  );
}

export function deleteGoal(goalId: string): Promise<void> {
  return send(
    `/api/goals/${encodeURIComponent(goalId)}`,
    { method: "DELETE", headers: { Accept: "application/json" } },
    204,
    null,
  );
}
