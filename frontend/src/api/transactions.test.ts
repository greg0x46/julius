import { describe, expect, it, vi } from "vitest";

import { formatMoney } from "../presentation/money";
import { transactionQuery, transactionResult, transactionJsonResponse } from "../test/transactionFixtures";
import { ApiError } from "./problems";
import { createManualTransaction, queryTransactions, setTransactionInclusion } from "./transactions";
import { transactionId } from "../test/transactionFixtures";

describe("transaction API boundary", () => {
  it("forwards cancellation and parses exact monetary strings", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(transactionJsonResponse());
    const controller = new AbortController();
    const result = await queryTransactions(transactionQuery, controller.signal);
    expect(result.items[0]?.amount).toBe("123.4500");
    controller.abort();
    expect(fetchMock.mock.calls[0]?.[1]?.signal?.aborted).toBe(true);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/transactions/query",
      expect.objectContaining({ method: "POST", signal: expect.any(AbortSignal) }),
    );
  });

  it.each([
    ["monetary JSON numbers", { ...transactionResult, totals: [{ ...transactionResult.totals[0]!, inflow: 1 }] }],
    ["extra private fields", { ...transactionResult, raw_import_id: "private" }],
  ])("rejects %s", async (_name, payload) => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(transactionJsonResponse(payload));
    await expect(queryTransactions(transactionQuery)).rejects.toBeInstanceOf(ApiError);
  });

  it("handles application/problem+json", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      transactionJsonResponse(
        { type: "/problems/query", title: "Indisponível", status: 503, detail: "Tente novamente." },
        { status: 503, headers: { "content-type": "application/problem+json" } },
      ),
    );
    await expect(queryTransactions(transactionQuery)).rejects.toMatchObject({
      message: "Tente novamente.",
    });
  });
});

describe("transaction inclusion API boundary", () => {
  it("sends an explicit idempotent PUT and validates the confirmation", async () => {
    const response = {
      transaction_id: transactionId,
      state: "ignored",
      changed_at: "2026-07-30T13:00:00Z",
    };
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      transactionJsonResponse(response),
    );
    await expect(setTransactionInclusion(transactionId, "ignored")).resolves.toEqual(response);
    expect(fetchMock).toHaveBeenCalledWith(
      `/api/transactions/${transactionId}/inclusion`,
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({ state: "ignored" }),
      }),
    );
  });

  it("rejects invalid confirmations and preserves problem details", async () => {
    vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        transactionJsonResponse({ transaction_id: transactionId, state: "ignored" }),
      )
      .mockResolvedValueOnce(
        transactionJsonResponse(
          { type: "/problems/write", title: "Indisponível", status: 503, detail: "Repita." },
          { status: 503, headers: { "content-type": "application/problem+json" } },
        ),
      );
    await expect(setTransactionInclusion(transactionId, "ignored")).rejects.toMatchObject({
      message: "A confirmação da decisão é inválida.",
    });
    await expect(setTransactionInclusion(transactionId, "ignored")).rejects.toMatchObject({
      message: "Repita.",
    });
  });
});

describe("string-only money formatting", () => {
  it.each([
    // BRL delegates to formatBRL: always "R$" and two decimals.
    ["9007199254740993.1200", "BRL", "R$\u00a09.007.199.254.740.993,12"],
    ["3", "BRL", "R$\u00a03,00"],
    ["-0.50", "BRL", "-R$\u00a00,50"],
    // Other currencies keep their code and any extra precision, padded to 2.
    ["-0.50", "USD", "-USD\u00a00,50"],
    ["3", "USD", "USD\u00a03,00"],
    ["1234.5678", "USD", "USD\u00a01.234,5678"],
  ])("formats %s %s without going through floats", (value, currency, expected) => {
    expect(formatMoney(value, currency)).toBe(expected);
  });
});

describe("money HTTP round trip", () => {
  it.each(["-1.005", "9007199254740993.123456789012345678", "0.000000000000000001"])("retains %s through serialization and response parsing", async (amount) => {
    const item = { ...transactionResult.items[0]!, amount, amount_in_account_currency: amount };
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(transactionJsonResponse(item, { status: 201 }));
    const result = await createManualTransaction({ account_id: item.account.id, description: "Synthetic precision", amount, occurred_at: "2026-07-30", category_id: null }, "precision-test");
    expect(result.amount).toBe(amount);
    const requestBody = JSON.parse(String(fetchMock.mock.calls.at(-1)?.[1]?.body));
    expect(requestBody.amount).toBe(amount);
  });
});
