import { keepPreviousData, useQuery } from "@tanstack/react-query";

import type { InvestmentPeriodGrouping, InvestmentPeriodSource } from "../api/contracts";
import { getInvestmentPeriod } from "../api/investmentPortfolio";
import { queryKeys } from "../api/queryKeys";
import type { PeriodRequest } from "../presentation/investmentPeriod";

/**
 * The backend's period view of the book. A null `request` (a period that has
 * not started yet) sends nothing. The previous answer stays on screen while a
 * new period, grouping or origin loads, so the page does not collapse.
 */
export function useInvestmentPortfolioPeriod(
  request: PeriodRequest | null,
  groupBy: InvestmentPeriodGrouping,
  source: InvestmentPeriodSource | null,
) {
  const params = request === null ? null : { ...request, groupBy, source };
  return useQuery({
    queryKey: queryKeys.investmentPeriodFor(params ?? { from: null, to: "", groupBy, source }),
    queryFn: ({ signal }) => getInvestmentPeriod(params!, signal),
    enabled: params !== null,
    placeholderData: keepPreviousData,
  });
}
