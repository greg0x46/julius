import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { expect, it, vi } from "vitest";

import type { NetWorthSnapshot } from "../../api/contracts";
import { NetWorthChart } from "./NetWorthChart";

vi.mock("recharts", () => ({
  ResponsiveContainer: ({ children }: { children: ReactNode }) => children,
  LineChart: ({ data, children }: { data: unknown; children: ReactNode }) => (
    <div>
      <pre data-testid="points">{JSON.stringify(data)}</pre>
      {children}
    </div>
  ),
  Line: ({ connectNulls }: { connectNulls: boolean }) => <span data-testid="line">{String(connectNulls)}</span>,
  CartesianGrid: () => null,
  Tooltip: () => null,
  XAxis: () => null,
  YAxis: () => null,
}));

it("leaves days that cannot be valued as gaps, not zeros", () => {
  const valued: NetWorthSnapshot = {
    captured_at: "2020-01-01",
    is_backfilled: false,
    valuation_version: 1,
    valuation_status: "complete",
    net_worth: "2500",
    cash_balance: "2500",
    investment_balance: "0",
    credit_card_balance: "0",
    payables_debt: "0",
    total_assets: "2500",
    total_liabilities: "0",
  };
  render(
    <NetWorthChart
      snapshots={[
        valued,
        { captured_at: "2020-01-02", is_backfilled: true, valuation_version: 1, valuation_status: "unavailable" },
        { ...valued, captured_at: "2020-01-03", valuation_version: 0, valuation_status: "legacy", net_worth: "2100" },
        { ...valued, captured_at: "2020-01-04", net_worth: "2600" },
      ]}
    />,
  );
  const points = JSON.parse(screen.getByTestId("points").textContent!);
  // A legacy row is a real (BRL-only) observation, so it is plotted; only the unavailable day is a gap.
  expect(points.map((point: { netWorth: number | null }) => point.netWorth)).toEqual([2500, null, 2100, 2600]);
  expect(screen.getByTestId("line")).toHaveTextContent("false");
  expect(screen.getByRole("img")).toHaveAccessibleName(/lacunas/);
});
