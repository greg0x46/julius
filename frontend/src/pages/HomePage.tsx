import { DailyAllowanceCard } from "../components/home/DailyAllowanceCard";
import { MonthBalancesCard } from "../components/home/MonthBalancesCard";
import { NegativeBalanceAlert } from "../components/home/NegativeBalanceAlert";
import { NetWorthSummaryCard } from "../components/home/NetWorthSummaryCard";
import { PrimaryGoalCard } from "../components/home/PrimaryGoalCard";
import { TotalOwedCard } from "../components/home/TotalOwedCard";
import { ProjectionSummaryCard } from "../components/home/ProjectionSummaryCard";
import { SpendingByCategoryCard } from "../components/home/SpendingByCategoryCard";
import { PeriodBalanceCard } from "../components/home/PeriodBalanceCard";

import { PeriodNavigator } from "../components/filters/PeriodNavigator";
import { periodPresets } from "../components/filters/periodPresets";
import { Page } from "../components/layout";
import { usePeriod } from "../hooks/usePeriod";

export function HomePage() {
  const { period, setPeriod } = usePeriod();
  return (
    <Page
      title="Início"
      compactMobileHeader
      context={
        <PeriodNavigator
          id="home-period"
          value={[period.from, period.to]}
          presets={periodPresets()}
          onChange={setPeriod}
          reset={{ preset: "this-month", label: "Este mês" }}
          bare
        />
      }
    >
      {/* Areas, not nesting: the layout names where each block goes (see home.css). */}
      <div className="dashboard-layout">
        <div className="dashboard-area-top">
          <div className="home-summary section">
            <DailyAllowanceCard />
            <MonthBalancesCard />
            <div className="home-summary-period">
              <PeriodBalanceCard period={period} />
            </div>
          </div>
        </div>
        <div className="dashboard-area-alert">
          <NegativeBalanceAlert period={period} />
        </div>
        <div className="dashboard-area-chart">
          <ProjectionSummaryCard period={period} />
        </div>
        <div className="dashboard-area-category">
          <SpendingByCategoryCard period={period} />
        </div>
        <div className="dashboard-area-cards">
          <NetWorthSummaryCard />
          <PrimaryGoalCard />
          <TotalOwedCard />
        </div>
      </div>
    </Page>
  );
}
