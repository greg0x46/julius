import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import type { NetWorthSnapshot } from "../../api/contracts";
import { NetWorthChart } from "./NetWorthChart";

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
        {
          captured_at: "2020-01-02",
          is_backfilled: true,
          valuation_version: 1,
          valuation_status: "unavailable",
        },
        {
          ...valued,
          captured_at: "2020-01-03",
          valuation_version: 0,
          valuation_status: "legacy",
          net_worth: "2100",
        },
        { ...valued, captured_at: "2020-01-04", net_worth: "2600" },
      ]}
    />,
  );
  const line = document.querySelector('[data-series="netWorth"]')!;
  expect(line.getAttribute("d")?.match(/M/g)).toHaveLength(2);
  fireEvent.change(screen.getByRole("combobox"), { target: { value: "1" } });
  expect(screen.queryByText(/Patrimônio líquido:/)).not.toBeInTheDocument();
  fireEvent.change(screen.getByRole("combobox"), { target: { value: "2" } });
  expect(screen.getByText(/Patrimônio líquido:.*2.100,00/)).toBeInTheDocument();
  expect(screen.getByRole("img")).toHaveAccessibleName(/lacunas/);
});

const originalTZ = process.env.TZ;
afterEach(() => {
  if (originalTZ === undefined) delete process.env.TZ;
  else process.env.TZ = originalTZ;
});

it("labels a midnight-UTC calendar day as that day in a western timezone", () => {
  process.env.TZ = "America/Sao_Paulo";
  const snapshot: NetWorthSnapshot = {
    captured_at: "2026-08-18T00:00:00Z",
    is_backfilled: false,
    valuation_version: 1,
    valuation_status: "complete",
    net_worth: "100",
    cash_balance: "100",
    investment_balance: "0",
    credit_card_balance: "0",
    payables_debt: "0",
    total_assets: "100",
    total_liabilities: "0",
  };
  render(<NetWorthChart snapshots={[snapshot]} />);
  expect(
    screen.getByRole("option", { name: "18/08/2026" }),
  ).toBeInTheDocument();
  expect(screen.getByRole("img")).toHaveAttribute(
    "aria-label",
    expect.stringContaining("de 18/08 a 18/08"),
  );
});
