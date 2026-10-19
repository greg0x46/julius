import { fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { expect, it, vi } from "vitest";

import type { TimelineSeries } from "../../api/contracts";
import { ProjectionTimeline, TimelineTooltip } from "./ProjectionTimeline";

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

it("supports percentage scale and intentional empty selection", () => {
  render(<ProjectionTimeline series={series(["1000", "750", "800"])} referenceDate="2026-10-22" />);
  fireEvent.click(screen.getByRole("radio", { name: "Variação %" }));
  expect(JSON.parse(screen.getByTestId("points").textContent!)[1]).toMatchObject({ pastBalance: -25, futureBalance: -25 });
  fireEvent.click(screen.getByRole("checkbox", { name: "Saldo" }));
  expect(screen.getByRole("status")).toHaveTextContent("Selecione ao menos uma métrica");
  expect(screen.queryByTestId("points")).not.toBeInTheDocument();
});

it("offers keyboard day drill-down only for historical days", () => {
  const select = vi.fn();
  render(<ProjectionTimeline series={series(["1000", "750", "800"])} referenceDate="2026-10-22" onSelectDay={select} />);
  fireEvent.click(screen.getByText(/Ver valores/));
  fireEvent.click(screen.getByRole("button", { name: "21/10/2026" }));
  expect(select).toHaveBeenCalledWith("2026-10-21");
  expect(screen.queryByRole("button", { name: "23/10/2026" })).not.toBeInTheDocument();
});

it("rejects a zero baseline without inventing a percentage curve", () => {
  render(<ProjectionTimeline series={series(["0", "750", "800"])} referenceDate="2026-10-22" />);
  fireEvent.click(screen.getByRole("radio", { name: "Variação %" }));
  expect(screen.getByRole("status")).toHaveTextContent("valor inicial positivo");
  expect(screen.queryByTestId("points")).not.toBeInTheDocument();
});

it("deduplicates the realized/forecast crossover and preserves actual money in percentage tooltips", () => {
  render(<TimelineTooltip active label="2026-10-22" scale="percent" originalValues={new Map([["2026-10-22", { balance: "750.01" }]])}
    payload={[{ name: "Saldo realizado", value: -25 }, { name: "Saldo projetado", value: -25 }]} />);
  expect(screen.getByText(/Saldo realizado:/)).toHaveTextContent("-25% · R$ 750,01");
  expect(screen.queryByText(/Saldo projetado:/)).not.toBeInTheDocument();
});
