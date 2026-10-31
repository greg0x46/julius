import { useQuery } from "@tanstack/react-query";

import { getTimelineDataRange } from "../api/timeline";
import { queryKeys } from "../api/queryKeys";
import { useTimezone } from "./useTimezone";

/**
 * The widest window the balance curve can cover. Only the "todo o período"
 * option needs it, so callers pass enabled=false while another period is
 * selected and the request never leaves.
 */
export function useTimelineDataRange(enabled: boolean) {
  const timezone = useTimezone();
  const query = useQuery({
    queryKey: queryKeys.timelineDataRangeFor(timezone),
    queryFn: ({ signal }) => getTimelineDataRange(timezone, signal),
    enabled,
  });

  return {
    range: query.data ?? null,
    isLoading: query.isLoading,
    error: query.error,
    refetch: query.refetch,
  };
}
