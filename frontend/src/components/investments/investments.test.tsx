import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { integratedAccount } from "../../test/investmentWorkspaceFixtures";
import { InvestmentOperationFields } from "./InvestmentOperationForm";
import { fromMoneyInput, toMoneyInput } from "./moneyDraft";

describe("InvestmentOperationFields", () => {
  it("offers trades for the manual holdings of an integrated account, but no cash opening balance", async () => {
    render(
      <InvestmentOperationFields
        formId="operation"
        operation={null}
        accounts={[integratedAccount]}
        positions={[]}
        submitError={null}
        onSubmit={vi.fn()}
      />,
    );
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Tipo" }));
    const option = (label: string) => screen.queryByText(label, { selector: ".ant-select-item-option-content" });
    expect(await screen.findByText("Compra", { selector: ".ant-select-item-option-content" })).toBeInTheDocument();
    expect(option("Venda")).toBeInTheDocument();
    expect(option("Cotação manual")).toBeInTheDocument();
    expect(option("Saldo inicial")).toBeNull();
  });
});

describe("moneyDraft", () => {
  it("round-trips a decimal string through the money field without drifting", () => {
    expect(toMoneyInput("1234.50")).toBe("1234.50");
    expect(fromMoneyInput("1234.5")).toBe("1234.50");
    expect(toMoneyInput(null)).toBeNull();
    expect(fromMoneyInput(null)).toBeNull();
  });
});
