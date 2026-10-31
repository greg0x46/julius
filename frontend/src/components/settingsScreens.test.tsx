import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { AutomationRule, Category } from "../api/contracts";
import { SettingsPage } from "../pages/SettingsPage";
import { AutomationEntryForm } from "./automationRules/AutomationEntryForm";
import { CategoryList } from "./categories/CategoryList";
import * as compactScreen from "./shared/useCompactScreen";

afterEach(() => vi.restoreAllMocks());

const category = (overrides: Partial<Category>): Category => ({
  id: "c1",
  name: "Mercado",
  kind: "expense",
  is_active: true,
  icon: "shopping-cart",
  color: "#2a78d6",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  ...overrides,
});

describe("SettingsPage", () => {
  it("groups the screens under Organização, Dados and Conta", () => {
    render(
      <MemoryRouter>
        <SettingsPage />
      </MemoryRouter>,
    );
    const group = (name: string) => within(screen.getByRole("region", { name }));
    expect(group("Organização").getByRole("link", { name: "Abrir Categorias" })).toHaveAttribute(
      "href",
      "/configuracoes/categorias",
    );
    expect(group("Dados").getByRole("link", { name: "Abrir Open Banking" })).toBeInTheDocument();
    expect(group("Conta").getByRole("link", { name: "Abrir Segurança" })).toBeInTheDocument();
  });
});

describe("CategoryList on a phone", () => {
  const categories = [
    category({ id: "a", name: "Mercado" }),
    category({ id: "b", name: "Salário", kind: "income" }),
    category({ id: "c", name: "Academia", is_active: false }),
  ];

  it("groups rows by kind, marks only inactive ones and opens the edit sheet on tap", async () => {
    vi.spyOn(compactScreen, "useCompactScreen").mockReturnValue(true);
    const onRename = vi.fn();
    render(
      <CategoryList
        categories={categories}
        isLoading={false}
        togglingCategoryId={null}
        onRename={onRename}
        onToggle={() => undefined}
      />,
    );

    expect(screen.getByRole("list", { name: "Despesas" })).toBeInTheDocument();
    expect(screen.getByRole("list", { name: "Receitas" })).toBeInTheDocument();
    expect(screen.queryByRole("list", { name: "Transferências" })).not.toBeInTheDocument();
    expect(screen.getAllByText("Inativa")).toHaveLength(1);
    // The table's switch and Editar link are not repeated on the row.
    expect(screen.queryByRole("switch")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Editar" })).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Editar categoria Salário" }));
    expect(onRename).toHaveBeenCalledWith(categories[1]);
  });

  it("says there are no categories yet", () => {
    vi.spyOn(compactScreen, "useCompactScreen").mockReturnValue(true);
    render(
      <CategoryList
        categories={[]}
        isLoading={false}
        togglingCategoryId={null}
        onRename={() => undefined}
        onToggle={() => undefined}
      />,
    );
    expect(screen.getByText("Nenhuma categoria ainda")).toBeInTheDocument();
  });
});

describe("AutomationEntryForm", () => {
  const rule = (conditionCount: number): AutomationRule => ({
    id: "r1",
    name: "Ignorar IOF",
    is_active: true,
    logic_operator: "and",
    conditions: Array.from({ length: conditionCount }, () => ({
      field: "description" as const,
      operator: "contains" as const,
      value: "IOF",
    })),
    actions: [{ type: "ignore", scenario_id: null, category_id: null }],
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  });

  const renderForm = (conditionCount: number) =>
    render(
      <AutomationEntryForm
        open
        entry={{ rule: rule(conditionCount), linkedCommitment: null }}
        conditionOptions={{ accounts: [], cards: [] }}
        conditionOptionsLoading={false}
        categories={[]}
        commitments={[]}
        commitmentsLoading={false}
        submitting={false}
        submitError={null}
        onSubmit={() => undefined}
        onCancel={() => undefined}
        onDelete={() => undefined}
      />,
    );

  it("offers the AND/OR choice only with two or more conditions", () => {
    const { unmount } = renderForm(1);
    expect(screen.queryByText("Combinar condições com")).not.toBeInTheDocument();
    unmount();

    renderForm(2);
    expect(screen.getByText("Combinar condições com")).toBeInTheDocument();
  });

  it("deletes from the edit sheet after confirming", async () => {
    const onDelete = vi.fn();
    render(
      <AutomationEntryForm
        open
        entry={{ rule: rule(1), linkedCommitment: null }}
        conditionOptions={{ accounts: [], cards: [] }}
        conditionOptionsLoading={false}
        categories={[]}
        commitments={[]}
        commitmentsLoading={false}
        submitting={false}
        submitError={null}
        onSubmit={() => undefined}
        onCancel={() => undefined}
        onDelete={onDelete}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Excluir automação" }));
    await userEvent.click(await screen.findByRole("button", { name: "Excluir" }));
    expect(onDelete).toHaveBeenCalledTimes(1);
  });
});

it("translates category controls and accessible icon and color names", async () => {
  const { I18nProvider } = await import("../i18n/I18nProvider");
  const { useTranslation } = await import("../i18n/context");
  const { CategoryForm } = await import("./categories/CategoryForm");
  function LanguageSwitch() {
    const { setLanguage } = useTranslation();
    return <button onClick={() => setLanguage("en")}>English</button>;
  }
  localStorage.removeItem("julius.language");
  render(<I18nProvider><LanguageSwitch /><CategoryForm open category={null} submitting={false} submitError={null} onSubmit={() => undefined} onCancel={() => undefined} /></I18nProvider>);
  await userEvent.click(screen.getByRole("button", { name: "English" }));
  expect(screen.getByRole("dialog", { name: "New category" })).toBeVisible();
  expect(screen.getByRole("radio", { name: "Shopping cart" })).toBeInTheDocument();
  expect(screen.getByRole("radio", { name: "Color Blue" })).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Save" }));
  expect(screen.getByText("Enter a category name.")).toBeVisible();
  localStorage.removeItem("julius.language");
});
