import { useQuery } from "@tanstack/react-query";

import { getPayableTotalOwed } from "../api/payables";
import { queryKeys } from "../api/queryKeys";

/** What is still owed: open debts plus the card bills still accruing. Under the payables root, so a payable write refreshes it. */
export function usePayableTotalOwed() {
  const query = useQuery({
    queryKey: queryKeys.payableTotalOwed,
    queryFn: ({ signal }) => getPayableTotalOwed(signal),
  });

  return {
    total: query.data ?? null,
    isLoading: query.isLoading,
    error: query.error,
    refetch: query.refetch,
  };
}
