import { describe, expect, it } from "vitest";
import { parseTransfer } from "./transfers";

const movement = { transaction_id: "22222222-2222-4222-8222-222222222222", account_id: "11111111-1111-4111-8111-111111111111", account_name: "Synthetic", currency_code: "BRL", amount: "-540" };
const transfer = { id: "88888888-8888-4888-8888-888888888888", source: movement, destination: { ...movement, currency_code: "USD", amount: "100" }, effective_rate: "5.4", fee: null };
describe("parseTransfer", () => {
  it("preserves native decimal strings and an explicitly supplied separate fee", () => {
    expect(parseTransfer(transfer)).toEqual(transfer);
    expect(parseTransfer({ ...transfer, fee: { ...movement, amount: "-10" } }).fee?.amount).toBe("-10");
  });
  it("preserves unavailable native amounts after provider corrections", () => {
    const corrected = { ...transfer, destination: { ...transfer.destination, amount: null }, effective_rate: "", fee: { ...movement, amount: null } };
    expect(parseTransfer(corrected)).toEqual(corrected);
    expect(() => parseTransfer({ ...transfer, source: { ...movement, amount: "" } })).toThrow();
  });
  it("rejects numeric money and missing currencies", () => {
    expect(() => parseTransfer({ ...transfer, source: { ...movement, amount: 540 } })).toThrow();
    expect(() => parseTransfer({ ...transfer, destination: { ...movement, currency_code: "" } })).toThrow();
  });
});
