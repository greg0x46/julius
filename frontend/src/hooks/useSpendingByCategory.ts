import { useQuery } from "@tanstack/react-query";

import { getSpendingByCategory } from "../api/transactions";
import { useTimezone } from "./useTimezone";
import { queryKeys } from "../api/queryKeys";

export const spendingByCategoryQueryKey = queryKeys.spendingByCategory;

export function useSpendingByCategory() {
  const timezone = useTimezone();

  const query = useQuery({
    queryKey: spendingByCategoryQueryKey(timezone),
    queryFn: ({ signal }) => getSpendingByCategory(timezone, signal),
  });

  return {
    spending: query.data,
    isLoading: query.isLoading,
    error: query.error,
    refetch: () => query.refetch(),
  };
}
