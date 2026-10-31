import { apiMessage } from "../i18n/apiMessage";
import { apiFetch } from "./transport";
import type { Period } from "../hooks/usePeriod";
import { requestJson } from "./client";
import { parseCategoryBreakdown, type CategoryDirection } from "./contracts";
import {
  parseProblem,
  parseSpendingByCategory,
  parseTransactionCategoryResult,
  parseTransactionInclusionResult,
  parseTransactionItem,
  parseTransactionQueryResult,
  parseTransactionReconciliation,
  type ManualTransactionWrite,
  type Problem,
  type TransactionReconciliation,
  type SpendingByCategory,
  type TransactionCategoryResult,
  type TransactionInclusionResult,
  type TransactionInclusionState,
  type TransactionItem,
  type TransactionQuery,
  type TransactionQueryResult,
} from "./contracts";
import { ApiError, isAbortError } from "./problems";

export async function queryTransactions(
  query: TransactionQuery,
  signal?: AbortSignal,
): Promise<TransactionQueryResult> {
  let response: Response;
  try {
    response = await apiFetch("/api/transactions/query", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(query),
      signal,
    });
  } catch (error) {
    if (isAbortError(error)) throw error;
    throw new ApiError("transport", apiMessage("apiTransactionsQuery"));
  }

  const contentType = response.headers.get("content-type")?.split(";")[0].trim();
  let body: unknown;
  try {
    body = await response.json();
  } catch {
    throw new ApiError("response", apiMessage("apiTransactionsResponse"));
  }

  if (!response.ok) {
    let problem: Problem | undefined;
    if (contentType === "application/problem+json") {
      try {
        problem = parseProblem(body);
      } catch {
        problem = undefined;
      }
    }
    throw new ApiError(
      "response",
      problem?.detail ?? problem?.title ?? apiMessage("apiTransactionsQuery"),
      problem,
    );
  }
  if (contentType !== "application/json") {
    throw new ApiError("response", apiMessage("apiTransactionsResponse"));
  }
  try {
    return parseTransactionQueryResult(body);
  } catch {
    throw new ApiError("response", apiMessage("apiTransactionsResponse"));
  }
}

export async function getSpendingByCategory(
  timezone: string,
  signal?: AbortSignal,
  scenarioId?: string,
): Promise<SpendingByCategory> {
  const params = new URLSearchParams({ timezone });
  if (scenarioId !== undefined) params.set("scenario_id", scenarioId);
  let response: Response;
  try {
    response = await apiFetch(`/api/transactions/spending-by-category?${params.toString()}`, {
      method: "GET",
      headers: { Accept: "application/json" },
      signal,
    });
  } catch (error) {
    if (isAbortError(error)) throw error;
    throw new ApiError("transport", apiMessage("apiSpendingQuery"));
  }

  const contentType = response.headers.get("content-type")?.split(";")[0].trim();
  let body: unknown;
  try {
    body = await response.json();
  } catch {
    throw new ApiError("response", apiMessage("apiSpendingResponse"));
  }

  if (!response.ok) {
    let problem: Problem | undefined;
    if (contentType === "application/problem+json") {
      try {
        problem = parseProblem(body);
      } catch {
        problem = undefined;
      }
    }
    throw new ApiError(
      "response",
      problem?.detail ?? problem?.title ?? apiMessage("apiSpendingQuery"),
      problem,
    );
  }
  if (contentType !== "application/json") {
    throw new ApiError("response", apiMessage("apiSpendingResponse"));
  }
  try {
    return parseSpendingByCategory(body);
  } catch {
    throw new ApiError("response", apiMessage("apiSpendingResponse"));
  }
}

export async function setTransactionInclusion(
  transactionId: string,
  state: TransactionInclusionState,
): Promise<TransactionInclusionResult> {
  let response: Response;
  try {
    response = await apiFetch(`/api/transactions/${transactionId}/inclusion`, {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ state }),
    });
  } catch (error) {
    if (isAbortError(error)) throw error;
    throw new ApiError("transport", apiMessage("apiDecisionSave"));
  }

  const contentType = response.headers.get("content-type")?.split(";")[0].trim();
  let body: unknown;
  try {
    body = await response.json();
  } catch {
    throw new ApiError("response", apiMessage("apiDecisionInvalid"));
  }
  if (!response.ok) {
    let problem: Problem | undefined;
    if (contentType === "application/problem+json") {
      try {
        problem = parseProblem(body);
      } catch {
        problem = undefined;
      }
    }
    throw new ApiError(
      "response",
      problem?.detail ?? problem?.title ?? apiMessage("apiDecisionSave"),
      problem,
    );
  }
  if (contentType !== "application/json") {
    throw new ApiError("response", apiMessage("apiDecisionInvalid"));
  }
  try {
    return parseTransactionInclusionResult(body);
  } catch {
    throw new ApiError("response", apiMessage("apiDecisionInvalid"));
  }
}

export async function setTransactionCategory(
  transactionId: string,
  categoryId: string,
): Promise<TransactionCategoryResult> {
  let response: Response;
  try {
    response = await apiFetch(`/api/transactions/${transactionId}/category`, {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ category_id: categoryId }),
    });
  } catch (error) {
    if (isAbortError(error)) throw error;
    throw new ApiError("transport", apiMessage("apiTxCategorySave"));
  }

  const contentType = response.headers.get("content-type")?.split(";")[0].trim();
  let body: unknown;
  try {
    body = await response.json();
  } catch {
    throw new ApiError("response", apiMessage("apiCategoryConfirmInvalid"));
  }
  if (!response.ok) {
    let problem: Problem | undefined;
    if (contentType === "application/problem+json") {
      try {
        problem = parseProblem(body);
      } catch {
        problem = undefined;
      }
    }
    throw new ApiError(
      "response",
      problem?.detail ?? problem?.title ?? apiMessage("apiTxCategorySave"),
      problem,
    );
  }
  if (contentType !== "application/json") {
    throw new ApiError("response", apiMessage("apiCategoryConfirmInvalid"));
  }
  try {
    return parseTransactionCategoryResult(body);
  } catch {
    throw new ApiError("response", apiMessage("apiCategoryConfirmInvalid"));
  }
}

async function sendManualTransaction(
  method: "POST" | "PUT",
  url: string,
  write: ManualTransactionWrite,
  failure: string,
  idempotencyKey?: string,
): Promise<TransactionItem> {
  let response: Response;
  try {
    response = await apiFetch(url, {
      method,
      headers: { "content-type": "application/json", ...(idempotencyKey ? { "Idempotency-Key": idempotencyKey } : {}) },
      body: JSON.stringify(write),
    });
  } catch (error) {
    if (isAbortError(error)) throw error;
    throw new ApiError("transport", failure);
  }

  const contentType = response.headers.get("content-type")?.split(";")[0].trim();
  let body: unknown;
  try {
    body = await response.json();
  } catch {
    throw new ApiError("response", failure);
  }
  if (!response.ok) {
    let problem: Problem | undefined;
    if (contentType === "application/problem+json") {
      try {
        problem = parseProblem(body);
      } catch {
        problem = undefined;
      }
    }
    throw new ApiError(
      response.status === 404 ? "not_found" : response.status === 409 ? "conflict" : "response",
      problem?.detail ?? problem?.title ?? failure,
      problem,
    );
  }
  if (contentType !== "application/json") {
    throw new ApiError("response", failure);
  }
  try {
    return parseTransactionItem(body);
  } catch {
    throw new ApiError("response", failure);
  }
}

export function createManualTransaction(write: ManualTransactionWrite, idempotencyKey: string = crypto.randomUUID()): Promise<TransactionItem> {
  return sendManualTransaction("POST", "/api/transactions", write, apiMessage("apiManualCreate"), idempotencyKey);
}

export function updateManualTransaction(
  transactionId: string,
  write: ManualTransactionWrite,
): Promise<TransactionItem> {
  return sendManualTransaction(
    "PUT",
    `/api/transactions/${encodeURIComponent(transactionId)}`,
    write,
    apiMessage("apiManualSave"),
  );
}

export async function deleteManualTransaction(transactionId: string): Promise<void> {
  const failure = apiMessage("apiManualDelete");
  let response: Response;
  try {
    response = await apiFetch(`/api/transactions/${encodeURIComponent(transactionId)}`, { method: "DELETE" });
  } catch (error) {
    if (isAbortError(error)) throw error;
    throw new ApiError("transport", failure);
  }
  if (response.status === 204) return;

  const contentType = response.headers.get("content-type")?.split(";")[0].trim();
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
    problem?.detail ?? problem?.title ?? failure,
    problem,
  );
}

/**
 * Reads what this transaction currently settles and what else it could,
 * in one round trip. Read-only on purpose: both writes go through the
 * occurrence-scoped endpoints in ./recurringCommitments, so there is exactly
 * one place that decides what a valid reconciliation is.
 */
export async function getTransactionReconciliation(
  transactionId: string,
  signal?: AbortSignal,
): Promise<TransactionReconciliation> {
  const failure = apiMessage("apiReconciliationQuery");
  let response: Response;
  try {
    response = await apiFetch(`/api/transactions/${encodeURIComponent(transactionId)}/reconciliation`, {
      method: "GET",
      headers: { Accept: "application/json" },
      signal,
    });
  } catch (error) {
    if (isAbortError(error)) throw error;
    throw new ApiError("transport", failure);
  }

  const contentType = response.headers.get("content-type")?.split(";")[0].trim();
  if (!response.ok) {
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
      problem?.detail ?? problem?.title ?? failure,
      problem,
    );
  }
  if (contentType !== "application/json") {
    throw new ApiError("response", failure);
  }
  try {
    return parseTransactionReconciliation(await response.json());
  } catch {
    throw new ApiError("response", failure);
  }
}

export function getCategoryBreakdown(timezone: string, period: Period | string, classification: CategoryDirection, signal?: AbortSignal) {
  const params = new URLSearchParams({ timezone, classification });
  if (typeof period === "string") params.set("month", period);
  else if (period.from === null) params.set("period", "all");
  else {
    params.set("date_from", period.from);
    params.set("date_to", period.to);
  }
  return requestJson(
    `/api/transactions/category-breakdown?${params}`,
    { method: "GET", headers: { Accept: "application/json" }, signal },
    parseCategoryBreakdown,
    200,
  );
}
