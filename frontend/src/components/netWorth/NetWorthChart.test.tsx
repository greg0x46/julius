import { fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";

import type { NetWorthSnapshot } from "../../api/contracts";
import { investmentPeriodFixture, periodSynced } from "../../test/investmentWorkspaceFixtures";
import { NetWorthChart, WealthTooltip } from "./NetWorthChart";

vi.mock("recharts", () => ({
  ResponsiveContainer: ({ children }: { children: ReactNode }) => children,
  LineChart: ({ data, children }: { data: unknown; children: ReactNode }) => (
    <div>
      <pre data-testid="points">{JSON.stringify(data)}</pre>
      {children}
    </div>
  ),
  Line: ({ connectNulls }: { connectNulls: boolean }) => <span data-testid="line">{String(connectNulls)}</span>,
  BarChart: ({ data, children }: { data: unknown; children: ReactNode }) => <div><pre data-testid="flow-points">{JSON.stringify(data)}</pre>{children}</div>,
  Bar: () => null,
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
  // Incomplete historical coverage cannot be compared with observed full valuations.
  expect(points.map((point: { netWorth: number | null }) => point.netWorth)).toEqual([2500, null, null, 2600]);
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
  const points = JSON.parse(screen.getByTestId("points").textContent!);
  expect(points[0].date).toBe("2026-08-18");
  expect(screen.getByRole("img")).toHaveAttribute("aria-label", expect.stringContaining("de 18/08/2026 a 18/08/2026"));
});

const observed: NetWorthSnapshot = {
  captured_at: "2026-01-01", is_backfilled: false, valuation_version: 1, valuation_status: "complete",
  net_worth: "1000000", cash_balance: "10", investment_balance: "999990", total_assets: "1000000",
  total_liabilities: "0", credit_card_balance: "0", payables_debt: "0",
};

it("keeps selection, lines, scale and accessible values synchronized", () => {
  render(<NetWorthChart snapshots={[observed, { ...observed, captured_at: "2026-01-02", cash_balance: "-5" }]} />);
  fireEvent.click(screen.getByRole("checkbox", { name: "Saldo observado" }));
  expect(screen.getAllByTestId("line")).toHaveLength(2);
  fireEvent.click(screen.getByRole("radio", { name: "Variação %" }));
  const points = JSON.parse(screen.getByTestId("points").textContent!);
  expect(points[1]).toMatchObject({ netWorth: 0, cash: -150 });
  expect(screen.getByText(/Variação desde 01\/01\/2026/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("checkbox", { name: "Patrimônio líquido" }));
  expect(screen.getAllByTestId("line")).toHaveLength(1);
  fireEvent.click(screen.getByRole("checkbox", { name: "Saldo observado" }));
  expect(screen.queryByTestId("points")).not.toBeInTheDocument();
  expect(screen.getByRole("status")).toHaveTextContent("Selecione ao menos uma métrica");
});

it("explains invalid percent baselines instead of plotting fabricated starts", () => {
  render(<NetWorthChart snapshots={[{ ...observed, net_worth: "0" }, { ...observed, captured_at: "2026-01-02" }]} />);
  fireEvent.click(screen.getByRole("radio", { name: "Variação %" }));
  expect(screen.getByRole("status")).toHaveTextContent("valor inicial positivo");
  expect(screen.queryByTestId("points")).not.toBeInTheDocument();
});

it("shows only selected tooltip values, with actual money in percentage mode", () => {
  render(<WealthTooltip active label="2026-01-02" selected={["cash"]} scale="percent" points={[
    { date: "2026-01-01", values: { cash: "10", netWorth: "1000000" } },
    { date: "2026-01-02", values: { cash: "-5", netWorth: "1000000" }, notes: ["Câmbio desatualizado", "Investimentos: histórico insuficiente ou parcial"] },
  ]} />);
  expect(screen.getByText(/Saldo observado:/)).toHaveTextContent("-150% · -R$ 5,00");
  expect(screen.queryByText(/Patrimônio líquido:/)).not.toBeInTheDocument();
  expect(screen.getByText("Câmbio desatualizado")).toBeInTheDocument();
  expect(screen.queryByText("Investimentos: histórico insuficiente ou parcial")).not.toBeInTheDocument();
});

it("keeps yield as a separate period flow when stocks use percentages, with incomplete periods as gaps", () => {
  const good = structuredClone(investmentPeriodFixture({ from: "2026-01-01", to: "2026-01-31" }));
  good.totals.partial = false;
  good.totals.positions_partial_start = 0;
  good.groups = [{ ...good.groups[0], positions: [{ ...periodSynced, partial_start: false, history_complete: true }] }];
  const partial = { ...good, from: "2026-02-01", to: "2026-02-28", totals: { ...good.totals, partial: true } };
  render(<NetWorthChart snapshots={[observed]} from="2026-01-01" to="2026-02-28" history={{ baseline: { ...good, to: "2026-01-01" }, periods: [good, partial] }} />);
  fireEvent.click(screen.getByRole("checkbox", { name: "Rendimentos de investimentos" }));
  fireEvent.click(screen.getByRole("radio", { name: "Variação %" }));
  const flows = JSON.parse(screen.getByTestId("flow-points").textContent!);
  expect(flows.map((point: { yield: number | null }) => point.yield)).toEqual([Number(good.totals.yield), null]);
  expect(screen.getByRole("heading", { name: "Rendimentos por mês · BRL" })).toBeInTheDocument();
  expect(screen.getByText(/A variação cambial dentro do período não está incluída/)).toBeInTheDocument();
  expect(screen.getByText("Ver rendimentos e retorno Modified Dietz")).toBeInTheDocument();
});

it("does not compare cached portfolio values in another reporting currency", () => {
  const period = investmentPeriodFixture({ currency_code: "USD" });
  render(<NetWorthChart snapshots={[observed]} history={{ baseline: period, periods: [period] }} />);
  expect(screen.getByRole("checkbox", { name: "Investimentos" })).toBeDisabled();
  expect(screen.getAllByText(/Moeda diferente do relatório/)).toHaveLength(2);
});

it.each([
  ["loading", "Carregando histórico"],
  ["error", "Histórico indisponível"],
  [undefined, "Sem histórico suficiente neste período"],
] as const)("allows clearing selected investment metrics when history becomes %s", (historyState, explanation) => {
  const period = structuredClone(investmentPeriodFixture({ from: "2026-01-01", to: "2026-01-31" }));
  period.totals.partial = false;
  period.totals.positions_partial_start = 0;
  period.groups = [{ ...period.groups[0], positions: [{ ...periodSynced, partial_start: false, history_complete: true }] }];
  const { rerender } = render(<NetWorthChart snapshots={[observed]} history={{ baseline: period, periods: [period] }} />);
  fireEvent.click(screen.getByRole("checkbox", { name: "Investimentos" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "Rendimentos de investimentos" }));

  rerender(<NetWorthChart snapshots={[observed]} historyState={historyState} />);
  for (const name of ["Investimentos", "Rendimentos de investimentos"]) {
    const metric = screen.getByRole("checkbox", { name });
    expect(metric).toBeChecked();
    expect(metric).toBeEnabled();
    expect(metric).toHaveAccessibleDescription(`— ${explanation}`);
    fireEvent.click(metric);
    expect(metric).not.toBeChecked();
    expect(metric).toBeDisabled();
  }
  fireEvent.click(screen.getByRole("checkbox", { name: "Patrimônio líquido" }));
  expect(screen.getByText("Selecione ao menos uma métrica para ver a evolução.")).toBeInTheDocument();
});
