import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { TimelineEntry, TimelineSeries } from "../../api/contracts";
import { ProjectionTimeline } from "./ProjectionTimeline";

function series(balances: string[]): TimelineSeries {
  const points = balances.map((balance, index) => ({
    date: `2026-10-${index + 21}`,
    balance,
    inflow: "0",
    outflow: "0",
    lowest_tier: "confirmado" as const,
  }));
  return {
    starting_balance: "1000",
    points,
    entries: [],
    lowest_balance: points[1] ?? points[0],
    first_negative: null,
  };
}
it("shows server balances due today for base and simulation and opens only realized days", () => {
  const onSelectDay = vi.fn();
  const { container } = render(
    <ProjectionTimeline
      series={series(["1000", "750", "800"])}
      simulation={series(["1000", "500", "550"])}
      referenceDate="2026-10-22"
      onSelectDay={onSelectDay}
    />,
  );
  expect(container.querySelector('[data-series="past"]')).toHaveAttribute(
    "d",
    expect.stringContaining("H"),
  );
  expect(container.querySelector('[data-series="future"]')).toHaveAttribute(
    "stroke-dasharray",
    "5 4",
  );
  fireEvent.change(screen.getByRole("combobox"), { target: { value: "1" } });
  expect(screen.getByText(/Saldo realizado:.*750,00/)).toBeInTheDocument();
  expect(screen.getByText(/Saldo \(simulação\):.*500,00/)).toBeInTheDocument();
  expect(
    screen.queryByText(/Saldo projetado:.*750,00/),
  ).not.toBeInTheDocument();
  fireEvent.click(
    screen.getByRole("button", { name: "Ver transações do dia" }),
  );
  expect(onSelectDay).toHaveBeenCalledWith("2026-10-22");
  fireEvent.change(screen.getByRole("combobox"), { target: { value: "2" } });
  expect(
    screen.queryByRole("button", { name: "Ver transações do dia" }),
  ).not.toBeInTheDocument();
  expect(onSelectDay).toHaveBeenCalledTimes(1);
});
it("keeps fractional balances exact in the tooltip", () => {
  render(
    <ProjectionTimeline
      series={series(["9007199254740993.12"])}
      referenceDate="2026-10-21"
    />,
  );
  fireEvent.change(screen.getByRole("combobox"), { target: { value: "0" } });
  expect(screen.getByText(/9\.007\.199\.254\.740\.993,12/)).toBeInTheDocument();
});

it("tracks the selected day when the period changes and hides days outside it", () => {
  const onSelectDay = vi.fn();
  const initial = series(["1000", "750", "800"]);
  const { rerender } = render(
    <ProjectionTimeline
      series={initial}
      referenceDate="2026-10-23"
      onSelectDay={onSelectDay}
    />,
  );
  fireEvent.change(screen.getByRole("combobox"), { target: { value: "2" } });
  const shorter = {
    ...initial,
    points: initial.points.slice(1),
  };
  rerender(
    <ProjectionTimeline
      series={shorter}
      referenceDate="2026-10-23"
      onSelectDay={onSelectDay}
    />,
  );
  expect(screen.getByRole("combobox")).toHaveValue("1");
  expect(screen.getByText(/Saldo realizado:.*800,00/)).toBeInTheDocument();
  fireEvent.click(
    screen.getByRole("button", { name: "Ver transações do dia" }),
  );
  expect(onSelectDay).toHaveBeenCalledWith("2026-10-23");

  rerender(
    <ProjectionTimeline
      series={series(["900"])}
      referenceDate="2026-10-23"
      onSelectDay={onSelectDay}
    />,
  );
  expect(screen.getByRole("combobox")).toHaveValue("");
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "Ver transações do dia" }),
  ).not.toBeInTheDocument();
});

function futureEntry(overrides: Partial<TimelineEntry>): TimelineEntry {
  return {
    date: "2026-10-23",
    description: "Compra",
    amount: "-100.00",
    reportable_amount: "-100.00",
    category_id: null,
    category_name: "Sem categoria",
    tier: "confirmado",
    source: "real",
    source_ref_id: "ref",
    scenario_id: null,
    ...overrides,
  };
}

describe("ProjectionTimeline markers", () => {
  it("labels the biggest future events and the first negative day, and says so in the legend", () => {
    const base = series(["1000", "750", "-200"]);
    render(
      <ProjectionTimeline
        series={{
          ...base,
          first_negative: "2026-10-23",
          entries: [
            futureEntry({ date: "2026-10-22", amount: "-950.00" }),
            futureEntry({ date: "2026-10-22", amount: "-50.00", description: "Outra" }),
          ],
        }}
        referenceDate="2026-10-21"
      />,
    );

    expect(screen.getByText("Fatura 22/10")).toBeInTheDocument();
    expect(screen.getByText("Negativo 23/10")).toBeInTheDocument();
    const legend = screen.getByRole("list", { name: "Legenda do gráfico" });
    expect(legend).toHaveTextContent("Evento");
    expect(legend).toHaveTextContent("Negativo");
  });

  it("leaves the first negative day to its own marker instead of stacking an event label on it", () => {
    render(
      <ProjectionTimeline
        series={{
          ...series(["1000", "750", "-200"]),
          first_negative: "2026-10-23",
          entries: [futureEntry({ date: "2026-10-23", amount: "-950.00" })],
        }}
        referenceDate="2026-10-21"
      />,
    );

    expect(screen.queryByText("Fatura 23/10")).not.toBeInTheDocument();
  });

  it("draws no event or negative marker when there is nothing to name", () => {
    const { container } = render(<ProjectionTimeline series={series(["1000", "750", "800"])} referenceDate="2026-10-22" />);

    // The lowest-balance dot is not a marker: it carries no label.
    expect(container.querySelectorAll(".ui-line-plot-marker")).toHaveLength(0);
    const legend = screen.getByRole("list", { name: "Legenda do gráfico" });
    expect(legend).not.toHaveTextContent("Evento");
    expect(legend).not.toHaveTextContent("Negativo");
  });

  it("no longer carries the negative-balance warning inside the chart", () => {
    render(<ProjectionTimeline series={{ ...series(["1000", "-5", "800"]), first_negative: "2026-10-22" }} referenceDate="2026-10-22" />);

    expect(screen.queryByText(/pode ficar negativo/)).toBeNull();
  });
});
