import { apiMessage } from "../i18n/apiMessage";
import { useQuery } from "@tanstack/react-query";
import { getCategoryBreakdown } from "../api/transactions";
import type { CategoryDirection } from "../api/contracts";
import { browserTimezone } from "./useTransactions";

import type { Period } from "./usePeriod";
import { queryKeys } from "../api/queryKeys";

export function useCategoryBreakdown(period: Period, classification: CategoryDirection) {
  const timezone = browserTimezone();
  const query = useQuery({
    queryKey: queryKeys.categoryBreakdown(timezone, period, classification),
    queryFn: ({ signal }) => getCategoryBreakdown(timezone!, period, classification, signal),
    enabled: timezone !== null,
  });
  return { ...query, error: timezone === null ? new Error(apiMessage("apiTimezoneUnavailable")) : query.error };
}
