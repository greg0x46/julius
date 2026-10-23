import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import type { NetWorthPoint } from "../../api/contracts";
import { NetWorthChart } from "./NetWorthChart";

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
        {
          date: "2020-01-02",
          valuation_status: "unavailable",
        },
        {
          ...valued,
          date: "2020-01-03",
          valuation_status: "stale",
          total: "2100",
        },
        { ...valued, date: "2020-01-04", total: "2600" },
      ]}
    />,
  );
  const line = document.querySelector('[data-series="netWorth"]')!;
  expect(line.getAttribute("d")?.match(/M/g)).toHaveLength(2);
  fireEvent.change(screen.getByRole("combobox"), { target: { value: "1" } });
  expect(screen.queryByText(/Patrimônio:/)).not.toBeInTheDocument();
  fireEvent.change(screen.getByRole("combobox"), { target: { value: "2" } });
  expect(screen.getByText(/Patrimônio:.*2.100,00/)).toBeInTheDocument();
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
  expect(
    screen.getByRole("option", { name: "18/08/2026" }),
  ).toBeInTheDocument();
  expect(screen.getByRole("img")).toHaveAttribute(
    "aria-label",
    expect.stringContaining("de 18/08 a 18/08"),
  );
});
