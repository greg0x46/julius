import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { parseTransactionQueryResult } from "../../api/contracts";
import { ReportCurrencyContext } from "../../hooks/useReportCurrency";
import { formatReportMoney } from "../../presentation/money";
import { transactionResult } from "../../test/transactionFixtures";
import { TransactionGroupHeader } from "./TransactionGroupHeader";
import { TransactionSummaryBar } from "./TransactionSummaryBar";

const native = transactionResult.totals;

describe("transaction reference totals", () => {
  it("parses report totals while preserving native totals and money", () => {
    const report = { currency_code: "USD", inflow: "0", outflow: "24.69", balance: "-24.69" };
    const parsed = parseTransactionQueryResult({
      ...transactionResult,
      currency_code: "USD",
      report_totals: report,
      groups: transactionResult.groups.map((group) => ({ ...group, report_totals: report })),
    });
    expect(parsed.report_totals).toEqual(report);
    expect(parsed.groups[0].report_totals).toEqual(report);
    expect(parsed.totals).toEqual(native);
    expect(parsed.items[0].effective_money).toEqual(transactionResult.items[0].effective_money);
  });

  it("displays converted summaries and group results across BRL → USD → BRL", () => {
    const view = render(<div />);
    for (const currency of ["BRL", "USD", "BRL"]) {
      const report = currency === "USD"
        ? { currency_code: "USD", inflow: "0", outflow: "24.69", balance: "-24.69" }
        : native[0];
      view.rerender(
        <ReportCurrencyContext.Provider value="EUR">
          <TransactionSummaryBar totalItems={1} totals={native} reportTotals={report} />
          <TransactionGroupHeader group={{ ...transactionResult.groups[0], report_totals: report }} />
        </ReportCurrencyContext.Provider>,
      );
      expect(screen.getAllByText(formatReportMoney(report.balance, currency).replace(/\u00a0/g, " "))).toHaveLength(2);
      expect(screen.getByText(formatReportMoney(report.outflow, currency).replace(/\u00a0/g, " "))).toBeInTheDocument();
    }
    expect(native[0].balance).toBe("-123.4500");
    expect(transactionResult.items[0].effective_money?.currency_code).toBe("BRL");
  });

  it("keeps legacy native BRL totals labeled BRL under a different report context", () => {
    render(
      <ReportCurrencyContext.Provider value="USD">
        <TransactionSummaryBar totalItems={1} totals={native} />
        <TransactionGroupHeader group={transactionResult.groups[0]} />
      </ReportCurrencyContext.Provider>,
    );
    expect(screen.getAllByText(formatReportMoney(native[0].balance, "BRL").replace(/\u00a0/g, " "))).toHaveLength(2);
  });
});
