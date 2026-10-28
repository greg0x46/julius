import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, onTestFinished, vi } from "vitest";

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
    fireEvent.click(screen.getByRole("combobox", { name: "Tipo" }));
    const option = (label: string) => screen.queryByText(label, { selector: '[role="option"]' });
    expect(await screen.findByText("Compra", { selector: '[role="option"]' })).toBeInTheDocument();
    expect(option("Venda")).toBeInTheDocument();
    expect(option("Cotação manual")).toBeInTheDocument();
    expect(option("Saldo inicial")).toBeNull();
  });

  it("scrolls the validation error into view", () => {
    const scrollIntoView = vi.fn();
    Element.prototype.scrollIntoView = scrollIntoView;
    onTestFinished(() => {
      delete (Element.prototype as Partial<Element>).scrollIntoView;
    });
    const { container } = render(
      <InvestmentOperationFields
        formId="operation"
        operation={null}
        accounts={[integratedAccount]}
        positions={[]}
        submitError={null}
        onSubmit={vi.fn()}
      />,
    );
    fireEvent.submit(container.querySelector("form")!);
    expect(screen.getByText("Informe um valor maior que zero.")).toBeInTheDocument();
    expect(scrollIntoView).toHaveBeenCalled();
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
