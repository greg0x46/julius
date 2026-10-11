import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { expect, it, vi } from "vitest";
import type { NetWorthSnapshot } from "../../api/contracts";
import { NetWorthChart } from "./NetWorthChart";

vi.mock("recharts", () => ({
  ResponsiveContainer: ({ children }: { children: ReactNode }) => children,
  LineChart: ({ data, children }: { data: unknown; children: ReactNode }) => <div><pre data-testid="points">{JSON.stringify(data)}</pre>{children}</div>,
  Line: ({ connectNulls }: { connectNulls: boolean }) => <span data-testid="line">{String(connectNulls)}</span>,
  CartesianGrid: () => null, Tooltip: () => null, XAxis: () => null, YAxis: () => null,
}));

it("leaves unavailable and unqualified legacy days as gaps", () => {
  const valuation = { reference_currency: "BRL" as const, valued_at: "2020-01-01T12:00:00Z", rate_policy: "latest_observed_at_or_before_v1", rates: {} };
  const valued: NetWorthSnapshot = { captured_at: "2020-01-01", is_backfilled: false, valuation, valuation_status: "complete", net_worth: "2500", cash_balance: "2500", investment_balance: "0", credit_card_balance: "0", payables_debt: "0", total_assets: "2500", total_liabilities: "0" };
  render(<NetWorthChart snapshots={[
    valued,
    { captured_at: "2020-01-02", is_backfilled: true, valuation, valuation_status: "unavailable" },
    { ...valued, captured_at: "2020-01-03", valuation: null, valuation_status: "legacy_unknown", net_worth: "2100" },
    { ...valued, captured_at: "2020-01-04", net_worth: "2600" },
  ]} />);
  const points = JSON.parse(screen.getByTestId("points").textContent!);
  expect(points.map((point: { netWorth: number | null }) => point.netWorth)).toEqual([2500, null, null, 2600]);
  expect(screen.getByTestId("line")).toHaveTextContent("false");
  expect(screen.getByRole("img")).toHaveAccessibleName(/em reais.*lacunas/);
});
