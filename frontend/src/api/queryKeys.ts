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
  investmentAssets: ["investmentAssets"],
  investmentAssetClassification: ["investmentAssetClassification"],
  investments: ["investments"],
  investmentTransactions: ["investmentTransactions"],
  investmentWorkspace: ["investmentWorkspace"],
  netWorth: ["netWorth"],
  payables: ["payables"],
  recurringCommitments: ["recurringCommitments"],
  scenarios: ["scenarios"],
  statementImports: ["statement-imports"],
  syncRuns: ["sync-runs"],
  timeline: ["timeline"],
  timelineDataRange: ["timeline-data-range"],
  transactionReconciliation: ["transactionReconciliation"],
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
    part: "accounts" | "portfolios" | "positions" | "operations" | "reconciliations" | "summary",
  ) => [...roots.investmentWorkspace, part] as const,
  investmentDetail: (investmentId: string) => [...roots.investments, investmentId] as const,
  investmentDetailTransactions: (investmentId: string) =>
    [...roots.investments, investmentId, "transactions"] as const,

  payablesOfKind: (kind: PayableKind | null) => [...roots.payables, kind ?? "all"] as const,
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
  transactionsFor: (query: TransactionQuery) => [...roots.transactions, query] as const,
  transactionsInvalidTimezone: [...roots.transactions, "invalid-timezone"] as const,
  transactionsInvalidTimezoneFor: (accountId: string) =>
    [...roots.transactions, "invalid-timezone", accountId] as const,
  categoryBreakdown: (timezone: string | null, period: unknown, classification: unknown) =>
    [...roots.transactions, "category-breakdown", timezone, period, classification] as const,
  spendingByCategory: (timezone: string | null) =>
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

/** An investment ledger write moves cash flow, balance curves and net worth. */
export const invalidateAfterLedgerChange = (client: Client) =>
  invalidateAll(client, [
    roots.investmentWorkspace,
    roots.transactions,
    roots.timeline,
    roots.netWorth,
  ]);

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
) => invalidateAll(client, [[...queryKeys.payablesOfKind(kind), payableId], roots.payables]);

/** Any create/update/delete of a payable. */
export const invalidateAfterPayableChange = (client: Client) => invalidate(client, roots.payables);

/** Rules are saved; their retroactive pass also rewrites transactions. */
export const invalidateAfterAutomationRuleChange = async (
  client: Client,
  options: { transactionsChanged: boolean },
) => {
  await invalidate(client, roots.automationRules);
  if (options.transactionsChanged) await invalidate(client, roots.transactions);
};

/** Toggling or deleting a rule touches only the rule list. */
export const invalidateAutomationRules = (client: Client) =>
  invalidate(client, roots.automationRules);

/** A manual transaction create/update/delete. */
export const invalidateAfterTransactionChange = (client: Client) =>
  invalidate(client, roots.transactions);

/** Marks every transactions query stale without refetching it. */
export const markTransactionsStale = (client: Client) =>
  client.invalidateQueries({ queryKey: roots.transactions, refetchType: "none" });

/** Closing day changes only the account record and the list that shows it. */
export const invalidateAfterAccountClosingDayChange = (client: Client, accountId: string) =>
  Promise.all([
    client.invalidateQueries({ queryKey: roots.accounts, exact: true }),
    client.invalidateQueries({ queryKey: queryKeys.accountDetail(accountId), exact: true }),
  ]);

export const invalidateAfterStatementImport = (client: Client) =>
  invalidateAll(client, [queryKeys.statementImportAccounts, queryKeys.statementImportHistory]);

export const invalidateSyncRunList = (client: Client) => invalidate(client, queryKeys.syncRunsList);
export const invalidateDataSources = (client: Client) => invalidate(client, roots.dataSources);
export const invalidateDataSourcesList = (client: Client) =>
  invalidate(client, queryKeys.dataSourcesList);
export const invalidateTransactionReconciliation = (client: Client, transactionId: string) =>
  invalidate(client, queryKeys.transactionReconciliationOf(transactionId));
export const invalidateRecurringCommitments = (client: Client) =>
  invalidate(client, roots.recurringCommitments);
export const invalidateCategories = (client: Client) => invalidate(client, roots.categories);
export const invalidateAuthSession = (client: Client) => invalidate(client, roots.authSession);
