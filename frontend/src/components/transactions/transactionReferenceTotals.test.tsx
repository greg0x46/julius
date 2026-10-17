import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { parseTransactionQueryResult, type ReportingSummary } from "../../api/contracts";
import { formatReportMoney } from "../../presentation/money";
import { transactionResult } from "../../test/transactionFixtures";
import { TransactionGroupHeader } from "./TransactionGroupHeader";
import { TransactionSummaryBar } from "./TransactionSummaryBar";

const native = transactionResult.totals;

function summary(currency: string, totals: { inflow: string; outflow: string; balance: string }): ReportingSummary {
  return {
    currency_code: currency,
    status: "complete",
    eligible: 1,
    converted: 1,
    missing: 0,
    stale: 0,
    totals: { currency_code: currency, ...totals },
  };
}

const usd = summary("USD", { inflow: "0", outflow: "24.69", balance: "-24.69" });

describe("transaction reference totals", () => {
  it("parses reference-currency summaries while preserving native totals and money", () => {
    const parsed = parseTransactionQueryResult({
      ...transactionResult,
      currency_code: "USD",
      reporting: usd,
      groups: transactionResult.groups.map((group) => ({ ...group, reporting: usd })),
    });
    expect(parsed.reporting).toEqual(usd);
    expect(parsed.groups[0].reporting).toEqual(usd);
    expect(parsed.totals).toEqual(native);
    expect(parsed.items[0].effective_money).toEqual(transactionResult.items[0].effective_money);
  });

  it("displays converted summaries and group results across BRL → USD → BRL", () => {
    const view = render(<div />);
    for (const currency of ["BRL", "USD", "BRL"]) {
      const report = currency === "USD" ? usd : summary("BRL", native[0]);
      view.rerender(
        <>
          <TransactionSummaryBar totalItems={1} totals={native} reporting={report} />
          <TransactionGroupHeader group={{ ...transactionResult.groups[0], reporting: report }} />
        </>,
      );
      const totals = report.totals!;
      expect(screen.getAllByText(formatReportMoney(totals.balance, currency).replace(/\u00a0/g, " "))).toHaveLength(2);
      expect(screen.getByText(formatReportMoney(totals.outflow, currency).replace(/\u00a0/g, " "))).toBeInTheDocument();
    }
    expect(native[0].balance).toBe("-123.4500");
    expect(transactionResult.items[0].effective_money?.currency_code).toBe("BRL");
  });
});
