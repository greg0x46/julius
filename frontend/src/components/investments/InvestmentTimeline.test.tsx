import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { InvestmentTransaction } from "../../api/contracts";
import { TimezoneContext } from "../../hooks/useTimezone";
import { InvestmentTimeline } from "./InvestmentTimeline";

const purchase: InvestmentTransaction = {
  id: "manual-purchase",
  external_id: "manual-purchase",
  origin: "manual",
  movement_type: "BUY",
  direction: "inflow",
  quantity: "7",
  value: "38",
  amount: "266",
  occurred_at: null,
  trade_date: "2026-01-05T00:00:00Z",
  notes: null,
};

describe("investment movement dates", () => {
  it("keeps the recorded calendar day while rendering provider instants in the owner's timezone", () => {
    render(
      <TimezoneContext.Provider value="America/Sao_Paulo">
        <InvestmentTimeline
          transactions={[
            purchase,
            { ...purchase, id: "provider-purchase", origin: "provider", occurred_at: "2026-01-05T00:00:00Z" },
          ]}
          isLoading={false}
          error={null}
          onRetry={vi.fn()}
          onRecord={vi.fn()}
          onEdit={vi.fn()}
        />
      </TimezoneContext.Provider>,
    );

    expect(screen.getByText(/^05 jan\..*Manual/)).toBeInTheDocument();
    expect(screen.getByText(/^04 jan\./)).toBeInTheDocument();
  });
});
