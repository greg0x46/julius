import { describe, expect, it, vi } from "vitest";

import { bankAccount } from "../test/accountFixtures";
import { adjustManualAccount, listAccountCurrencies, writeManualAccount } from "./accounts";
import { parseAccount } from "./contracts";

const manual = { ...bankAccount, origin: "manual" as const, external_id: null, opening_balance: "100.001", balance: "100.001", currency_code: "USD" };
const response = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status, headers: { "Content-Type": "application/json" } });

describe("manual account API", () => {
  it("accepts accounts without a provider identity and retains exact native values", () => {
    expect(parseAccount(manual)).toEqual(manual);
    expect(() => parseAccount({ ...manual, external_id: "fake" })).toThrow();
  });

  it("creates and updates through the shared transport", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(response(manual, 201)).mockResolvedValueOnce(response(manual));
    const body = { name: "USD", currency_code: "USD", opening_balance: "100.001", archived: false };
    await expect(writeManualAccount(null, body)).resolves.toEqual(manual);
    await expect(writeManualAccount(manual.id, { ...body, archived: true })).resolves.toEqual(manual);
    expect(fetchMock).toHaveBeenNthCalledWith(1, "/api/accounts", expect.objectContaining({ method: "POST", body: JSON.stringify(body) }));
    expect(new Headers(fetchMock.mock.calls[0]?.[1]?.headers).get("X-Julius-Request")).toBe("1");
    expect(fetchMock).toHaveBeenNthCalledWith(2, `/api/accounts/${manual.id}`, expect.objectContaining({ method: "PUT" }));
  });

  it("sends signed adjustments and exposes supported currencies", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(response(manual)).mockResolvedValueOnce(response(["BRL", "EUR", "USD"]));
    await adjustManualAccount(manual.id, "-5.50", "Correção");
    expect(fetchMock).toHaveBeenNthCalledWith(1, `/api/accounts/${manual.id}/adjustments`, expect.objectContaining({ body: JSON.stringify({ amount: "-5.50", description: "Correção" }) }));
    await expect(listAccountCurrencies()).resolves.toEqual(["BRL", "EUR", "USD"]);
  });

  it("keeps currency validation feedback", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({ type: "/problems/account-has-movements", title: "Conta com movimentações", detail: "A moeda não pode ser alterada.", status: 409 }), { status: 409, headers: { "Content-Type": "application/problem+json" } }));
    await expect(writeManualAccount(manual.id, { name: "USD", currency_code: "EUR", opening_balance: "100", archived: false })).rejects.toMatchObject({ message: "A moeda não pode ser alterada." });
  });
});
