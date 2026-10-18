import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { expect, it, vi } from "vitest";

import type { TimelineSeries } from "../../api/contracts";
import { ProjectionTimeline } from "./ProjectionTimeline";

vi.mock("recharts", () => ({
  ResponsiveContainer: ({ children }: { children: ReactNode }) => children,
  LineChart: ({ data, children }: { data: unknown; children: ReactNode }) => (
    <div>
      <pre data-testid="points">{JSON.stringify(data)}</pre>
      {children}
    </div>
  ),
  Line: () => null,
  CartesianGrid: () => null,
  ReferenceDot: () => null,
  ReferenceLine: () => null,
  Tooltip: () => null,
  XAxis: () => null,
  YAxis: () => null,
}));

function series(balances: string[]): TimelineSeries {
  const points = balances.map((balance, index) => ({
    date: `2026-10-${index + 21}`, balance,
    inflow: "0", outflow: "0", lowest_tier: "confirmado" as const,
  }));
  return { starting_balance: "1000", points, entries: [], lowest_balance: points[1], first_negative: null };
}

it("plots the server's end-of-day balances including movements due today for base and simulation", () => {
  render(
    <ProjectionTimeline
      series={series(["1000", "750", "800"])}
      simulation={series(["1000", "500", "550"])}
      referenceDate="2026-10-22"
    />,
  );

  const points = JSON.parse(screen.getByTestId("points").textContent!);
  expect(points).toEqual([
    { date: "2026-10-21", balance: 1000, pastBalance: 1000, simulationBalance: 1000 },
    { date: "2026-10-22", balance: 750, pastBalance: 750, futureBalance: 750, simulationBalance: 500 },
    { date: "2026-10-23", balance: 800, futureBalance: 800, simulationBalance: 550 },
  ]);
});
