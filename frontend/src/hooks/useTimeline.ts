import { useQuery } from "@tanstack/react-query";

import { getTimeline } from "../api/timeline";
import type { TimelineParams } from "../api/contracts";
import { queryKeys } from "../api/queryKeys";

/**
 * enabled=false holds the request back while the caller still lacks a window
 * to ask about — the Home dashboard's "todo o período" waits on the data
 * range before it knows its own bounds.
 */
export function useTimeline(params: TimelineParams, enabled = true) {
  const query = useQuery({
    queryKey: queryKeys.timelineFor(params),
    queryFn: ({ signal }) => getTimeline(params, signal),
    enabled,
  });

  return {
    valuation: query.data ?? {},
    currencyCode: query.data?.currency_code ?? "BRL",
    base: query.data?.base ?? null,
    periodTotals: query.data?.period_totals ?? null,
    simulation: query.data?.simulation ?? null,
    scenarioImpacts: query.data?.scenario_impacts ?? [],
    isLoading: enabled && query.isLoading,
    error: query.error,
    refetch: query.refetch,
  };
}
