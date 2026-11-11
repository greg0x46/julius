import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { InvestmentTransaction } from "../../api/contracts";
import { InvestmentMovementForm } from "./InvestmentMovementForm";

const investmentId = "11111111-1111-4111-8111-111111111111";

const manualPurchase: InvestmentTransaction = {
  id: "22222222-2222-4222-8222-222222222222",
  external_id: "22222222-2222-4222-8222-222222222222",
  origin: "manual",
  movement_type: "BUY",
  direction: "inflow",
  quantity: "7",
  value: "38",
  amount: "266",
  occurred_at: null,
  trade_date: "2026-01-05",
  notes: "corretora antiga",
};

function renderForm(movement: InvestmentTransaction | null = null, submitError: string | null = null) {
  const onSubmit = vi.fn();
  const onDelete = vi.fn();
  render(
    <InvestmentMovementForm
      open
      investmentId={investmentId}
      movement={movement}
      submitting={false}
      submitError={submitError}
      onSubmit={onSubmit}
      onDelete={onDelete}
      onCancel={vi.fn()}
    />,
  );
  return { onSubmit, onDelete };
}

const save = () => userEvent.click(screen.getByRole("button", { name: "Salvar" }));
const type = (label: string, value: string) =>
  fireEvent.change(screen.getByRole("textbox", { name: label }), { target: { value } });

describe("recording a movement", () => {
  it("sends a purchase priced by quantity and unit price, leaving the total for the server", async () => {
    const { onSubmit } = renderForm();
    type("Quantidade", "7");
    type("Preço unitário", "38");
    await save();
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        investment_id: investmentId,
        kind: "BUY",
        quantity: "7",
        unit_price: "38",
        amount: null,
        notes: null,
      }),
    );
  });

  it("sends the typed total, costs included", async () => {
    const { onSubmit } = renderForm();
    type("Quantidade", "7");
    type("Valor total (com taxas)", "270,50");
    await save();
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ quantity: "7", amount: "270.50" }));
  });

  it("asks for a quantity and a value before sending", async () => {
    const { onSubmit } = renderForm();
    await save();
    expect(screen.getByText("Informe uma quantidade maior que zero.")).toBeInTheDocument();
    type("Quantidade", "7");
    await save();
    expect(screen.getByText("Informe um valor maior que zero.")).toBeInTheDocument();
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("shows what the server refused", () => {
    renderForm(null, "Revise os campos enviados.");
    expect(screen.getByText("Revise os campos enviados.")).toBeInTheDocument();
  });
});

describe("correcting a movement", () => {
  it("loads the recorded values and offers to delete it", async () => {
    const { onSubmit, onDelete } = renderForm(manualPurchase);
    expect(screen.getByRole("textbox", { name: "Quantidade" })).toHaveValue("7");
    expect(screen.getByRole("textbox", { name: "Observações (opcional)" })).toHaveValue("corretora antiga");
    await save();
    // 7 × 38 = 266: the total was priced by the server, so it is not sent back.
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ kind: "BUY", trade_date: "2026-01-05", quantity: "7", unit_price: "38", amount: null }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Excluir" }));
    expect(onDelete).toHaveBeenCalledWith(manualPurchase);
  });

  it("keeps a total that was typed, costs included", async () => {
    const { onSubmit } = renderForm({ ...manualPurchase, amount: "270.50" });
    await save();
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ quantity: "7", amount: "270.50" }));
  });
});
