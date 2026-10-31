import { apiMessage } from "../i18n/apiMessage";
import { useQuery } from "@tanstack/react-query";

import { getSpendingByCategory } from "../api/transactions";
import { browserTimezone } from "./useTransactions";
import { queryKeys } from "../api/queryKeys";

export const spendingByCategoryQueryKey = queryKeys.spendingByCategory;

export function useSpendingByCategory() {
  const timezone = browserTimezone();

  const query = useQuery({
    queryKey: spendingByCategoryQueryKey(timezone),
    queryFn: ({ signal }) => getSpendingByCategory(timezone as string, signal),
    enabled: timezone !== null,
  });

  return {
    spending: query.data,
    isLoading: timezone !== null && query.isLoading,
    error: timezone === null ? new Error(apiMessage("apiTimezoneUnavailable")) : query.error,
    refetch: () => query.refetch(),
  };
}
