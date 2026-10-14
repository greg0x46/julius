import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { transactionResult } from "../../test/transactionFixtures";
import { detailValue } from "../../presentation/transactionDetail";
import { TransactionAmount } from "./TransactionAmount";

describe("transaction currency presentation", () => {
  it.each(["BRL", "USD", "EUR"])("shows effective %s money with the existing expense tone", (currency) => {
    const item = { ...transactionResult.items[0], effective_money: { value: "1234.50", currency_code: currency, source: "transaction_currency" as const } };
    render(<TransactionAmount item={item} />);
    const expected = `-${currency === "BRL" ? "R$" : currency}\u00a01.234,50`;
    expect(screen.getByText(expected.replaceAll("\u00a0", " "))).toHaveClass("money-neutral");
    expect(detailValue(item)).toBe(expected);
  });

  it.each(["USD", "EUR"])("shows %s income with a positive sign and tone", (currency) => {
    const item = {
      ...transactionResult.items[0],
      classification: "inflow" as const,
      effective_money: { value: "1234.50", currency_code: currency, source: "transaction_currency" as const },
    };
    render(<TransactionAmount item={item} />);
    expect(screen.getByText(`+${currency} 1.234,50`)).toHaveClass("money-positive");
    expect(detailValue(item)).toBe(`+${currency}\u00a01.234,50`);
  });

  it("uses the selected account amount without replacing the original amount", () => {
    const item = { ...transactionResult.items[0], amount: "10", currency_code: "USD", effective_money: { value: "52", currency_code: "BRL", source: "account_currency" as const } };
    render(<TransactionAmount item={item} />);
    expect(screen.getByText("-R$ 52,00")).toBeInTheDocument();
    expect(item.amount).toBe("10");
  });

  it("keeps missing money explicit", () => {
    const item = { ...transactionResult.items[0], effective_money: null };
    render(<TransactionAmount item={item} />);
    expect(screen.getByText("Valor indisponível")).toBeInTheDocument();
    expect(detailValue(item)).toBe("Valor indisponível");
  });
});
