import { apiMessage } from "../i18n/apiMessage";
import { apiFetch } from "./transport";
import {
  parseCategory,
  parseCategoryList,
  parseProblem,
  type Category,
  type CategoryCreate,
  type CategoryUpdate,
  type Problem,
} from "./contracts";
import { ApiError, isAbortError } from "./problems";

const defaultMessage = () => apiMessage("apiCategorySave");

async function send<T>(
  input: RequestInfo | URL,
  init: RequestInit,
  expectedStatus: number,
  parser: (value: unknown) => T,
): Promise<T> {
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
    throw new ApiError(
      response.status === 404 ? "not_found" : "response",
      problem?.detail ?? problem?.title ?? defaultMessage(),
      problem,
    );
  }

  if (contentType !== "application/json") {
    throw new ApiError("response", defaultMessage());
  }
  try {
    return parser(await response.json());
  } catch {
    throw new ApiError("response", defaultMessage());
  }
}

export function listCategories(signal?: AbortSignal): Promise<Category[]> {
  return send(
    "/api/categories",
    { method: "GET", headers: { Accept: "application/json" }, signal },
    200,
    parseCategoryList,
  );
}

export function createCategory(write: CategoryCreate): Promise<Category> {
  return send(
    "/api/categories",
    {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(write),
    },
    201,
    parseCategory,
  );
}

export function updateCategory(categoryId: string, write: CategoryUpdate): Promise<Category> {
  return send(
    `/api/categories/${encodeURIComponent(categoryId)}`,
    {
      method: "PATCH",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(write),
    },
    200,
    parseCategory,
  );
}
