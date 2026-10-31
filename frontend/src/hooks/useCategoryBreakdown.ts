import { useQuery } from "@tanstack/react-query";
import { getCategoryBreakdown } from "../api/transactions";
import type { CategoryDirection } from "../api/contracts";
import { useTimezone } from "./useTimezone";

import type { Period } from "./usePeriod";
import { queryKeys } from "../api/queryKeys";

export function useCategoryBreakdown(period: Period, classification: CategoryDirection) {
  const timezone = useTimezone();
  return useQuery({
    queryKey: queryKeys.categoryBreakdown(timezone, period, classification),
    queryFn: ({ signal }) => getCategoryBreakdown(timezone, period, classification, signal),
  });
}
