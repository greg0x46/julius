import { apiMessage } from "../i18n/apiMessage";
import type { ManualTransactionWrite } from "./contracts";
import { parseProblem } from "./contracts";
import { decimal, isUuid, requiredRecord } from "./contracts/shared";
import { apiFetch } from "./transport";
import { ApiError, isAbortError } from "./problems";

export interface TransferMovement {
  transaction_id: string;
  account_id: string;
  account_name: string;
  amount: string | null;
  currency_code: string;
}
export interface AccountTransfer {
  id: string;
  source: TransferMovement;
  destination: TransferMovement;
  effective_rate: string;
  fee: TransferMovement | null;
}
export type TransferWrite = {
  source_transaction_id: string;
  destination_transaction_id: string;
  fee_transaction_id?: string;
} | {
  source: ManualTransactionWrite;
  destination: ManualTransactionWrite;
  fee?: ManualTransactionWrite;
};

function parseMovement(value: unknown): TransferMovement {
  const row = requiredRecord(value, ["transaction_id", "account_id", "account_name", "amount", "currency_code"]);
  if (typeof row.transaction_id !== "string" || !isUuid(row.transaction_id) || typeof row.account_id !== "string" || !isUuid(row.account_id) || typeof row.account_name !== "string" || typeof row.currency_code !== "string" || !/^[A-Z]{3}$/.test(row.currency_code)) throw new TypeError("Transferência inválida.");
  return { transaction_id: row.transaction_id, account_id: row.account_id, account_name: row.account_name, amount: row.amount === null ? null : decimal(row.amount), currency_code: row.currency_code };
}
export function parseTransfer(value: unknown): AccountTransfer {
  const row = requiredRecord(value, ["id", "source", "destination", "effective_rate", "fee"]);
  if (typeof row.id !== "string" || !isUuid(row.id)) throw new TypeError("Transferência inválida.");
  return { id: row.id, source: parseMovement(row.source), destination: parseMovement(row.destination), effective_rate: row.effective_rate === "" ? "" : decimal(row.effective_rate), fee: row.fee === null ? null : parseMovement(row.fee) };
}

async function transferRequest(url: string, method: "GET" | "POST" | "DELETE", write?: TransferWrite, signal?: AbortSignal): Promise<unknown> {
  let response: Response;
  try {
    response = await apiFetch(url, { method, headers: { "Content-Type": "application/json" }, body: write ? JSON.stringify(write) : undefined, signal });
  } catch (error) {
    if (isAbortError(error)) throw error;
    throw new ApiError("transport", apiMessage("apiCommunication"));
  }
  if (response.status === 204 && method === "DELETE") return null;
  const failure = apiMessage("apiTransferUnconfirmed");
  let body: unknown;
  try { body = await response.json(); }
  catch { throw new ApiError("response", failure); }
  if (!response.ok) {
    let problem;
    try { problem = parseProblem(body); } catch { problem = undefined; }
    throw new ApiError("response", problem?.detail || problem?.title || failure, problem);
  }
  const expected = method === "POST" ? 201 : 200;
  if (response.status !== expected || response.headers.get("Content-Type")?.split(";")[0] !== "application/json") throw new ApiError("response", failure);
  return body;
}
export async function getTransactionTransfer(id: string, signal?: AbortSignal): Promise<AccountTransfer | null> {
  const value = await transferRequest(`/api/transactions/${encodeURIComponent(id)}/transfer`, "GET", undefined, signal);
  return value === null ? null : parseTransfer(value);
}
export async function createTransfer(write: TransferWrite): Promise<AccountTransfer> {
  return parseTransfer(await transferRequest("/api/transfers", "POST", write));
}
export async function deleteTransfer(id: string): Promise<void> {
  await transferRequest(`/api/transfers/${encodeURIComponent(id)}`, "DELETE");
}
