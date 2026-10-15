import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "../api/problems";
import * as dataSourcesApi from "../api/dataSources";
import * as scenariosApi from "../api/scenarios";
import type { Scenario } from "../api/contracts";
import { QueryTestProvider } from "../test/QueryTestProvider";
import { DataSourceConnections } from "./DataSourceConnections";
import { StandaloneScenarioList } from "./scenarios/StandaloneScenarioCard";
import * as compactScreen from "./shared/useCompactScreen";

vi.mock("../api/dataSources");
vi.mock("../api/scenarios");
afterEach(() => vi.clearAllMocks());

const idleSync = {
  state: { kind: "idle" as const },
  submit: vi.fn(),
  reset: vi.fn(),
};

describe("DataSourceConnections: new connection sheet", () => {
  it("opens blank every time and drops the previous error", async () => {
    vi.spyOn(compactScreen, "useCompactScreen").mockReturnValue(false);
    vi.mocked(dataSourcesApi.listDataSources).mockResolvedValue([]);
    vi.mocked(dataSourcesApi.createDataSource).mockRejectedValue(new ApiError("transport", "offline"));
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <QueryTestProvider>
          <DataSourceConnections sync={idleSync} runs={[]} />
        </QueryTestProvider>
      </MemoryRouter>,
    );

    await user.click(await screen.findByRole("button", { name: "Adicionar conexão" }));
    const sheet = within(await screen.findByRole("dialog"));
    await user.click(sheet.getByLabelText("Item ID (Pluggy)"));
    await user.paste("item-123");
    await user.click(sheet.getByRole("button", { name: "Adicionar" }));
    expect(await screen.findByText("Não foi possível adicionar a conexão.")).toBeVisible();

    await user.click(sheet.getByRole("button", { name: "Cancelar" }));
    await waitFor(() => expect(screen.queryByLabelText("Item ID (Pluggy)")).toBeNull());

    await user.click(screen.getByRole("button", { name: "Adicionar conexão" }));
    expect(await screen.findByLabelText("Item ID (Pluggy)")).toHaveValue("");
    expect(screen.queryByText("Não foi possível adicionar a conexão.")).toBeNull();
  });
});

describe("StandaloneScenarioList: adding a hypothetical transaction", () => {
  const scenario: Scenario = {
    id: "9b0f0c1e-7f8e-4a43-8f55-3d3f3f3f3f01",
    kind: "standalone",
    name: "Viagem",
    payable_id: null,
    is_active: true,
    is_accounting_source: false,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };

  it("keeps what was typed and says why when the write fails", async () => {
    vi.spyOn(compactScreen, "useCompactScreen").mockReturnValue(false);
    vi.mocked(scenariosApi.getScenario).mockResolvedValue({ ...scenario, transactions: [] } as never);
    vi.mocked(scenariosApi.listScenarioPlannedTransactions).mockResolvedValue([]);
    const onAdd = vi.fn().mockRejectedValue(new Error("Servidor indisponível."));
    const user = userEvent.setup();
    render(
      <QueryTestProvider>
        <StandaloneScenarioList
          scenarios={[scenario]}
          isLoading={false}
          onDeleteScenario={async () => null}
          onAddTransaction={onAdd}
          onDeleteTransaction={async () => undefined}
          onToggleScenario={async () => null}
          onActionError={() => undefined}
        />
      </QueryTestProvider>,
    );

    // A tap on the row expands it.
    await user.click(screen.getByText("Viagem"));
    const form = within(await screen.findByRole("form", { name: "Nova transação hipotética" }));
    await user.click(form.getByLabelText("Descrição"));
    await user.paste("Hotel");
    await user.type(form.getByLabelText("Valor"), "100");
    await user.click(form.getByRole("button", { name: "Adicionar transação" }));

    expect(await form.findByText("Servidor indisponível.")).toBeVisible();
    expect(form.getByLabelText("Descrição")).toHaveValue("Hotel");
    expect(onAdd).toHaveBeenCalledTimes(1);
  });
});
