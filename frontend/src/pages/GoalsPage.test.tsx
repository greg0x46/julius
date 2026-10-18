import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Goal } from "../api/contracts";
import * as goalsApi from "../api/goals";
import { ApiError } from "../api/problems";
import { QueryTestProvider } from "../test/QueryTestProvider";
import { GoalsPage } from "./GoalsPage";

vi.mock("../api/goals");

afterEach(() => vi.restoreAllMocks());

function goal(overrides: Partial<Goal> = {}): Goal {
  return {
    id: "11111111-1111-4111-8111-111111111111",
    name: "Cem mil investidos",
    metric: "invested_value",
    target_amount: "100000.00",
    target_date: null,
    notes: null,
    current_value: "45210.50",
    progress: "0.452105",
    currency_code: "BRL",
    created_at: "2026-10-01T12:00:00Z",
    updated_at: "2026-10-01T12:00:00Z",
    ...overrides,
  };
}

function renderPage() {
  return render(
    <MemoryRouter>
      <QueryTestProvider>
        <GoalsPage />
      </QueryTestProvider>
    </MemoryRouter>,
  );
}

describe("GoalsPage", () => {
  it("shows a loading state while the goals are on their way", () => {
    vi.mocked(goalsApi.listGoals).mockReturnValue(new Promise(() => undefined));
    renderPage();
    expect(screen.getByRole("status", { name: "Carregando metas" })).toBeVisible();
  });

  it("says there are no goals yet instead of drawing an empty list", async () => {
    vi.mocked(goalsApi.listGoals).mockResolvedValue([]);
    renderPage();
    expect(await screen.findByText("Nenhuma meta ainda")).toBeVisible();
  });

  it("says only that the load failed, never 'nothing yet', when the goals cannot be loaded", async () => {
    vi.mocked(goalsApi.listGoals).mockRejectedValue(new ApiError("response", "falhou"));
    renderPage();
    expect(await screen.findByText("Não foi possível carregar as metas")).toBeVisible();
    expect(screen.queryByText("Nenhuma meta ainda")).toBeNull();
  });

  it("shows the value, the target and the real progress percent", async () => {
    vi.mocked(goalsApi.listGoals).mockResolvedValue([goal()]);
    renderPage();
    const section = await screen.findByRole("region", { name: "Cem mil investidos" });
    expect(within(section).getByText("R$ 45.210,50")).toBeVisible();
    expect(within(section).getByText("R$ 100.000,00")).toBeVisible();
    expect(within(section).getByText("45%")).toBeVisible();
    expect(within(section).getByRole("progressbar")).toHaveAttribute("aria-valuenow", "45");
  });

  it.each([
    ["995", "0.995"],
    ["999.9999999999999", "0.9999999999999999"],
  ])("keeps an unreached goal incomplete at value %s", async (current_value, progress) => {
    vi.mocked(goalsApi.listGoals).mockResolvedValue([
      goal({ target_amount: "1000", current_value, progress }),
    ]);
    renderPage();
    const section = await screen.findByRole("region", { name: "Cem mil investidos" });
    expect(within(section).getByText("99%")).toBeVisible();
    expect(within(section).getByRole("progressbar")).toHaveAttribute("aria-valuenow", "99");
    expect(within(section).queryByText("Meta atingida")).toBeNull();
    expect(section.querySelector(".ant-progress-status-success")).toBeNull();
  });

  it.each([
    ["100000.00", "1", "100%"],
    ["123450.00", "1.2345", "123%"],
  ])("marks a goal reached at value %s and caps its bar", async (current_value, progress, percent) => {
    vi.mocked(goalsApi.listGoals).mockResolvedValue([
      goal({ current_value, progress }),
    ]);
    renderPage();
    const section = await screen.findByRole("region", { name: "Cem mil investidos" });
    expect(within(section).getByText(percent)).toBeVisible();
    expect(within(section).getByRole("progressbar")).toHaveAttribute("aria-valuenow", "100");
    expect(within(section).getByText("Meta atingida")).toBeVisible();
    expect(section.querySelector(".ant-progress-status-success")).not.toBeNull();
  });

  it("creates a goal from the form and refreshes the list", async () => {
    vi.mocked(goalsApi.listGoals).mockResolvedValue([]);
    vi.mocked(goalsApi.createGoal).mockResolvedValue(goal());
    renderPage();
    await screen.findByText("Nenhuma meta ainda");

    await userEvent.click(screen.getByRole("button", { name: "Nova meta" }));
    await userEvent.type(await screen.findByLabelText("Nome da meta"), "Cem mil investidos");
    await userEvent.type(screen.getByLabelText("Valor da meta"), "100000");
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() =>
      expect(goalsApi.createGoal).toHaveBeenCalledWith({
        name: "Cem mil investidos",
        metric: "invested_value",
        target_amount: "100000.00",
        target_date: null,
        notes: null,
      }),
    );
    await waitFor(() => expect(goalsApi.listGoals).toHaveBeenCalledTimes(2));
  });

  it("keeps the form open with the message when the target is missing", async () => {
    vi.mocked(goalsApi.listGoals).mockResolvedValue([]);
    renderPage();
    await screen.findByText("Nenhuma meta ainda");

    await userEvent.click(screen.getByRole("button", { name: "Nova meta" }));
    await userEvent.type(await screen.findByLabelText("Nome da meta"), "Sem valor");
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));

    expect(await screen.findByText("O valor da meta precisa ser maior que zero.")).toBeVisible();
    expect(goalsApi.createGoal).not.toHaveBeenCalled();
  });

  it("deletes a goal after confirming", async () => {
    vi.mocked(goalsApi.listGoals).mockResolvedValue([goal()]);
    vi.mocked(goalsApi.deleteGoal).mockResolvedValue(undefined);
    vi.spyOn(window, "confirm").mockReturnValue(true);
    renderPage();
    await screen.findByRole("region", { name: "Cem mil investidos" });

    await userEvent.click(screen.getByRole("button", { name: "Ações de Cem mil investidos" }));
    await userEvent.click(await screen.findByText("Excluir meta"));

    await waitFor(() => expect(goalsApi.deleteGoal).toHaveBeenCalledWith(goal().id));
  });
});
