import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ReportingSummary } from "../../api/contracts";
import { TransactionSummaryBar } from "./TransactionSummaryBar";

vi.mock("../shared/useCompactScreen", () => ({ useCompactScreen: () => false }));
const totals = [{ currency_code: "BRL", inflow: "100", outflow: "0", balance: "100" }, { currency_code: "USD", inflow: "10", outflow: "0", balance: "10" }];
const reporting: ReportingSummary = { currency_code: "BRL", status: "complete", eligible: 2, converted: 2, missing: 0, stale: 0, totals: { currency_code: "BRL", inflow: "150", outflow: "0", balance: "150" } };

describe("reporting summary", () => {
  it("displays the consolidated BRL amount with currency drill-down", () => {
    render(<TransactionSummaryBar totalItems={2} totals={totals} reporting={reporting} />);
    expect(screen.getByText("+R$ 150,00")).toBeInTheDocument();
    expect(screen.getByText("Totais por moeda")).toBeInTheDocument();
    expect(screen.getByText("Entradas (USD)")).toBeInTheDocument();
  });
  it("does not present a subtotal as a complete result when rates are missing", () => {
    render(<TransactionSummaryBar totalItems={2} totals={totals} reporting={{ ...reporting, status: "incomplete", converted: 1, missing: 1, totals: null }} />);
    expect(screen.getByText("Total incompleto")).toBeInTheDocument();
    expect(screen.getByText("Faltam cotações históricas para 1 transação.")).toBeInTheDocument();
    expect(screen.queryByText("+R$ 150,00")).not.toBeInTheDocument();
  });
  it("retains BRL-only presentation without extra controls", () => {
    render(<TransactionSummaryBar totalItems={1} totals={[totals[0]]} />);
    expect(screen.getByText("+R$ 100,00")).toBeInTheDocument();
    expect(screen.queryByText("Totais por moeda")).not.toBeInTheDocument();
  });
  it("does not silently drop foreign currencies from an older response", () => {
    render(<TransactionSummaryBar totalItems={2} totals={totals} />);
    expect(screen.getByText("Total indisponível")).toBeInTheDocument();
  });
});
