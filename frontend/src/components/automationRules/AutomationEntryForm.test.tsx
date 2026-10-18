import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { AutomationRule, Category, RecurringCommitment } from "../../api/contracts";
import { AutomationEntryForm, type AutomationEntry } from "./AutomationEntryForm";
import { AutomationEntryList } from "./AutomationEntryList";

vi.mock("../../hooks/useAccounts", () => ({ useAccounts: () => ({ accounts: [], isLoading: false }) }));

const category: Category = {
  id: "rent-category", name: "Aluguel", kind: "expense", is_active: true,
  icon: "home", color: "#fff", created_at: "", updated_at: "",
};
const otherCategory = { ...category, id: "other-category", name: "Moradia" };
const commitment: RecurringCommitment = {
  id: "rent", name: "Aluguel mensal", kind: "expense", amount: "1500.00",
  category_id: category.id, account_id: null, cadence: "monthly", day_of_month: 5,
  month_of_year: null, start_date: "2026-01-01", end_date: null,
  is_active: true, created_at: "", updated_at: "",
};
const rule: AutomationRule = {
  id: "rule", name: "Conciliar aluguel", is_active: true, logic_operator: "and",
  conditions: [{ field: "amount", operator: "within_percent", value: "1500:10" }],
  actions: [{ type: "reconcile", scenario_id: commitment.id, category_id: null }],
  created_at: "", updated_at: "",
};

function renderForm(entry: AutomationEntry | null) {
  const onSubmit = vi.fn();
  const view = render(<AutomationEntryForm
    open entry={entry} conditionOptions={{ accounts: [], cards: [] }} conditionOptionsLoading={false}
    categories={[category, otherCategory]} commitments={[commitment]} commitmentsLoading={false}
    submitting={false} submitError={null} onSubmit={onSubmit} onCancel={() => undefined}
  />);
  return { ...view, onSubmit };
}

async function choose(label: string, option: string) {
  await userEvent.click(screen.getByRole("combobox", { name: label }));
  fireEvent.click(await screen.findByText(option, { selector: ".ant-select-item-option-content" }));
}

function clearCategory() {
  const field = screen.getByRole("combobox", { name: "Categoria a aplicar (opcional)" }).closest(".ant-select")!;
  const clear = field.querySelector(".ant-select-clear")!;
  fireEvent.mouseDown(clear);
  fireEvent.click(clear);
}

describe("reconcile category action", () => {
  it("loads and saves both actions, permits changing and removing the category", async () => {
    const combined = { ...rule, actions: [...rule.actions, { type: "set_category" as const, scenario_id: null, category_id: otherCategory.id }] };
    const { onSubmit } = renderForm({ rule: combined, linkedCommitment: commitment });
    expect(screen.getByText("Despesa: Moradia")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));
    expect(onSubmit).toHaveBeenLastCalledWith(expect.objectContaining({ actionTypes: ["reconcile", "set_category"], categoryId: otherCategory.id }));
    await choose("Categoria a aplicar (opcional)", "Despesa: Aluguel");
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));
    expect(onSubmit).toHaveBeenLastCalledWith(expect.objectContaining({ categoryId: category.id }));
    clearCategory();
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));
    expect(onSubmit).toHaveBeenLastCalledWith(expect.objectContaining({ actionTypes: ["reconcile"], categoryId: null }));
  });

  it("preserves an existing reconcile-only rule and allows adding categorization", async () => {
    const { onSubmit } = renderForm({ rule, linkedCommitment: commitment });
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));
    expect(onSubmit).toHaveBeenLastCalledWith(expect.objectContaining({ actionTypes: ["reconcile"], categoryId: null }));
    await choose("Categoria a aplicar (opcional)", "Despesa: Moradia");
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));
    expect(onSubmit).toHaveBeenLastCalledWith(expect.objectContaining({ actionTypes: ["reconcile", "set_category"], categoryId: otherCategory.id }));
  });

  it("defaults a new rule from the selected commitment, but respects a cleared category", async () => {
    const { onSubmit } = renderForm(null);
    fireEvent.change(screen.getByRole("textbox", { name: "Nome da automação" }), { target: { value: "Aluguel" } });
    await userEvent.click(screen.getByRole("checkbox", { name: "Ignorar transação" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "Conciliar recorrência" }));
    await userEvent.click(screen.getByText("Recorrência existente"));
    await choose("Selecione uma recorrência", commitment.name);
    expect(screen.getByText("Despesa: Aluguel")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));
    expect(onSubmit).toHaveBeenLastCalledWith(expect.objectContaining({ actionTypes: ["reconcile", "set_category"], categoryId: category.id }));
    clearCategory();
    await choose("Categoria", otherCategory.name);
    expect(screen.getByRole("combobox", { name: "Categoria a aplicar (opcional)" })).toHaveValue("");
    expect(screen.getByText("Sem categoria a aplicar")).toBeInTheDocument();
  }, 30_000);

  it("defaults from a new commitment's category", async () => {
    renderForm(null);
    await userEvent.click(screen.getByRole("checkbox", { name: "Conciliar recorrência" }));
    await choose("Categoria", category.name);
    expect(screen.getByText("Despesa: Aluguel")).toBeInTheDocument();
  });

  it("never sends a fractional day of the month for the commitment", async () => {
    const { onSubmit } = renderForm({ rule, linkedCommitment: commitment });
    const input = screen.getByRole("spinbutton", { name: "Dia do mês" });
    fireEvent.change(input, { target: { value: "6.5" } });
    fireEvent.blur(input);
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));
    expect(onSubmit).toHaveBeenCalledOnce();
    expect(Number.isInteger(onSubmit.mock.calls[0]![0].reconcile.commitment.day_of_month)).toBe(true);
  });

  it("lists both actions on the same automation", () => {
    render(<AutomationEntryList
      rules={[{ ...rule, actions: [...rule.actions, { type: "set_category", scenario_id: null, category_id: category.id }] }]}
      categories={[category]} commitments={[commitment]} isLoading={false} togglingRuleId={null}
      onEditRule={() => undefined} onToggleRule={() => undefined} onDeleteRule={() => undefined}
    />);
    const list = screen.getByRole("table", { name: "Automações" });
    expect(within(list).getByText("Concilia: Aluguel mensal")).toBeInTheDocument();
    expect(within(list).getByText("Categoriza: Aluguel")).toBeInTheDocument();
  });
});
