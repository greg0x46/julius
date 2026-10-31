import { useQuery } from "@tanstack/react-query";

import { queryTransactions } from "../api/transactions";
import type {
  TransactionFilters,
  TransactionGrouping,
  TransactionQuery,
} from "../api/contracts";
import { queryKeys } from "../api/queryKeys";
import { monthRange, zonedToday } from "../presentation/timezone";
import { useTimezone } from "./useTimezone";

export const DEFAULT_PAGE_SIZE = 50;

/** The filters for the whole current month, "current" being as the day is in `timezone`. */
export function currentMonthFilters(timezone: string, now = new Date()): TransactionFilters {
  const [start, end] = monthRange(zonedToday(timezone, now));
  return {
    date_from: start,
    date_to: end,
    description: null,
    account_ids: [],
    credit_card: null,
    card_balance: null,
    category_ids: [],
    classification: null,
    amount_min: null,
    amount_max: null,
    uncategorized: null,
  };
}

export function transactionQueryKey(query: TransactionQuery) {
  return queryKeys.transactionsFor(query);
}

export function useTransactions(
  filters: TransactionFilters,
  groupBy: TransactionGrouping,
  page: number,
  pageSize = DEFAULT_PAGE_SIZE,
) {
  const timezone = useTimezone();
  const query: TransactionQuery = { timezone, group_by: groupBy, page, page_size: pageSize, filters };
  const result = useQuery({
    queryKey: transactionQueryKey(query),
    queryFn: ({ signal }) => queryTransactions(query, signal),
    retry: false,
  });
  return { ...result, query };
}
