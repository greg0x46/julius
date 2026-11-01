import { useTranslation } from "../../i18n/context";
import { Alert, Button, Skeleton, Switch } from "../ui";
import { useMemo, useState } from "react";

import type {
  InvestmentPeriodGroup,
  InvestmentPeriodGrouping,
  InvestmentPeriodPosition,
  InvestmentPosition,
} from "../../api/contracts";
import { useInvestmentPortfolioPeriod } from "../../hooks/useInvestmentPortfolioPeriod";
import type { PeriodRequest } from "../../presentation/investmentPeriod";
import { problemMessage } from "../../presentation/reporting";
import { UnavailableState } from "../AsyncState";
import { EmptyState, ListToolbar, SearchField, Section } from "../layout";
import { ReportValuation } from "../shared/ReportValuation";
import type { RecordMenuItem } from "../shared/RecordMenu";
import { InvestmentPeriodSummary } from "./InvestmentPeriodSummary";
import { InvestmentPositionsPeriod } from "./InvestmentPositionsPeriod";

/**
 * The period view of the book: the backend's summary and positions for the
 * chosen window, subtotaled by the grouping picked on the page tabs. Search
 * and the closed switch only choose which rows are listed, never what a total
 * is.
 */
export function InvestmentPeriodView({
  request,
  groupBy,
  workspacePositions,
  onNewPosition,
  positionMenu,
  groupMenu,
}: {
  /** Null when the chosen period has not started yet. */
  request: PeriodRequest | null;
  /** How the list is subtotaled; chosen on the page's tabs. */
  groupBy: InvestmentPeriodGrouping;
  workspacePositions: InvestmentPosition[];
  onNewPosition: () => void;
  positionMenu: (position: InvestmentPosition | undefined) => RecordMenuItem[];
  groupMenu: (group: InvestmentPeriodGroup) => RecordMenuItem[];
}) {
  const { t, language } = useTranslation();
  const [search, setSearch] = useState("");
  const [showClosed, setShowClosed] = useState(false);
  const query = useInvestmentPortfolioPeriod(request, groupBy);
  const data = request === null ? undefined : query.data;
  const byId = useMemo(
    () => new Map(workspacePositions.map((position) => [position.id, position])),
    [workspacePositions],
  );
  const allPositions = useMemo(() => data?.groups.flatMap((group) => group.positions) ?? [], [data]);

  if (request === null) {
    return (
      <Section title={t("positions")}>
        <EmptyState title={t("periodNotStarted")} hint={t("periodNotStartedHint")} />
      </Section>
    );
  }

  const updating = query.isFetching;
  const href = (position: InvestmentPeriodPosition) => {
    const linked = byId.get(position.id)?.linked_investment_id;
    return linked ? `/investimentos/${linked}` : null;
  };

  return (
    <>
      {query.isPending && (
        <div className="section" role="status" aria-label={t("loadingInvestments")}>
          <div className="section-body">
            <Skeleton active paragraph={{ rows: 5 }} />
          </div>
        </div>
      )}
      {query.isError && !data && (
        <UnavailableState onRetry={() => void query.refetch()}>
          {problemMessage(query.error) ?? t("investmentPeriodError")}
        </UnavailableState>
      )}
      {query.isError && data && (
        <Alert
          type="warning"
          showIcon
          message={t("possiblyStaleData")}
          description={<Button onClick={() => void query.refetch()}>{t("retry")}</Button>}
          style={{ marginBottom: 16 }}
        />
      )}
      {data && (
        <ReportValuation valuation={data}>
          <InvestmentPeriodSummary data={data} positions={allPositions} updating={updating} />
          <Section
            title={t("positions")}
            trailing={t(data.totals.positions_total === 1 ? "positionCountOne" : "positionCountMany", { count: data.totals.positions_total.toLocaleString(language) })}
            className="investment-positions-section"
          >
            <ListToolbar
              label={t("positionControls")}
              start={
                <SearchField
                  id="investment-search"
                  label={t("searchAsset")}
                  placeholder={t("nameOrTicker")}
                  value={search}
                  onChange={setSearch}
                />
              }
              end={
                <label className="list-toolbar-switch">
                  <Switch size="small" checked={showClosed} onChange={setShowClosed} />
                  <span>{t("showClosed")}</span>
                </label>
              }
            />
            <div aria-busy={updating} className={updating ? "investment-results is-updating" : "investment-results"}>
              <InvestmentPositionsPeriod
                data={data}
                search={search}
                showClosed={showClosed}
                hrefOf={href}
                positionMenu={(position) => positionMenu(byId.get(position.id))}
                groupMenu={groupMenu}
                empty={
                  <EmptyState
                    title={t("noPositionsInPeriod")}
                    hint={t("noPositionsInPeriodHint")}
                    action={<Button onClick={onNewPosition}>{t("newPosition")}</Button>}
                  />
                }
              />
            </div>
          </Section>
        </ReportValuation>
      )}
    </>
  );
}
