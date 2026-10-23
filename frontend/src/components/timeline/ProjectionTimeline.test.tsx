import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import type { TimelineSeries } from "../../api/contracts";
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
  fireEvent.click(
    screen.getByRole("button", { name: "Ver transações do dia" }),
  );
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
