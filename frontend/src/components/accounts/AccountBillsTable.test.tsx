import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { AccountBillsTable } from "./AccountBillsTable";

describe("AccountBillsTable", () => {
  it("shows the error without claiming there are no bills", () => {
    render(<AccountBillsTable bills={[]} isLoading={false} error={new Error("x")} />);
    expect(screen.getByText("Não foi possível carregar as faturas desta conta.")).toBeInTheDocument();
    expect(screen.queryByText("Nenhuma fatura fechada")).not.toBeInTheDocument();
  });

  it("shows the empty state when the load succeeded", () => {
    render(<AccountBillsTable bills={[]} isLoading={false} error={null} />);
    expect(screen.getByText("Nenhuma fatura fechada")).toBeInTheDocument();
  });
  it("does not render a dash after 'Fechou em' when the closing date is unknown", () => {
    const bill = {
      id: "b1",
      external_id: "e1",
      due_date: "2026-01-09",
      closing_date: null,
      total_amount: "100.00",
      currency_code: "BRL",
      minimum_payment_amount: "10.00",
    };
    render(<AccountBillsTable bills={[bill]} isLoading={false} error={null} />);
    expect(screen.getByText("Fechamento não informado")).toBeInTheDocument();
    expect(screen.queryByText(/Fechou em/)).not.toBeInTheDocument();
  });
});
