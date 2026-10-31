import { describe, expect, it, vi } from "vitest";

import { transactionResult } from "../../test/transactionFixtures";
import { blankManualDraft, manualDraftFrom, manualDraftIssue, manualDraftToWrite } from "./manualTransactionDraft";

describe("manual money edit precision", () => {
  it.each(["-1.005", "9007199254740993.123456789", "-0.000000000000000001"])("preserves %s on an unchanged save", (amount) => {
    const item = { ...transactionResult.items[0]!, amount, classification: amount.startsWith("-") ? "outflow" as const : "inflow" as const };
    const draft = manualDraftFrom(item, item.account.id, "America/Sao_Paulo");
    expect(manualDraftIssue(draft)).toBeNull();
    expect(manualDraftToWrite(draft).amount).toBe(amount);
  });
});

describe("manual draft dates follow the configured timezone", () => {
  // 02:30Z is still the 28th in São Paulo and already the 29th in Tokyo.
  const item = { ...transactionResult.items[0]!, occurred_at: "2026-07-29T02:30:00Z" };

  it("dates an edited line by the day it happened in the zone", () => {
    expect(manualDraftFrom(item, item.account.id, "America/Sao_Paulo").date.format("YYYY-MM-DD")).toBe("2026-07-28");
    expect(manualDraftFrom(item, item.account.id, "Asia/Tokyo").date.format("YYYY-MM-DD")).toBe("2026-07-29");
  });

  it("dates a new draft today in the zone", () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-07-29T23:30:00Z"));
    try {
      expect(blankManualDraft("a1", "America/Sao_Paulo").date.format("YYYY-MM-DD")).toBe("2026-07-29");
      expect(blankManualDraft("a1", "Asia/Tokyo").date.format("YYYY-MM-DD")).toBe("2026-07-30");
    } finally {
      vi.useRealTimers();
    }
  });
});
