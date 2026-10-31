import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  getAccount,
  listAccountBills,
  listAccountCards,
  setAccountClosingDay,
} from "../api/accounts";
import { queryTransactions } from "../api/transactions";
import { ApiError } from "../api/problems";
import type { Account, TransactionItem, TransactionQuery } from "../api/contracts";
import { transactionQueryKey } from "./useTransactions";
import { useTimezone } from "./useTimezone";
import { invalidateAfterAccountClosingDayChange, queryKeys } from "../api/queryKeys";

const RECENT_TRANSACTIONS_LIMIT = 10;

export type AccountDetailState = {
  accountId: string;
  snapshot: Account | null;
  freshness: "loading" | "fresh" | "stale" | "not_found" | "unavailable";
  retrying: boolean;
};

// recentTransactionsQuery reuses POST /api/transactions/query rather than a
// dedicated endpoint: it already resolves classification, effective money,
// inclusion state, category and card info for each row, and the transaction
// components on this page render exactly that shape. No date range — the
// point is the most recent activity, whenever it happened.
function recentTransactionsQuery(accountId: string, timezone: string): TransactionQuery {
  return {
    timezone,
    group_by: "none",
    page: 1,
    page_size: RECENT_TRANSACTIONS_LIMIT,
    filters: {
      date_from: null,
      date_to: null,
      description: null,
      account_ids: [accountId],
      category_ids: [],
      classification: null,
      amount_min: null,
      amount_max: null,
      uncategorized: null,
    },
  };
}

/** Stable while loading, so consumers can compare results by identity. */
const noTransactions: TransactionItem[] = [];

export function useAccountDetail(accountId: string) {
  const detailQuery = useQuery({
    queryKey: queryKeys.accountDetail(accountId),
    queryFn: ({ signal }) => getAccount(accountId, signal),
  });
  const snapshot = detailQuery.data ?? null;
  const freshness: AccountDetailState["freshness"] = detailQuery.isPending
    ? "loading"
    : detailQuery.isError
      ? snapshot !== null
        ? "stale"
        : detailQuery.error instanceof ApiError && detailQuery.error.kind === "not_found"
          ? "not_found"
          : "unavailable"
      : "fresh";
  const state: AccountDetailState = {
    accountId,
    snapshot,
    freshness,
    retrying: detailQuery.isFetching && snapshot !== null,
  };

  const cardsQuery = useQuery({
    queryKey: queryKeys.accountCards(accountId),
    queryFn: ({ signal }) => listAccountCards(accountId, signal),
  });

  const billsQuery = useQuery({
    queryKey: queryKeys.accountBills(accountId),
    queryFn: ({ signal }) => listAccountBills(accountId, signal),
  });

  const queryClient = useQueryClient();
  const closingDayMutation = useMutation({
    mutationFn: (closingDay: number | null) => setAccountClosingDay(accountId, closingDay),
    onSuccess: () => {
      // Only the account record changed, and the list screen shows the same
      // closing day — so both go stale, exactly. A prefix invalidation would
      // also drop this account's cards, bills and recent transactions, none
      // of which a closing day can affect.
      void invalidateAfterAccountClosingDayChange(queryClient, accountId);
    },
  });

  const timezone = useTimezone();
  // Keyed like every other transactions query (transactionQueryKey), not
  // nested under queryKeys.accounts: useTransactionInclusion/useTransactionCategory/
  // useManualTransaction all invalidate the ["transactions", ...] prefix, and
  // this list needs to pick up those edits too — the panel this page opens
  // writes through the same hooks.
  const transactionsQuery = useQuery({
    queryKey: transactionQueryKey(recentTransactionsQuery(accountId, timezone)),
    queryFn: ({ signal }) => queryTransactions(recentTransactionsQuery(accountId, timezone), signal),
  });

  return {
    state,
    retry: () => void detailQuery.refetch({ cancelRefetch: true }),
    setClosingDay: closingDayMutation.mutateAsync,
    savingClosingDay: closingDayMutation.isPending,
    cards: cardsQuery.data ?? [],
    cardsLoading: cardsQuery.isLoading,
    cardsError: cardsQuery.error,
    bills: billsQuery.data ?? [],
    billsLoading: billsQuery.isLoading,
    billsError: billsQuery.error,
    transactions: transactionsQuery.data?.items ?? noTransactions,
    transactionsLoading: transactionsQuery.isLoading,
    transactionsError: transactionsQuery.error,
  };
}
