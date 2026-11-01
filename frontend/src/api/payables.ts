import { apiMessage } from "../i18n/apiMessage";
import { apiFetch } from "./transport";
import {
  parseEligibleTransactionList,
  parsePayable,
  parsePayableDetail,
  parsePayableLink,
  parsePayableList,
  parsePayableTotalOwed,
  parsePayableTotalToReceive,
  parseProblem,
  type EligibleTransaction,
  type Payable,
  type PayableCreate,
  type PayableDetail,
  type PayableKind,
  type PayableLink,
  type PayableTotalOwed,
  type PayableTotalToReceive,
  type PayableUpdate,
  type Problem,
} from "./contracts";
import { ApiError, isAbortError } from "./problems";

const defaultMessage = () => apiMessage("apiPayableSave");

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
      response.status === 404 ? "not_found" : response.status === 409 ? "conflict" : "response",
      problem?.detail ?? problem?.title ?? defaultMessage(),
      problem,
    );
  }

  if (parser === null) {
    return undefined as T;
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

export function listPayables(kind: PayableKind | null, signal?: AbortSignal): Promise<Payable[]> {
  const params = new URLSearchParams();
  if (kind !== null) params.set("kind", kind);
  const query = params.toString();
  return send(
    `/api/payables${query === "" ? "" : `?${query}`}`,
    { method: "GET", headers: { Accept: "application/json" }, signal },
    200,
    parsePayableList,
  );
}

export function getPayableTotalOwed(signal?: AbortSignal): Promise<PayableTotalOwed> {
  return send(
    "/api/payables/total-owed",
    { method: "GET", headers: { Accept: "application/json" }, signal },
    200,
    parsePayableTotalOwed,
  );
}

export function getPayableTotalToReceive(signal?: AbortSignal): Promise<PayableTotalToReceive> {
  return send(
    "/api/payables/total-to-receive",
    { method: "GET", headers: { Accept: "application/json" }, signal },
    200,
    parsePayableTotalToReceive,
  );
}

export function createPayable(write: PayableCreate): Promise<Payable> {
  return send(
    "/api/payables",
    {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(write),
    },
    201,
    parsePayable,
  );
}

export function getPayable(payableId: string, signal?: AbortSignal): Promise<PayableDetail> {
  return send(
    `/api/payables/${encodeURIComponent(payableId)}`,
    { method: "GET", headers: { Accept: "application/json" }, signal },
    200,
    parsePayableDetail,
  );
}

export function updatePayable(payableId: string, write: PayableUpdate): Promise<Payable> {
  return send(
    `/api/payables/${encodeURIComponent(payableId)}`,
    {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(write),
    },
    200,
    parsePayable,
  );
}

export function deletePayable(payableId: string): Promise<void> {
  return send(`/api/payables/${encodeURIComponent(payableId)}`, { method: "DELETE" }, 204, null);
}

export function listEligibleTransactions(
  kind: PayableKind,
  search: string,
  signal?: AbortSignal,
): Promise<EligibleTransaction[]> {
  const params = new URLSearchParams({ kind });
  if (search.trim() !== "") params.set("search", search.trim());
  return send(
    `/api/payables/eligible-transactions?${params.toString()}`,
    { method: "GET", headers: { Accept: "application/json" }, signal },
    200,
    parseEligibleTransactionList,
  );
}

export function createPayableLink(payableId: string, transactionId: string): Promise<PayableLink> {
  return send(
    `/api/payables/${encodeURIComponent(payableId)}/links`,
    {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ transaction_id: transactionId }),
    },
    201,
    parsePayableLink,
  );
}

export function deletePayableLink(payableId: string, linkId: string): Promise<void> {
  return send(
    `/api/payables/${encodeURIComponent(payableId)}/links/${encodeURIComponent(linkId)}`,
    { method: "DELETE" },
    204,
    null,
  );
}
