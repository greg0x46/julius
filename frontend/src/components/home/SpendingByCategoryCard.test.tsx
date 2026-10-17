import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { expect, it, vi } from "vitest";
import * as categoryHook from "../../hooks/useCategoryBreakdown";
import { SpendingByCategoryCard } from "./SpendingByCategoryCard";

vi.mock("../../hooks/useCategoryBreakdown");
it("replaces the category chart and zero state when FX coverage is missing", () => {
  vi.mocked(categoryHook.useCategoryBreakdown).mockReturnValue({
    data: { month: "2026-03", classification: "outflow", currency_code: "BRL", total: null, items: [], reporting: { currency_code: "BRL", status: "unavailable", eligible: 1, converted: 0, missing: 1, stale: 0, totals: null }, totals: [{ currency_code: "EUR", inflow: "0", outflow: "10", balance: "-10" }] },
    isLoading: false, error: null, refetch: vi.fn(),
  } as unknown as ReturnType<typeof categoryHook.useCategoryBreakdown>);
  render(<MemoryRouter><SpendingByCategoryCard period={{ from: "2026-03-01", to: "2026-03-31" }} /></MemoryRouter>);
  expect(screen.getByText("Total indisponível")).toBeInTheDocument();
  expect(screen.getByText("Faltam cotações históricas para 1 transação.")).toBeInTheDocument();
  expect(screen.getByText("Saídas (EUR)")).toBeInTheDocument();
  expect(screen.queryByText("R$ 0,00")).not.toBeInTheDocument();
  expect(screen.queryByText("Nenhuma saída neste período.")).not.toBeInTheDocument();
});
