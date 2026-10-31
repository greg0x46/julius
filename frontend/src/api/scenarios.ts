import { apiMessage } from "../i18n/apiMessage";
import { apiFetch } from "./transport";
import {
  parseProblem,
  parsePlannedTransactionList,
  parseScenarioDetail,
  parseScenarioList,
  parseScenario,
  parseScenarioRealization,
  parseScenarioTransaction,
  type GenerateInstallmentsWrite,
  type Problem,
  type PlannedTransaction,
  type ReadjustWrite,
  type RealizationWrite,
  type Scenario,
  type ScenarioKind,
  type ScenarioRealization,
  type ScenarioCreate,
  type ScenarioDetail,
  type ScenarioTransaction,
  type ScenarioTransactionWrite,
} from "./contracts";
import { ApiError, isAbortError } from "./problems";

const defaultMessage = () => apiMessage("apiPlanSave");

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

export function listPayableScenarios(payableId: string, signal?: AbortSignal): Promise<Scenario[]> {
  return send(
    `/api/payables/${encodeURIComponent(payableId)}/scenarios`,
    { method: "GET", headers: { Accept: "application/json" }, signal },
    200,
    parseScenarioList,
  );
}

export function createPayableScenario(
  payableId: string,
  write: ScenarioCreate,
): Promise<ScenarioDetail> {
  return send(
    `/api/payables/${encodeURIComponent(payableId)}/scenarios`,
    {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(write),
    },
    201,
    parseScenarioDetail,
  );
}

export function listStandaloneScenarios(signal?: AbortSignal): Promise<Scenario[]> {
  return send(
    "/api/scenarios?kind=standalone",
    { method: "GET", headers: { Accept: "application/json" }, signal },
    200,
    parseScenarioList,
  );
}

export function listScenarios(
  filters: { kind?: ScenarioKind; isActive?: boolean } = {},
  signal?: AbortSignal,
): Promise<Scenario[]> {
  const params = new URLSearchParams();
  if (filters.kind) params.set("kind", filters.kind);
  if (filters.isActive !== undefined) params.set("is_active", String(filters.isActive));
  const query = params.toString();
  return send(
    `/api/scenarios${query === "" ? "" : `?${query}`}`,
    { method: "GET", headers: { Accept: "application/json" }, signal },
    200,
    parseScenarioList,
  );
}

export function setScenarioActive(scenarioId: string, isActive: boolean): Promise<Scenario> {
  return send(
    `/api/scenarios/${encodeURIComponent(scenarioId)}`,
    {
      method: "PATCH",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ is_active: isActive }),
    },
    200,
    parseScenario,
  );
}

export function listScenarioPlannedTransactions(
  scenarioId: string,
  from?: string,
  to?: string,
  signal?: AbortSignal,
): Promise<PlannedTransaction[]> {
  const params = new URLSearchParams();
  if (from) params.set("from", from);
  if (to) params.set("to", to);
  const query = params.toString();
  return send(
    `/api/scenarios/${encodeURIComponent(scenarioId)}/planned-transactions${query === "" ? "" : `?${query}`}`,
    { method: "GET", headers: { Accept: "application/json" }, signal },
    200,
    parsePlannedTransactionList,
  );
}

export interface PlannedRealizationWrite {
  state?: "linked" | "detached";
  transaction_id?: string | null;
  allocated_amount?: number;
}

export function realizePlannedTransaction(
  scenarioId: string,
  eventKey: string,
  write: PlannedRealizationWrite,
): Promise<ScenarioRealization> {
  return send(
    `/api/scenarios/${encodeURIComponent(scenarioId)}/planned-transactions/${encodeURIComponent(eventKey)}/realization`,
    {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(write),
    },
    200,
    parseScenarioRealization,
  );
}

export function deletePlannedTransactionRealization(scenarioId: string, eventKey: string): Promise<void> {
  return send(
    `/api/scenarios/${encodeURIComponent(scenarioId)}/planned-transactions/${encodeURIComponent(eventKey)}/realization`,
    { method: "DELETE" },
    204,
    null,
  );
}

export function createStandaloneScenario(write: ScenarioCreate): Promise<ScenarioDetail> {
  return send(
    "/api/scenarios",
    {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(write),
    },
    201,
    parseScenarioDetail,
  );
}

export function getScenario(scenarioId: string, signal?: AbortSignal): Promise<ScenarioDetail> {
  return send(
    `/api/scenarios/${encodeURIComponent(scenarioId)}`,
    { method: "GET", headers: { Accept: "application/json" }, signal },
    200,
    parseScenarioDetail,
  );
}

export function deleteScenario(scenarioId: string): Promise<void> {
  return send(`/api/scenarios/${encodeURIComponent(scenarioId)}`, { method: "DELETE" }, 204, null);
}

export function createScenarioTransaction(
  scenarioId: string,
  write: ScenarioTransactionWrite,
): Promise<ScenarioTransaction> {
  return send(
    `/api/scenarios/${encodeURIComponent(scenarioId)}/transactions`,
    {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(write),
    },
    201,
    parseScenarioTransaction,
  );
}

export function updateScenarioTransaction(
  scenarioId: string,
  transactionId: string,
  write: ScenarioTransactionWrite,
): Promise<ScenarioTransaction> {
  return send(
    `/api/scenarios/${encodeURIComponent(scenarioId)}/transactions/${encodeURIComponent(transactionId)}`,
    {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(write),
    },
    200,
    parseScenarioTransaction,
  );
}

export function deleteScenarioTransaction(scenarioId: string, transactionId: string): Promise<void> {
  return send(
    `/api/scenarios/${encodeURIComponent(scenarioId)}/transactions/${encodeURIComponent(transactionId)}`,
    { method: "DELETE" },
    204,
    null,
  );
}

export function createRealization(
  scenarioId: string,
  transactionId: string,
  write: RealizationWrite,
): Promise<ScenarioTransaction> {
  return send(
    `/api/scenarios/${encodeURIComponent(scenarioId)}/transactions/${encodeURIComponent(transactionId)}/realizations`,
    {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(write),
    },
    201,
    parseScenarioTransaction,
  );
}

export function deleteRealization(
  scenarioId: string,
  transactionId: string,
  realizationId: string,
): Promise<void> {
  return send(
    `/api/scenarios/${encodeURIComponent(scenarioId)}/transactions/${encodeURIComponent(transactionId)}/realizations/${encodeURIComponent(realizationId)}`,
    { method: "DELETE" },
    204,
    null,
  );
}

function parseScenarioTransactionArray(value: unknown): ScenarioTransaction[] {
  if (!Array.isArray(value)) throw new TypeError("Resposta de parcelas inválida.");
  return value.map(parseScenarioTransaction);
}

export function generateInstallments(
  scenarioId: string,
  write: GenerateInstallmentsWrite,
): Promise<ScenarioTransaction[]> {
  return send(
    `/api/scenarios/${encodeURIComponent(scenarioId)}/generate-installments`,
    {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(write),
    },
    201,
    parseScenarioTransactionArray,
  );
}

export function readjustInstallments(
  scenarioId: string,
  write: ReadjustWrite,
): Promise<ScenarioTransaction[]> {
  return send(
    `/api/scenarios/${encodeURIComponent(scenarioId)}/readjust`,
    {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(write),
    },
    200,
    parseScenarioTransactionArray,
  );
}
