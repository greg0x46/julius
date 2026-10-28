import { useMemo } from "react";

import type { Period } from "./usePeriod";
import { useHomePeriodBounds } from "./useHomePeriodBounds";
import { useTimeline } from "./useTimeline";
import { useToday } from "./useToday";

const dateFormat = "YYYY-MM-DD";

/**
 * The balance series for the period Home shows, anchored on today. Everything
 * that reads the chart's window (the chart, the negative-balance alert) goes
 * through here, so they share one request and can never disagree.
 */
export function useHomeProjection(period: Period) {
  const today = useToday();
  // "Todo o período" is the one window whose bounds live in the database — the
  // oldest transaction, the last planned installment — so it stays unresolved
  // until useHomePeriodBounds answers.
  const { bounds, wholePeriod, isLoading: rangeLoading, error: rangeError, refetch: refetchRange } =
    useHomePeriodBounds(period);
  const referenceDate = today.format(dateFormat);
  const params = useMemo(
    () => ({
      // referenceDate is always today, never the start of the window: it is
      // what anchors the series to the real balance, so days before it read as
      // history and days after it as projection.
      referenceDate,
      from: bounds ? bounds.from : referenceDate,
      to: bounds ? bounds.to : referenceDate,
    }),
    [referenceDate, bounds],
  );
  const timeline = useTimeline(params, bounds !== null);
  const retry = () => {
    if (rangeError) refetchRange();
    if (timeline.error) timeline.refetch();
  };

  return {
    timeline,
    base: timeline.base,
    bounds,
    wholePeriod,
    referenceDate,
    isLoading: timeline.isLoading || rangeLoading,
    error: timeline.error ?? rangeError,
    retry,
  };
}
