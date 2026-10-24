import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";

import type { NetWorthPoint } from "../../api/contracts";
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
  const valued: NetWorthPoint = {
    date: "2020-01-01",
    valuation_status: "complete",
    total: "2500",
    cash_balance: "2500",
    investment_balance: "0",
  };
  render(
    <NetWorthChart
      points={[
        valued,
        { date: "2020-01-02", valuation_status: "unavailable" },
        { ...valued, date: "2020-01-03", valuation_status: "stale", total: "2100" },
        { ...valued, date: "2020-01-04", total: "2600" },
      ]}
    />,
  );
  const points = JSON.parse(screen.getByTestId("points").textContent!);
  // A stale-rate day is still a valuation, so it is plotted; only the unavailable day is a gap.
  expect(points.map((point: { netWorth: number | null }) => point.netWorth)).toEqual([2500, null, 2100, 2600]);
  expect(screen.getByTestId("line")).toHaveTextContent("false");
  expect(screen.getByRole("img")).toHaveAccessibleName(/lacunas/);
});

const originalTZ = process.env.TZ;
afterEach(() => {
  if (originalTZ === undefined) delete process.env.TZ;
  else process.env.TZ = originalTZ;
});

it("labels a midnight-UTC calendar day as that day in a western timezone", () => {
  process.env.TZ = "America/Sao_Paulo";
  const point: NetWorthPoint = {
    date: "2026-08-18T00:00:00Z",
    valuation_status: "complete",
    total: "100",
    cash_balance: "100",
    investment_balance: "0",
  };
  render(<NetWorthChart points={[point]} />);
  const points = JSON.parse(screen.getByTestId("points").textContent!);
  expect(points[0].date).toBe("18/08");
  expect(screen.getByRole("img")).toHaveAttribute("aria-label", expect.stringContaining("de 18/08 a 18/08"));
});
