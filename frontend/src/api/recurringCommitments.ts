import { apiMessage } from "../i18n/apiMessage";
import { apiFetch } from "./transport";
import {
  parseEligibleTransactionList,
  parseRecurrenceOccurrence,
  parseRecurrenceOccurrenceList,
  parseRecurringCommitment,
  parseRecurringCommitmentList,
  parseProblem,
  type EligibleTransaction,
  type Problem,
  type RecurrenceOccurrence,
  type ReconciliationWrite,
  type RecurringCommitment,
  type RecurringCommitmentWrite,
} from "./contracts";
import { ApiError, isAbortError } from "./problems";

const defaultMessage = () => apiMessage("recurrenceSaveFailed");

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

export function listRecurringCommitments(signal?: AbortSignal): Promise<RecurringCommitment[]> {
  return send(
    "/api/recurring-scenarios",
    { method: "GET", headers: { Accept: "application/json" }, signal },
    200,
    parseRecurringCommitmentList,
  );
}

export function createRecurringCommitment(
  write: RecurringCommitmentWrite,
): Promise<RecurringCommitment> {
  return send(
    "/api/recurring-scenarios",
    {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(write),
    },
    201,
    parseRecurringCommitment,
  );
}

export function updateRecurringCommitment(
  commitmentId: string,
  write: RecurringCommitmentWrite,
): Promise<RecurringCommitment> {
  return send(
    `/api/recurring-scenarios/${encodeURIComponent(commitmentId)}`,
    {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(write),
    },
    200,
    parseRecurringCommitment,
  );
}

export function setRecurringCommitmentActive(
  commitmentId: string,
  isActive: boolean,
): Promise<RecurringCommitment> {
  return send(
    `/api/recurring-scenarios/${encodeURIComponent(commitmentId)}`,
    {
      method: "PATCH",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ is_active: isActive }),
    },
    200,
    parseRecurringCommitment,
  );
}

export function deleteRecurringCommitment(commitmentId: string): Promise<void> {
  return send(
    `/api/recurring-scenarios/${encodeURIComponent(commitmentId)}`,
    { method: "DELETE" },
    204,
    null,
  );
}

/**
 * An occurrence's address: the recurring Scenario plus the calendar day.
 * Occurrences have no stored id — the schedule generates them — so every
 * reconciliation write is keyed this way.
 */
function occurrenceBase(commitmentId: string, occurrenceDate: string): string {
  return `/api/scenarios/${encodeURIComponent(commitmentId)}/occurrences/${encodeURIComponent(occurrenceDate)}`;
}

export function listRecurrenceOccurrences(
  commitmentId: string,
  signal?: AbortSignal,
): Promise<RecurrenceOccurrence[]> {
  return send(
    `/api/scenarios/${encodeURIComponent(commitmentId)}/occurrences`,
    { method: "GET", headers: { Accept: "application/json" }, signal },
    200,
    parseRecurrenceOccurrenceList,
  );
}

export function listReconciliationCandidates(
  commitmentId: string,
  occurrenceDate: string,
  search: string,
  signal?: AbortSignal,
): Promise<EligibleTransaction[]> {
  const params = new URLSearchParams();
  if (search.trim() !== "") params.set("search", search.trim());
  const query = params.toString();
  return send(
    `${occurrenceBase(commitmentId, occurrenceDate)}/candidates${query === "" ? "" : `?${query}`}`,
    { method: "GET", headers: { Accept: "application/json" }, signal },
    200,
    parseEligibleTransactionList,
  );
}

/** Records a manual decision: link a transaction, or detach the occurrence. */
export function putReconciliation(
  commitmentId: string,
  occurrenceDate: string,
  write: ReconciliationWrite,
): Promise<RecurrenceOccurrence> {
  return send(
    `${occurrenceBase(commitmentId, occurrenceDate)}/reconciliation`,
    {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(write),
    },
    200,
    parseRecurrenceOccurrence,
  );
}

/**
 * Forgets the manual decision, handing the occurrence back to the automation
 * rule — "voltar ao automático", not "desconciliar".
 */
export function deleteReconciliation(
  commitmentId: string,
  occurrenceDate: string,
): Promise<void> {
  return send(
    `${occurrenceBase(commitmentId, occurrenceDate)}/reconciliation`,
    { method: "DELETE" },
    204,
    null,
  );
}
