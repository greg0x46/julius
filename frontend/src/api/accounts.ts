import { apiFetch } from "./transport";
import {
  parseAccount,
  parseAccountBillList,
  parseAccountCardList,
  parseAccountList,
  parseProblem,
  type Account,
  type AccountBill,
  type AccountCard,
  type Problem,
} from "./contracts";
import { ApiError, isAbortError } from "./problems";

const defaultMessage = "Não foi possível carregar as contas.";

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

  if (contentType !== "application/json") {
    throw new ApiError("response", defaultMessage);
  }
  try {
    return parser(await response.json());
  } catch {
    throw new ApiError("response", defaultMessage);
  }
}

const get = { method: "GET", headers: { Accept: "application/json" } } as const;

export function listAccounts(signal?: AbortSignal): Promise<Account[]> {
  return send("/api/accounts", { ...get, signal }, 200, parseAccountList);
}

export function getAccount(accountId: string, signal?: AbortSignal): Promise<Account> {
  return send(`/api/accounts/${encodeURIComponent(accountId)}`, { ...get, signal }, 200, parseAccount);
}

export function listAccountCards(accountId: string, signal?: AbortSignal): Promise<AccountCard[]> {
  return send(
    `/api/accounts/${encodeURIComponent(accountId)}/cards`,
    { ...get, signal },
    200,
    parseAccountCardList,
  );
}

/** Records the user's own closing day for a card, or clears it (null) so the
 *  provider's value takes over again. */
export function setAccountClosingDay(accountId: string, closingDay: number | null): Promise<Account> {
  return send(
    `/api/accounts/${encodeURIComponent(accountId)}/closing-day`,
    {
      method: "PUT",
      headers: { Accept: "application/json", "content-type": "application/json" },
      body: JSON.stringify({ closing_day: closingDay }),
    },
    200,
    parseAccount,
  );
}

export function listAccountBills(accountId: string, signal?: AbortSignal): Promise<AccountBill[]> {
  return send(
    `/api/accounts/${encodeURIComponent(accountId)}/bills`,
    { ...get, signal },
    200,
    parseAccountBillList,
  );
}

export interface ManualAccountWrite {
  name: string;
  currency_code: string;
  opening_balance: string;
  archived: boolean;
}

export function writeManualAccount(accountId: string | null, body: ManualAccountWrite): Promise<Account> {
  return send(accountId ? `/api/accounts/${encodeURIComponent(accountId)}` : "/api/accounts", {
    method: accountId ? "PUT" : "POST",
    headers: { Accept: "application/json", "content-type": "application/json" },
    body: JSON.stringify(body),
  }, accountId ? 200 : 201, parseAccount);
}

export function adjustManualAccount(accountId: string, amount: string, description: string): Promise<Account> {
  return send(`/api/accounts/${encodeURIComponent(accountId)}/adjustments`, {
    method: "POST",
    headers: { Accept: "application/json", "content-type": "application/json" },
    body: JSON.stringify({ amount, description }),
  }, 200, parseAccount);
}

export function listAccountCurrencies(signal?: AbortSignal): Promise<string[]> {
  return send("/api/accounts/currencies", { ...get, signal }, 200, (value) => {
    if (!Array.isArray(value) || !value.every((code) => typeof code === "string" && /^[A-Z]{3}$/.test(code))) {
      throw new TypeError("Lista de moedas inválida.");
    }
    return value as string[];
  });
}
