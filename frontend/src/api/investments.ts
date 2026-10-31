import { apiMessage } from "../i18n/apiMessage";
import { apiFetch } from "./transport";
import {
  parseInvestment,
  parseInvestmentList,
  parseInvestmentTransactionList,
  parseProblem,
  type Investment,
  type InvestmentTransaction,
  type Problem,
} from "./contracts";
import { ApiError, isAbortError } from "./problems";

const defaultMessage = () => apiMessage("apiInvestmentsLoad");

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
      response.status === 404 ? "not_found" : response.status === 409 ? "conflict" : "response",
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

export function listInvestments(signal?: AbortSignal): Promise<Investment[]> {
  return send(
    "/api/investments",
    { method: "GET", headers: { Accept: "application/json" }, signal },
    200,
    parseInvestmentList,
  );
}

export function getInvestment(investmentId: string, signal?: AbortSignal): Promise<Investment> {
  return send(
    `/api/investments/${encodeURIComponent(investmentId)}`,
    { method: "GET", headers: { Accept: "application/json" }, signal },
    200,
    parseInvestment,
  );
}

export function listInvestmentTransactions(
  investmentId: string,
  signal?: AbortSignal,
): Promise<InvestmentTransaction[]> {
  return send(
    `/api/investments/${encodeURIComponent(investmentId)}/transactions`,
    { method: "GET", headers: { Accept: "application/json" }, signal },
    200,
    parseInvestmentTransactionList,
  );
}
