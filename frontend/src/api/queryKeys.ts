import type { QueryClient, QueryKey } from "@tanstack/react-query";

import type { PayableKind, TimelineParams, TransactionQuery } from "./contracts";

/**
 * Every TanStack Query key the app uses, and the invalidation policies that
 * say which of them a kind of write makes stale. Hooks pick a policy instead
 * of listing keys, so a new mutation cannot forget a dependent screen by
 * remembering the wrong strings.
 *
 * Root keys are prefixes: invalidating one drops every key beneath it.
 */

const roots = {
  authSession: ["auth-session"],
  accounts: ["accounts"],
  automationRules: ["automationRules"],
  automationRuleConditionOptions: ["automationRuleConditionOptions"],
  categories: ["categories"],
  dataSources: ["data-sources"],
  goals: ["goals"],
  investmentAssets: ["investmentAssets"],
  investmentAssetClassification: ["investmentAssetClassification"],
  investments: ["investments"],
  investmentTransactions: ["investmentTransactions"],
  investmentWorkspace: ["investmentWorkspace"],
  netWorth: ["netWorth"],
  payables: ["payables"],
  preferences: ["preferences"],
  recurringCommitments: ["recurringCommitments"],
  scenarios: ["scenarios"],
  statementImports: ["statement-imports"],
  syncRuns: ["sync-runs"],
  timeline: ["timeline"],
  timelineDataRange: ["timeline-data-range"],
  transactionReconciliation: ["transactionReconciliation"],
  transactionTransfers: ["transaction-transfer"],
  transactions: ["transactions"],
} as const;

export const queryKeys = {
  ...roots,

  accountDetail: (accountId: string) => [...roots.accounts, accountId] as const,
  accountCards: (accountId: string) => [...roots.accounts, accountId, "cards"] as const,
  accountBills: (accountId: string) => [...roots.accounts, accountId, "bills"] as const,

  dataSourcesList: [...roots.dataSources, "list"] as const,

  investmentTransactionsOf: (positionId: string) =>
    [...roots.investmentTransactions, positionId] as const,
  investmentWorkspacePart: (
    part: "accounts" | "positions" | "operations" | "reconciliations" | "summary",
  ) => [...roots.investmentWorkspace, part] as const,
  // Under the workspace root so every ledger/asset write refreshes it too.
  investmentPeriod: [...roots.investmentWorkspace, "period"] as const,
  investmentPeriodFor: (params: {
    from: string | null;
    to: string;
    groupBy: string;
    source: string | null;
  }) => [...roots.investmentWorkspace, "period", params] as const,
  investmentDetail: (investmentId: string) => [...roots.investments, investmentId] as const,
  investmentDetailTransactions: (investmentId: string) =>
    [...roots.investments, investmentId, "transactions"] as const,

  payablesOfKind: (kind: PayableKind | null) => [...roots.payables, kind ?? "all"] as const,
  payableTotalOwed: [...roots.payables, "total-owed"] as const,
  payableScenarios: (payableId: string) => [...roots.payables, payableId, "scenarios"] as const,

  recurrenceOccurrences: (commitmentId: string) =>
    [...roots.recurringCommitments, commitmentId, "occurrences"] as const,
  recurrenceCandidates: (commitmentId: string, occurrenceDate: string | null, search: string) =>
    [...roots.recurringCommitments, commitmentId, "candidates", occurrenceDate, search] as const,

  scenariosStandalone: [...roots.scenarios, "standalone"] as const,
  scenariosAll: [...roots.scenarios, "all"] as const,
  scenarioDetail: (scenarioId: string) => [...roots.scenarios, scenarioId, "detail"] as const,
  scenarioPlannedTransactions: (scenarioId: string) =>
    [...roots.scenarios, scenarioId, "planned-transactions"] as const,

  statementImportAccounts: [...roots.statementImports, "accounts"] as const,
  statementImportHistory: [...roots.statementImports, "history"] as const,

  syncRunsList: [...roots.syncRuns, "list"] as const,
  syncRunDetail: (runId: string) => [...roots.syncRuns, "detail", runId] as const,

  timelineDataRangeFor: (timezone: string) => [...roots.timelineDataRange, timezone] as const,
  timelineFor: (params: TimelineParams) => [...roots.timeline, params] as const,
  transactionTransfer: (id: string) => [...roots.transactionTransfers, id] as const,
  transactionsFor: (query: TransactionQuery) => [...roots.transactions, query] as const,
  categoryBreakdown: (timezone: string, period: unknown, classification: unknown) =>
    [...roots.transactions, "category-breakdown", timezone, period, classification] as const,
  spendingByCategory: (timezone: string) =>
    [...roots.transactions, "spending-by-category", timezone] as const,
  transactionReconciliationOf: (transactionId: string) =>
    [...roots.transactionReconciliation, transactionId] as const,
} as const;

type Client = Pick<QueryClient, "invalidateQueries">;

const invalidate = (client: Client, queryKey: QueryKey) =>
  client.invalidateQueries({ queryKey });

const invalidateAll = (client: Client, keys: readonly QueryKey[]) =>
  Promise.all(keys.map((queryKey) => invalidate(client, queryKey)));

/** The balance curve and period totals the unified projector builds. */
const projectionViews: readonly QueryKey[] = [roots.timeline, roots.timelineDataRange];

/*
 * Policies. Each returns a promise that settles when the invalidated queries
 * have refetched (markTransactionsStale excepted: it only marks them stale);
 * callers that must not wait simply don't await it.
 */

/** A scenario write changes what the projector includes. */
export const invalidateAfterScenarioChange = (client: Client) =>
  invalidateAll(client, [roots.scenarios, ...projectionViews]);

/** A payable's accounting plan feeds the projector the same way. */
export const invalidateAfterPayablePlanChange = (client: Client, payableId: string) =>
  invalidateAll(client, [queryKeys.payableScenarios(payableId), ...projectionViews]);

/** An investment ledger write moves cash flow, balance curves, net worth and goal progress. */
export const invalidateAfterLedgerChange = (client: Client) =>
  invalidateAll(client, [
    roots.investmentWorkspace,
    roots.transactions,
    roots.timeline,
    roots.netWorth,
    roots.goals,
  ]);

/** A goal's own write only changes the goals list. */
export const invalidateAfterGoalChange = (client: Client) => invalidateAll(client, [roots.goals]);

/** Registering or editing an asset changes the catalog and the workspace. */
export const invalidateAfterInvestmentAssetChange = (client: Client) =>
  invalidateAll(client, [roots.investmentAssets, roots.investmentWorkspace]);

/** Reconciling from the transaction drawer. */
export const invalidateAfterTransactionReconciliation = (client: Client) =>
  invalidateAll(client, [
    roots.transactionReconciliation,
    roots.recurringCommitments,
    roots.timeline,
  ]);

/** Reconciling from the recurrence page, seen from the commitment's end. */
export const invalidateAfterOccurrenceReconciliation = (client: Client, commitmentId: string) =>
  invalidateAll(client, [
    queryKeys.recurrenceOccurrences(commitmentId),
    roots.timeline,
    roots.transactions,
    roots.transactionReconciliation,
  ]);

/** Linking or unlinking a transaction on a payable. */
export const invalidateAfterPayableLinkChange = (
  client: Client,
  kind: PayableKind,
  payableId: string,
) =>
  invalidateAll(client, [
    [...queryKeys.payablesOfKind(kind), payableId],
    roots.payables,
    ...projectionViews,
  ]);

/** Any create/update/delete of a payable; its plan scenarios feed the projector. */
export const invalidateAfterPayableChange = (client: Client) =>
  invalidateAll(client, [roots.payables, roots.scenarios, ...projectionViews]);

/**
 * A rule is saved: its reconcile action changes what the projector expects, and
 * the retroactive pass also rewrites transactions and their reconciliations.
 */
export const invalidateAfterAutomationRuleChange = async (
  client: Client,
  options: { transactionsChanged: boolean },
) => {
  await invalidateAll(client, [roots.automationRules, ...projectionViews]);
  if (options.transactionsChanged) {
    await invalidateAll(client, [
      roots.transactions,
      roots.transactionTransfers,
      roots.transactionReconciliation,
      roots.recurringCommitments,
    ]);
  }
};

/** Toggling or deleting a rule touches only the rule list. */
export const invalidateAutomationRules = (client: Client) =>
  invalidate(client, roots.automationRules);

/** A manual transaction create/update/delete. */
export const invalidateAfterTransactionChange = (client: Client) =>
  invalidateAll(client, [
    roots.transactions,
    roots.accounts,
    roots.timeline,
    roots.timelineDataRange,
    roots.netWorth,
    roots.payables,
    roots.transactionTransfers,
    roots.recurringCommitments,
    roots.transactionReconciliation,
  ]);

export const invalidateAfterManualAccountChange = (client: Client) =>
  invalidateAll(client, [
    roots.accounts,
    roots.transactions,
    roots.timeline,
    roots.timelineDataRange,
    roots.netWorth,
  ]);

/** Marks every transactions query stale without refetching it. */
export const markTransactionsStale = (client: Client) =>
  client.invalidateQueries({ queryKey: roots.transactions, refetchType: "none" });

/** Ignoring a transfer participant can remove the whole relationship; period totals change too. */
export const markTransactionInclusionViewsStale = (client: Client) =>
  Promise.all([
    markTransactionsStale(client),
    client.invalidateQueries({ queryKey: roots.timeline, refetchType: "none" }),
    client.invalidateQueries({ queryKey: roots.transactionTransfers, refetchType: "none" }),
  ]);

/** Closing day changes only the account record and the list that shows it. */
export const invalidateAfterAccountClosingDayChange = (client: Client, accountId: string) =>
  Promise.all([
    client.invalidateQueries({ queryKey: roots.accounts, exact: true }),
    client.invalidateQueries({ queryKey: queryKeys.accountDetail(accountId), exact: true }),
  ]);

/** An import writes ledger entries (and may create an account), not just its own history. */
export const invalidateAfterStatementImport = (client: Client) =>
  invalidateAll(client, [
    queryKeys.statementImportAccounts,
    queryKeys.statementImportHistory,
    roots.accounts,
    roots.transactions,
    roots.timeline,
    roots.timelineDataRange,
    roots.netWorth,
  ]);

export const invalidateSyncRunList = (client: Client) => invalidate(client, queryKeys.syncRunsList);
export const invalidateDataSources = (client: Client) => invalidate(client, roots.dataSources);
export const invalidateDataSourcesList = (client: Client) =>
  invalidate(client, queryKeys.dataSourcesList);
export const invalidateTransactionReconciliation = (client: Client, transactionId: string) =>
  invalidate(client, queryKeys.transactionReconciliationOf(transactionId));
/** A recurring commitment is planned cash flow, so any write to one moves the projection. */
export const invalidateAfterRecurringCommitmentChange = (client: Client) =>
  invalidateAll(client, [roots.recurringCommitments, ...projectionViews]);
/** Category names and kinds are embedded in transaction rows and rule conditions. */
export const invalidateCategories = (client: Client) =>
  Promise.all([
    invalidate(client, roots.categories),
    invalidate(client, roots.automationRules),
    invalidate(client, roots.automationRuleConditionOptions),
    markTransactionsStale(client),
  ]);
export const invalidateAuthSession = (client: Client) => invalidate(client, roots.authSession);

// Not affected by the timezone: the session, the preferences and the currency list.
const timezoneNeutralRoots = new Set([roots.authSession[0], roots.preferences[0], "account-currencies"]);

/**
 * The timezone moves "today" and every day or month boundary, on the server as
 * well, so anything but the session, the preferences and the currency list may
 * answer differently. Dropped rather than refetched: a screen opened later
 * must not flash numbers for the old zone.
 */
export async function discardAfterTimezoneChange(
  client: Pick<QueryClient, "cancelQueries" | "removeQueries">,
) {
  const predicate = (query: { queryKey: QueryKey }) => !timezoneNeutralRoots.has(query.queryKey[0] as string);
  await client.cancelQueries({ predicate });
  client.removeQueries({ predicate });
}
