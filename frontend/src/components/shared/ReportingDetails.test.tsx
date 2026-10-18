import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { ReportingSummary } from "../../api/contracts";
import { ReportingDetails } from "./ReportingDetails";

const report: ReportingSummary = { currency_code: "BRL", status: "unavailable", eligible: 1, converted: 0, missing: 1, stale: 0, totals: null };

describe("conversion feedback", () => {
  it.each([
    ["unsupported", "Sincronizar não resolve essa limitação."],
    ["historical_unavailable", "não há cotação para 18/05/2024"],
    ["provider_failure", "Tente novamente mais tarde."],
  ] as const)("explains %s while retaining native totals", (reason, text) => {
    render(<ReportingDetails report={{ ...report, issues: [{ currency: "XTS", account: "Conta sintética", date: "2024-05-18", reason }] }} totals={[{ currency_code: "XTS", inflow: "100", outflow: "0", balance: "100" }]} />);
    expect(screen.getByRole("status")).toHaveTextContent(text);
    expect(screen.getByRole("status")).toHaveTextContent("XTS (Conta sintética)");
    expect(screen.getByText("Totais por moeda")).toBeVisible();
    expect(screen.queryByText(/Faltam cotações históricas/)).toBeNull();
  });
});
