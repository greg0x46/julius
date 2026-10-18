import { apiFetch } from "./transport";
import {
  parseInvestmentAccount,
  parseInvestmentAccountList,
  parseInvestmentAsset,
  parseInvestmentAssetList,
  parseInvestmentAssetClassification,
  parseInvestmentOperation,
  parseInvestmentOperationList,
  parseInvestmentPeriod,
  parseInvestmentPosition,
  parseInvestmentPositionList,
  parseInvestmentReconciliation,
  parseInvestmentReconciliationList,
  parseInvestmentSummary,
  parseProblem,
  type InvestmentAccount,
  type InvestmentAccountUpdate,
  type InvestmentAccountWrite,
  type InvestmentAsset,
  type InvestmentAssetWrite,
  type InvestmentAssetClassDefinition,
  type InvestmentOperation,
  type InvestmentOperationWrite,
  type InvestmentPeriod,
  type InvestmentPeriodGrouping,
  type InvestmentPeriodSource,
  type InvestmentPosition,
  type InvestmentPositionUpdate,
  type InvestmentPositionWrite,
  type InvestmentReconciliation,
  type InvestmentReconciliationWrite,
  type InvestmentSummary,
  type Problem,
} from "./contracts";
import { ApiError, isAbortError } from "./problems";

const defaultMessage = "Não foi possível atualizar os investimentos.";

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

const get = { method: "GET", headers: { Accept: "application/json" } } as const;
const json = { "content-type": "application/json", Accept: "application/json" } as const;

export function listInvestmentAccounts(signal?: AbortSignal): Promise<InvestmentAccount[]> {
  return send("/api/investment-accounts", { ...get, signal }, 200, parseInvestmentAccountList);
}

export function createInvestmentAccount(write: InvestmentAccountWrite): Promise<InvestmentAccount> {
  return send(
    "/api/investment-accounts",
    { method: "POST", headers: json, body: JSON.stringify({ ...write, kind: "manual" }) },
    201,
    parseInvestmentAccount,
  );
}

export function updateInvestmentAccount(
  accountId: string,
  write: InvestmentAccountUpdate,
): Promise<InvestmentAccount> {
  return send(
    `/api/investment-accounts/${encodeURIComponent(accountId)}`,
    { method: "PUT", headers: json, body: JSON.stringify(write) },
    200,
    parseInvestmentAccount,
  );
}

export function deleteInvestmentAccount(accountId: string): Promise<void> {
  return send(
    `/api/investment-accounts/${encodeURIComponent(accountId)}`,
    { method: "DELETE", headers: { Accept: "application/json" } },
    204,
    null,
  );
}

export function listInvestmentAssets(signal?: AbortSignal): Promise<InvestmentAsset[]> {
  return send("/api/investment-assets", { ...get, signal }, 200, parseInvestmentAssetList);
}

export function listInvestmentAssetClassification(signal?: AbortSignal): Promise<InvestmentAssetClassDefinition[]> {
  return send("/api/investment-asset-classification", { ...get, signal }, 200, parseInvestmentAssetClassification);
}

export function createInvestmentAsset(write: InvestmentAssetWrite): Promise<InvestmentAsset> {
  return send(
    "/api/investment-assets",
    { method: "POST", headers: json, body: JSON.stringify(write) },
    201,
    parseInvestmentAsset,
  );
}

export function updateInvestmentAsset(
  assetId: string,
  write: InvestmentAssetWrite,
): Promise<InvestmentAsset> {
  return send(
    `/api/investment-assets/${encodeURIComponent(assetId)}`,
    { method: "PUT", headers: json, body: JSON.stringify(write) },
    200,
    parseInvestmentAsset,
  );
}

export function deleteInvestmentAsset(assetId: string): Promise<void> {
  return send(
    `/api/investment-assets/${encodeURIComponent(assetId)}`,
    { method: "DELETE", headers: { Accept: "application/json" } },
    204,
    null,
  );
}

export type InvestmentPositionFilters = {
  accountId?: string | null;
  includeClosed?: boolean;
  source?: "manual" | "synced" | null;
  sourceId?: string | null;
};

export function listInvestmentPositions(
  filters: InvestmentPositionFilters = {},
  signal?: AbortSignal,
): Promise<InvestmentPosition[]> {
  const params = new URLSearchParams();
  if (filters.accountId) params.set("account_id", filters.accountId);
  if (filters.includeClosed) params.set("include_closed", "true");
  if (filters.source) params.set("source", filters.source);
  if (filters.sourceId) params.set("source_id", filters.sourceId);
  const query = params.toString();
  return send(
    `/api/investment-positions${query ? `?${query}` : ""}`,
    { ...get, signal },
    200,
    parseInvestmentPositionList,
  );
}

export function getInvestmentPosition(positionId: string, signal?: AbortSignal): Promise<InvestmentPosition> {
  return send(
    `/api/investment-positions/${encodeURIComponent(positionId)}`,
    { ...get, signal },
    200,
    parseInvestmentPosition,
  );
}

export function createInvestmentPosition(write: InvestmentPositionWrite): Promise<InvestmentPosition> {
  return send(
    "/api/investment-positions",
    { method: "POST", headers: json, body: JSON.stringify(write) },
    201,
    parseInvestmentPosition,
  );
}

export function updateInvestmentPosition(
  positionId: string,
  write: InvestmentPositionUpdate,
): Promise<InvestmentPosition> {
  return send(
    `/api/investment-positions/${encodeURIComponent(positionId)}`,
    { method: "PUT", headers: json, body: JSON.stringify(write) },
    200,
    parseInvestmentPosition,
  );
}

export function deleteInvestmentPosition(positionId: string): Promise<void> {
  return send(
    `/api/investment-positions/${encodeURIComponent(positionId)}`,
    { method: "DELETE", headers: { Accept: "application/json" } },
    204,
    null,
  );
}

export type InvestmentOperationFilters = {
  accountId?: string | null;
  positionId?: string | null;
  source?: "manual" | "synced" | null;
  sourceId?: string | null;
  reconciliationState?: "linked" | "unlinked" | null;
  from?: string | null;
  to?: string | null;
};

export function listInvestmentOperations(
  filters: InvestmentOperationFilters = {},
  signal?: AbortSignal,
): Promise<InvestmentOperation[]> {
  const params = new URLSearchParams();
  if (filters.accountId) params.set("account_id", filters.accountId);
  if (filters.positionId) params.set("position_id", filters.positionId);
  if (filters.source) params.set("source", filters.source);
  if (filters.sourceId) params.set("source_id", filters.sourceId);
  if (filters.reconciliationState) params.set("reconciliation_state", filters.reconciliationState);
  if (filters.from) params.set("from", filters.from);
  if (filters.to) params.set("to", filters.to);
  const query = params.toString();
  return send(
    `/api/investment-operations${query ? `?${query}` : ""}`,
    { ...get, signal },
    200,
    parseInvestmentOperationList,
  );
}

export function createInvestmentOperation(write: InvestmentOperationWrite): Promise<InvestmentOperation> {
  return send(
    "/api/investment-operations",
    { method: "POST", headers: json, body: JSON.stringify(write) },
    201,
    parseInvestmentOperation,
  );
}

export function updateInvestmentOperation(
  operationId: string,
  write: InvestmentOperationWrite,
): Promise<InvestmentOperation> {
  return send(
    `/api/investment-operations/${encodeURIComponent(operationId)}`,
    { method: "PUT", headers: json, body: JSON.stringify(write) },
    200,
    parseInvestmentOperation,
  );
}

export function deleteInvestmentOperation(operationId: string): Promise<void> {
  return send(
    `/api/investment-operations/${encodeURIComponent(operationId)}`,
    { method: "DELETE", headers: { Accept: "application/json" } },
    204,
    null,
  );
}

export type InvestmentTransferWrite = {
  source_position_id: string;
  destination_position_id: string;
  quantity: string;
  occurred_on: string;
  notes?: string | null;
};

export function createInvestmentTransfer(write: InvestmentTransferWrite): Promise<InvestmentOperation[]> {
  return send("/api/investment-transfers",
    { method: "POST", headers: json, body: JSON.stringify(write) }, 201, parseInvestmentOperationList);
}

export function deleteInvestmentTransfer(transferId: string): Promise<void> {
  return send(`/api/investment-transfers/${encodeURIComponent(transferId)}`,
    { method: "DELETE", headers: { Accept: "application/json" } }, 204, null);
}

export function listInvestmentReconciliations(
  filters: { operationId?: string | null; financialTransactionId?: string | null } = {},
  signal?: AbortSignal,
): Promise<InvestmentReconciliation[]> {
  const params = new URLSearchParams();
  if (filters.operationId) params.set("operation_id", filters.operationId);
  if (filters.financialTransactionId) params.set("financial_transaction_id", filters.financialTransactionId);
  const query = params.toString();
  return send(
    `/api/investment-reconciliations${query ? `?${query}` : ""}`,
    { ...get, signal },
    200,
    parseInvestmentReconciliationList,
  );
}

export function createInvestmentReconciliation(
  write: InvestmentReconciliationWrite,
): Promise<InvestmentReconciliation> {
  return send(
    "/api/investment-reconciliations",
    { method: "POST", headers: json, body: JSON.stringify(write) },
    201,
    parseInvestmentReconciliation,
  );
}

export function deleteInvestmentReconciliation(reconciliationId: string): Promise<void> {
  return send(
    `/api/investment-reconciliations/${encodeURIComponent(reconciliationId)}`,
    { method: "DELETE", headers: { Accept: "application/json" } },
    204,
    null,
  );
}

export function getInvestmentSummary(signal?: AbortSignal): Promise<InvestmentSummary> {
  return send("/api/investment-summary", { ...get, signal }, 200, parseInvestmentSummary);
}

export type InvestmentCompoundWrite = {
  operations: InvestmentOperationWrite[];
  reconciliation?: Omit<InvestmentReconciliationWrite, "operation_id">;
};

export function createInvestmentOperations(write: InvestmentCompoundWrite): Promise<InvestmentOperation[]> {
  return send("/api/investment-operations/batch",
    { method: "POST", headers: json, body: JSON.stringify(write) }, 201, parseInvestmentOperationList);
}

export type InvestmentPeriodParams = {
  /** First day of the period; null means since inception. */
  from: string | null;
  to: string;
  groupBy: InvestmentPeriodGrouping;
  source: InvestmentPeriodSource | null;
};

export function getInvestmentPeriod(params: InvestmentPeriodParams, signal?: AbortSignal): Promise<InvestmentPeriod> {
  const query = new URLSearchParams();
  if (params.from) query.set("from", params.from);
  query.set("to", params.to);
  query.set("group_by", params.groupBy);
  if (params.source) query.set("source", params.source);
  return send(`/api/investment-portfolio?${query.toString()}`, { ...get, signal }, 200, parseInvestmentPeriod);
}
