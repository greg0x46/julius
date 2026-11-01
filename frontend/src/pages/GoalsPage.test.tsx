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
    projection: null,
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

  it("projects a goal behind its target to the date and states the monthly contribution", async () => {
    vi.mocked(goalsApi.listGoals).mockResolvedValue([
      goal({
        target_date: "2027-12-31",
        projection: {
          status: "behind",
          months_remaining: 14,
          monthly_return: "0.008",
          return_from: "2026-01-01",
          return_partial: false,
          projected_value: "52000.00",
          projected_progress: "0.52",
          required_monthly_contribution: "3100.50",
        },
      }),
    ]);
    renderPage();
    const projection = within(await screen.findByRole("region", { name: "Cem mil investidos" })).getByLabelText(
      "Projeção de Cem mil investidos",
    );
    expect(screen.getByText("31/12/2027 · faltam 14 meses")).toBeVisible();
    expect(within(projection).getByText("+0,80% ao mês")).toBeVisible();
    expect(within(projection).getByText(/52,00% da meta/)).toBeVisible();
    expect(within(projection).getByText("R$ 3.100,50")).toBeVisible();
  });

  it("needs no contribution when appreciation alone reaches the target, and shows a loss as negative", async () => {
    vi.mocked(goalsApi.listGoals).mockResolvedValue([
      goal({
        target_date: "2027-12-31",
        projection: {
          status: "on_track",
          months_remaining: 1,
          monthly_return: "-0.0125",
          return_from: "2026-01-01",
          return_partial: true,
          projected_value: "100500.00",
          projected_progress: "1.005",
          required_monthly_contribution: "0",
        },
      }),
    ]);
    renderPage();
    const projection = await screen.findByLabelText("Projeção de Cem mil investidos");
    expect(screen.getByText("31/12/2027 · faltam 1 mês")).toBeVisible();
    expect(within(projection).getByText("-1,25% ao mês")).toBeVisible();
    expect(within(projection).getByText("Nenhum — a valorização basta")).toBeVisible();
    expect(within(projection).getByRole("button", { name: /Sobre a projeção de Cem mil investidos.*parte das posições ficou de fora/ })).toBeInTheDocument();
  });

  it("says so when there is too little history to measure the appreciation", async () => {
    vi.mocked(goalsApi.listGoals).mockResolvedValue([
      goal({
        target_date: "2027-12-31",
        projection: {
          status: "behind",
          months_remaining: 12,
          monthly_return: null,
          return_from: null,
          return_partial: false,
          projected_value: "45210.50",
          projected_progress: "0.4521050",
          required_monthly_contribution: "4565.79",
        },
      }),
    ]);
    renderPage();
    const projection = await screen.findByLabelText("Projeção de Cem mil investidos");
    expect(within(projection).getByText("Sem histórico suficiente")).toBeVisible();
    expect(within(projection).getByRole("button", { name: /Sobre a projeção de Cem mil investidos.*considera valorização zero/ })).toBeInTheDocument();
  });

  it("says when a goal has no target date and offers a tile to add another", async () => {
    vi.mocked(goalsApi.listGoals).mockResolvedValue([goal()]);
    renderPage();
    await screen.findByRole("region", { name: "Cem mil investidos" });
    expect(screen.getByText("Sem data-alvo definida")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Adicionar nova meta" }));
    expect(await screen.findByLabelText("Nome da meta")).toBeVisible();
  });

  it("shows no projection for a goal without one", async () => {
    vi.mocked(goalsApi.listGoals).mockResolvedValue([goal()]);
    renderPage();
    await screen.findByRole("region", { name: "Cem mil investidos" });
    expect(screen.queryByLabelText(/^Projeção de/)).toBeNull();
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
    expect(section.querySelector(".goal-ring-success")).toBeNull();
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
    expect(section.querySelector(".goal-ring-success")).not.toBeNull();
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

it("updates goal controls and validation when switched to English", async () => {
  const { I18nProvider } = await import("../i18n/I18nProvider");
  const { useTranslation } = await import("../i18n/context");
  function LanguageSwitch() {
    const { setLanguage } = useTranslation();
    return <button onClick={() => setLanguage("en")}>English</button>;
  }
  vi.mocked(goalsApi.listGoals).mockResolvedValue([goal()]);
  localStorage.removeItem("julius.language");
  render(<I18nProvider><MemoryRouter><QueryTestProvider><LanguageSwitch /><GoalsPage /></QueryTestProvider></MemoryRouter></I18nProvider>);
  await screen.findByRole("region", { name: "Cem mil investidos" });
  await userEvent.click(screen.getByRole("button", { name: "English" }));
  expect(screen.getByRole("heading", { name: "Goals" })).toBeVisible();
  expect(screen.getByRole("progressbar", { name: "Progress for Cem mil investidos" })).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "New goal" }));
  await userEvent.click(screen.getByRole("button", { name: "Save" }));
  expect(screen.getByText("Enter a goal name.")).toBeVisible();
  localStorage.removeItem("julius.language");
});
