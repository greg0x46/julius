import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import * as scenariosApi from "../api/scenarios";
import type { Scenario, ScenarioDetail } from "../api/contracts";
import { usePayablePlan } from "./usePayablePlan";

vi.mock("../api/scenarios");

const payableId = "44444444-4444-4444-8444-444444444444";
const scenarioId = "55555555-5555-4555-8555-555555555555";

const scenarioSummary: Scenario = {
  id: scenarioId,
  kind: "debt_plan",
  name: "Plano de pagamento",
  payable_id: payableId,
  is_accounting_source: true,
  created_at: "2026-07-30T12:00:00Z",
  updated_at: "2026-07-30T12:00:00Z",
};

const scenarioDetail: ScenarioDetail = {
  ...scenarioSummary,
  transactions: [
    {
      id: "66666666-6666-4666-8666-666666666666",
      scenario_id: scenarioId,
      description: "Parcela 1/3",
      amount: "333.33",
      projected_at: "2026-09-01",
      category: null,
      status: "projetada",
      realizations: [],
    },
  ],
  accumulated_deviation: "0.00",
};

function setup() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return { client, wrapper };
}

describe("usePayablePlan", () => {
  it("reports no plan when the payable has no scenarios yet", async () => {
    vi.mocked(scenariosApi.listPayableScenarios).mockResolvedValue([]);
    const { wrapper } = setup();
    const { result } = renderHook(() => usePayablePlan(payableId), { wrapper });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.plan).toBeNull();
  });

  it("loads the accounting scenario's detail once the list resolves", async () => {
    vi.mocked(scenariosApi.listPayableScenarios).mockResolvedValue([scenarioSummary]);
    vi.mocked(scenariosApi.getScenario).mockResolvedValue(scenarioDetail);
    const { wrapper } = setup();
    const { result } = renderHook(() => usePayablePlan(payableId), { wrapper });

    await waitFor(() => expect(result.current.plan).not.toBeNull());
    expect(scenariosApi.getScenario).toHaveBeenCalledWith(scenarioId, expect.anything());
    expect(result.current.plan?.transactions).toHaveLength(1);
  });

  it("selects the accounting scenario independently of array order", async () => {
    const simulation: Scenario = {
      ...scenarioSummary,
      id: "99999999-9999-4999-8999-999999999999",
      name: "Simulação",
      is_accounting_source: false,
    };
    vi.mocked(scenariosApi.listPayableScenarios).mockResolvedValue([simulation, scenarioSummary]);
    vi.mocked(scenariosApi.getScenario).mockResolvedValue(scenarioDetail);
    const { wrapper } = setup();
    const { result } = renderHook(() => usePayablePlan(payableId), { wrapper });

    await waitFor(() => expect(result.current.plan).not.toBeNull());
    expect(scenariosApi.getScenario).toHaveBeenCalledWith(scenarioId, expect.anything());
    expect(scenariosApi.getScenario).not.toHaveBeenCalledWith(simulation.id, expect.anything());
  });

  it("invalidates the scenarios query after generating installments", async () => {
    vi.mocked(scenariosApi.listPayableScenarios).mockResolvedValue([scenarioSummary]);
    vi.mocked(scenariosApi.getScenario).mockResolvedValue(scenarioDetail);
    vi.mocked(scenariosApi.generateInstallments).mockResolvedValue(scenarioDetail.transactions);
    const { client, wrapper } = setup();
    const invalidateSpy = vi.spyOn(client, "invalidateQueries");
    const { result } = renderHook(() => usePayablePlan(payableId), { wrapper });
    await waitFor(() => expect(result.current.plan).not.toBeNull());

    await act(() => result.current.generateInstallments({ cadence: "mensal", months: 3 }));

    expect(scenariosApi.generateInstallments).toHaveBeenCalledWith(scenarioId, {
      cadence: "mensal",
      months: 3,
    });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["payables", payableId, "scenarios"] });
  });

  it("creates a plan and invalidates the scenarios query", async () => {
    vi.mocked(scenariosApi.listPayableScenarios).mockResolvedValue([]);
    vi.mocked(scenariosApi.createPayableScenario).mockResolvedValue({
      ...scenarioSummary,
      transactions: [],
      accumulated_deviation: "0.00",
    });
    const { client, wrapper } = setup();
    const invalidateSpy = vi.spyOn(client, "invalidateQueries");
    const { result } = renderHook(() => usePayablePlan(payableId), { wrapper });
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    await act(() => result.current.createPlan("Plano de pagamento"));

    expect(scenariosApi.createPayableScenario).toHaveBeenCalledWith(payableId, {
      name: "Plano de pagamento",
    });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["payables", payableId, "scenarios"] });
  });

  it("allocates a payable link to an installment and invalidates the scenarios query", async () => {
    const transactionId = scenarioDetail.transactions[0].id;
    vi.mocked(scenariosApi.listPayableScenarios).mockResolvedValue([scenarioSummary]);
    vi.mocked(scenariosApi.getScenario).mockResolvedValue(scenarioDetail);
    vi.mocked(scenariosApi.createRealization).mockResolvedValue({
      ...scenarioDetail.transactions[0],
      status: "paga",
    });
    const { client, wrapper } = setup();
    const invalidateSpy = vi.spyOn(client, "invalidateQueries");
    const { result } = renderHook(() => usePayablePlan(payableId), { wrapper });
    await waitFor(() => expect(result.current.plan).not.toBeNull());

    const linkId = "77777777-7777-4777-8777-777777777777";
    await act(() =>
      result.current.allocateRealization({
        transactionId,
        write: { payable_link_id: linkId, allocated_amount: "333.33" },
      }),
    );

    expect(scenariosApi.createRealization).toHaveBeenCalledWith(scenarioId, transactionId, {
      payable_link_id: linkId,
      allocated_amount: "333.33",
    });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["payables", payableId, "scenarios"] });
  });

  it("deallocates a realization from an installment and invalidates the scenarios query", async () => {
    const transactionId = scenarioDetail.transactions[0].id;
    vi.mocked(scenariosApi.listPayableScenarios).mockResolvedValue([scenarioSummary]);
    vi.mocked(scenariosApi.getScenario).mockResolvedValue(scenarioDetail);
    vi.mocked(scenariosApi.deleteRealization).mockResolvedValue(undefined);
    const { client, wrapper } = setup();
    const invalidateSpy = vi.spyOn(client, "invalidateQueries");
    const { result } = renderHook(() => usePayablePlan(payableId), { wrapper });
    await waitFor(() => expect(result.current.plan).not.toBeNull());

    const realizationId = "88888888-8888-4888-8888-888888888888";
    await act(() => result.current.deallocateRealization({ transactionId, realizationId }));

    expect(scenariosApi.deleteRealization).toHaveBeenCalledWith(scenarioId, transactionId, realizationId);
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["payables", payableId, "scenarios"] });
  });

  it("readjusts installments and invalidates the scenarios query", async () => {
    vi.mocked(scenariosApi.listPayableScenarios).mockResolvedValue([scenarioSummary]);
    vi.mocked(scenariosApi.getScenario).mockResolvedValue(scenarioDetail);
    vi.mocked(scenariosApi.readjustInstallments).mockResolvedValue(scenarioDetail.transactions);
    const { client, wrapper } = setup();
    const invalidateSpy = vi.spyOn(client, "invalidateQueries");
    const { result } = renderHook(() => usePayablePlan(payableId), { wrapper });
    await waitFor(() => expect(result.current.plan).not.toBeNull());

    await act(() => result.current.readjust({ strategy: "redistribuir" }));

    expect(scenariosApi.readjustInstallments).toHaveBeenCalledWith(scenarioId, {
      strategy: "redistribuir",
    });
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["payables", payableId, "scenarios"] });
  });
});
