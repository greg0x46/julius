import { apiMessage } from "../i18n/apiMessage";
import { apiFetch } from "./transport";
import { parseProblem, type Problem } from "./contracts";
import { ApiError, isAbortError } from "./problems";

type Parser<T> = (value: unknown) => T;

function isJson(contentType: string | null): boolean {
  return contentType?.toLowerCase().split(";")[0].trim() === "application/json";
}

function isProblemJson(contentType: string | null): boolean {
  return contentType?.toLowerCase().split(";")[0].trim() === "application/problem+json";
}

async function parseBody(response: Response): Promise<unknown> {
  try {
    return await response.json();
  } catch {
    throw new ApiError("response", apiMessage("apiResponseUnconfirmed"));
  }
}

export async function requestJson<T>(
  input: RequestInfo | URL,
  init: RequestInit,
  parser: Parser<T>,
  expectedStatus: number,
): Promise<T> {
  let response: Response;
  try {
    response = await apiFetch(input, init);
  } catch (error) {
    if (isAbortError(error)) {
      throw error;
    }
    throw new ApiError("transport", apiMessage("apiCommunication"));
  }

  const contentType = response.headers.get("content-type");
  if (response.status === 404 || response.status === 409) {
    let problem: Problem | undefined;
    if (isProblemJson(contentType)) {
      try {
        problem = parseProblem(await parseBody(response));
      } catch {
        problem = undefined;
      }
    }
    throw new ApiError(
      response.status === 404 ? "not_found" : "conflict",
      response.status === 404
        ? apiMessage("apiSyncNotFound")
        : apiMessage("apiSyncRunning"),
      problem,
    );
  }

  if (response.status !== expectedStatus || !isJson(contentType)) {
    throw new ApiError("response", apiMessage("apiResponseUnconfirmed"));
  }

  try {
    return parser(await parseBody(response));
  } catch (error) {
    if (error instanceof ApiError) {
      throw error;
    }
    throw new ApiError("response", apiMessage("apiResponseUnconfirmed"));
  }
}
