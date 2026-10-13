import { describe, expect, it, vi } from "vitest";

import {
  invalidateAfterAccountClosingDayChange,
  invalidateAfterAutomationRuleChange,
  invalidateAfterInvestmentAssetChange,
  invalidateAfterLedgerChange,
  invalidateAfterOccurrenceReconciliation,
  invalidateAfterPayableChange,
  invalidateAfterPayableLinkChange,
  invalidateAfterPayablePlanChange,
  invalidateAfterScenarioChange,
  invalidateAfterStatementImport,
  invalidateAfterTransactionChange,
  invalidateAfterTransactionReconciliation,
  invalidateAutomationRules,
  markTransactionsStale,
  queryKeys,
} from "./queryKeys";

function spyClient() {
  const invalidateQueries = vi.fn(() => Promise.resolve());
  return { client: { invalidateQueries }, invalidateQueries };
}

describe("queryKeys values", () => {
  it("keeps the runtime key shapes stable", () => {
    expect(queryKeys.scenariosStandalone).toEqual(["scenarios", "standalone"]);
    expect(queryKeys.scenariosAll).toEqual(["scenarios", "all"]);
    expect(queryKeys.scenarioDetail("s1")).toEqual(["scenarios", "s1", "detail"]);
    expect(queryKeys.scenarioPlannedTransactions("s1")).toEqual([
      "scenarios",
      "s1",
      "planned-transactions",
    ]);
    expect(queryKeys.payablesOfKind(null)).toEqual(["payables", "all"]);
    expect(queryKeys.payablesOfKind("debt")).toEqual(["payables", "debt"]);
    expect(queryKeys.payableScenarios("p1")).toEqual(["payables", "p1", "scenarios"]);
    expect(queryKeys.accountCards("a1")).toEqual(["accounts", "a1", "cards"]);
    expect(queryKeys.accountBills("a1")).toEqual(["accounts", "a1", "bills"]);
    expect(queryKeys.investmentWorkspacePart("summary")).toEqual(["investmentWorkspace", "summary"]);
    expect(queryKeys.recurrenceOccurrences("c1")).toEqual(["recurringCommitments", "c1", "occurrences"]);
    expect(queryKeys.recurrenceCandidates("c1", "2026-01-01", "q")).toEqual([
      "recurringCommitments",
      "c1",
      "candidates",
      "2026-01-01",
      "q",
    ]);
    expect(queryKeys.spendingByCategory("UTC")).toEqual(["transactions", "spending-by-category", "UTC"]);
    expect(queryKeys.transactionReconciliationOf("t1")).toEqual(["transactionReconciliation", "t1"]);
    expect(queryKeys.statementImportAccounts).toEqual(["statement-imports", "accounts"]);
    expect(queryKeys.syncRunsList).toEqual(["sync-runs", "list"]);
    expect(queryKeys.syncRunDetail("r1")).toEqual(["sync-runs", "detail", "r1"]);
    expect(queryKeys.dataSourcesList).toEqual(["data-sources", "list"]);
    expect(queryKeys.transactionsInvalidTimezoneFor("a1")).toEqual(["transactions", "invalid-timezone", "a1"]);
    expect(queryKeys.authSession).toEqual(["auth-session"]);
    expect(queryKeys.timeline).toEqual(["timeline"]);
    expect(queryKeys.timelineDataRange).toEqual(["timeline-data-range"]);
    expect(queryKeys.netWorth).toEqual(["netWorth"]);
  });
});

describe("invalidation policies", () => {
  const keysOf = (spy: ReturnType<typeof vi.fn>) =>
    spy.mock.calls.map((call) => (call as unknown as [{ queryKey: unknown }])[0].queryKey);

  it("scenario change refreshes scenarios and the projection views", async () => {
    const { client, invalidateQueries } = spyClient();
    await invalidateAfterScenarioChange(client);
    expect(keysOf(invalidateQueries)).toEqual([["scenarios"], ["timeline"], ["timeline-data-range"]]);
  });

  it("payable plan change refreshes that payable's scenarios and the projection views", async () => {
    const { client, invalidateQueries } = spyClient();
    await invalidateAfterPayablePlanChange(client, "p1");
    expect(keysOf(invalidateQueries)).toEqual([
      ["payables", "p1", "scenarios"],
      ["timeline"],
      ["timeline-data-range"],
    ]);
  });

  it("ledger change refreshes workspace, transactions, timeline and net worth", async () => {
    const { client, invalidateQueries } = spyClient();
    await invalidateAfterLedgerChange(client);
    expect(keysOf(invalidateQueries)).toEqual([
      ["investmentWorkspace"],
      ["transactions"],
      ["timeline"],
      ["netWorth"],
    ]);
  });

  it("awaits every invalidation it starts", async () => {
    let resolve!: () => void;
    const pending = new Promise<void>((r) => (resolve = r));
    const client = { invalidateQueries: vi.fn(() => pending) };
    let settled = false;
    void invalidateAfterLedgerChange(client).then(() => (settled = true));
    await Promise.resolve();
    expect(settled).toBe(false);
    resolve();
    await pending;
    await Promise.resolve();
    expect(settled).toBe(true);
  });

  it("investment asset change refreshes assets and workspace", async () => {
    const { client, invalidateQueries } = spyClient();
    await invalidateAfterInvestmentAssetChange(client);
    expect(keysOf(invalidateQueries)).toEqual([["investmentAssets"], ["investmentWorkspace"]]);
  });

  it("transaction reconciliation refreshes reconciliation, commitments and timeline", async () => {
    const { client, invalidateQueries } = spyClient();
    await invalidateAfterTransactionReconciliation(client);
    expect(keysOf(invalidateQueries)).toEqual([
      ["transactionReconciliation"],
      ["recurringCommitments"],
      ["timeline"],
    ]);
  });

  it("occurrence reconciliation refreshes occurrences, timeline, transactions and reconciliation", async () => {
    const { client, invalidateQueries } = spyClient();
    await invalidateAfterOccurrenceReconciliation(client, "c1");
    expect(keysOf(invalidateQueries)).toEqual([
      ["recurringCommitments", "c1", "occurrences"],
      ["timeline"],
      ["transactions"],
      ["transactionReconciliation"],
    ]);
  });

  it("payable link change refreshes the payable and the payables lists", async () => {
    const { client, invalidateQueries } = spyClient();
    await invalidateAfterPayableLinkChange(client, "debt", "p1");
    expect(keysOf(invalidateQueries)).toEqual([["payables", "debt", "p1"], ["payables"]]);
  });

  it("payable, transaction and rule policies touch only their roots", async () => {
    const payable = spyClient();
    await invalidateAfterPayableChange(payable.client);
    expect(keysOf(payable.invalidateQueries)).toEqual([["payables"]]);

    const tx = spyClient();
    await invalidateAfterTransactionChange(tx.client);
    expect(keysOf(tx.invalidateQueries)).toEqual([["transactions"], ["accounts"], ["timeline"], ["netWorth"], ["transaction-transfer"]]);

    const rules = spyClient();
    await invalidateAutomationRules(rules.client);
    expect(keysOf(rules.invalidateQueries)).toEqual([["automationRules"]]);
  });

  it("automation rule write refreshes transactions only after a retroactive pass", async () => {
    const without = spyClient();
    await invalidateAfterAutomationRuleChange(without.client, { transactionsChanged: false });
    expect(keysOf(without.invalidateQueries)).toEqual([["automationRules"]]);

    const withRetro = spyClient();
    await invalidateAfterAutomationRuleChange(withRetro.client, { transactionsChanged: true });
    expect(keysOf(withRetro.invalidateQueries)).toEqual([["automationRules"], ["transactions"]]);
  });

  it("marks transactions stale without refetching", async () => {
    const { client, invalidateQueries } = spyClient();
    await markTransactionsStale(client);
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ["transactions"], refetchType: "none" });
  });

  it("closing day change invalidates exactly the list and the one account", async () => {
    const { client, invalidateQueries } = spyClient();
    await invalidateAfterAccountClosingDayChange(client, "a1");
    expect(invalidateQueries.mock.calls).toEqual([
      [{ queryKey: ["accounts"], exact: true }],
      [{ queryKey: ["accounts", "a1"], exact: true }],
    ]);
  });

  it("statement import refreshes accounts and history", async () => {
    const { client, invalidateQueries } = spyClient();
    await invalidateAfterStatementImport(client);
    expect(keysOf(invalidateQueries)).toEqual([
      ["statement-imports", "accounts"],
      ["statement-imports", "history"],
    ]);
  });
});
