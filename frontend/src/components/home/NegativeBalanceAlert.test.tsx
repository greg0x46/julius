import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { TimelineSeries } from "../../api/contracts";
import * as projectionHook from "../../hooks/useHomeProjection";
import { NegativeBalanceAlert } from "./NegativeBalanceAlert";

vi.mock("../../hooks/useHomeProjection");

const period = { from: "2026-10-01", to: "2026-11-30" };

function mockProjection(overrides: Partial<ReturnType<typeof projectionHook.useHomeProjection>>, base?: Partial<TimelineSeries>) {
  const lowest = { date: "2026-11-24", balance: "-900.00", inflow: "0", outflow: "0", lowest_tier: "projetado" as const };
  const series: TimelineSeries = {
    points: [],
    entries: [],
    starting_balance: "1000.00",
    lowest_balance: lowest,
    first_negative: "2026-11-21",
    ...base,
  };
  vi.mocked(projectionHook.useHomeProjection).mockReturnValue({
    timeline: { currencyCode: "BRL" },
    base: series,
    bounds: period,
    wholePeriod: false,
    referenceDate: "2026-10-22",
    isLoading: false,
    error: null,
    retry: vi.fn(),
    ...overrides,
  } as unknown as ReturnType<typeof projectionHook.useHomeProjection>);
}

function renderAlert() {
  return render(
    <MemoryRouter>
      <NegativeBalanceAlert period={period} />
    </MemoryRouter>,
  );
}

describe("NegativeBalanceAlert", () => {
  beforeEach(() => vi.clearAllMocks());

  it("states the first negative day and the lowest point, as the server reports them", () => {
    mockProjection({});
    renderAlert();

    const alert = screen.getByRole("region", { name: "Alerta de projeção" });
    expect(alert).toHaveTextContent("O saldo fica negativo em 21/11 se nada mudar.");
    expect(alert).toHaveTextContent("Ponto mais baixo: -R$ 900,00 em 24/11.");
  });

  it("leaves the lowest point out when it is the last day of the window", () => {
    mockProjection({}, {
      points: [
        { date: "2026-11-21", balance: "-100.00", inflow: "0", outflow: "0", lowest_tier: "projetado" },
        { date: "2026-11-24", balance: "-900.00", inflow: "0", outflow: "0", lowest_tier: "projetado" },
      ],
    });
    renderAlert();

    expect(screen.getByRole("region")).toHaveTextContent("O saldo fica negativo em 21/11 se nada mudar.");
    expect(screen.getByRole("region")).not.toHaveTextContent("Ponto mais baixo");
  });

  it("links to the transactions of the displayed period", () => {
    mockProjection({});
    renderAlert();

    expect(screen.getByRole("link", { name: "Ver transações do período" })).toHaveAttribute(
      "href",
      "/transacoes?date_from=2026-10-01&date_to=2026-11-30",
    );
  });

  it("adds the year when the day is not in the current year", () => {
    mockProjection({}, {
      first_negative: "2027-01-05",
      lowest_balance: { date: "2027-01-07", balance: "-10.50", inflow: "0", outflow: "0", lowest_tier: "projetado" },
    });
    renderAlert();

    expect(screen.getByRole("region")).toHaveTextContent("negativo em 05/01/2027");
    expect(screen.getByRole("region")).toHaveTextContent("-R$ 10,50 em 07/01/2027");
  });

  it("uses the response currency", () => {
    mockProjection({ timeline: { currencyCode: "USD" } } as never);
    renderAlert();

    expect(screen.getByRole("region")).toHaveTextContent("-USD 900,00");
  });

  it("renders nothing when the balance never goes negative", () => {
    mockProjection({}, { first_negative: null });
    renderAlert();

    expect(screen.queryByRole("region")).toBeNull();
  });

  it("renders nothing while loading or when the series failed", () => {
    mockProjection({ isLoading: true, base: null });
    const { unmount } = renderAlert();
    expect(screen.queryByRole("region")).toBeNull();
    unmount();

    mockProjection({ error: new Error("boom") });
    renderAlert();
    expect(screen.queryByRole("region")).toBeNull();
  });
});
