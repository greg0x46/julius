import { DailyAllowanceCard } from "../components/home/DailyAllowanceCard";
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
      {/* Areas, not nesting: the order on a phone (result, chart, categories)
          differs from the wide layout (see home.css). */}
      <div className="dashboard-layout">
        <div className="dashboard-area-result dashboard-stack">
          <PeriodBalanceCard period={period} />
          <DailyAllowanceCard />
        </div>
        <div className="dashboard-area-chart">
          <ProjectionSummaryCard period={period} />
        </div>
        <div className="dashboard-area-category">
          <SpendingByCategoryCard period={period} />
        </div>
      </div>
    </Page>
  );
}
