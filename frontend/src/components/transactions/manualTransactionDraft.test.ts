import { describe, expect, it } from "vitest";

import { transactionResult } from "../../test/transactionFixtures";
import { manualDraftFrom, manualDraftIssue, manualDraftToWrite } from "./manualTransactionDraft";

describe("manual money edit precision", () => {
  it.each(["-1.005", "9007199254740993.123456789", "-0.000000000000000001"])("preserves %s on an unchanged save", (amount) => {
    const item = { ...transactionResult.items[0]!, amount, classification: amount.startsWith("-") ? "outflow" as const : "inflow" as const };
    const draft = manualDraftFrom(item, item.account.id);
    expect(manualDraftIssue(draft)).toBeNull();
    expect(manualDraftToWrite(draft).amount).toBe(amount);
  });
});
