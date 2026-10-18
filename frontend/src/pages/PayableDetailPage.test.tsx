import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

import * as payablesApi from "../api/payables";
import * as scenariosApi from "../api/scenarios";
import type { PayableDetail } from "../api/contracts";
import { QueryTestProvider } from "../test/QueryTestProvider";
import { PayableDetailPage } from "./PayableDetailPage";

vi.mock("../api/payables");
vi.mock("../api/scenarios");

const id = "44444444-4444-4444-8444-444444444444";
const receivable: PayableDetail = {
  id,
  kind: "receivable",
  name: "Empréstimo para Ana",
  total_amount: "1000",
  starting_settled_amount: "0",
  settled_amount: "0",
  remaining_amount: "1000",
  status: "open",
  link_count: 0,
  created_at: "2026-07-30T12:00:00Z",
  updated_at: "2026-07-30T12:00:00Z",
  links: [],
};

describe("PayableDetailPage", () => {
  it("takes the kind from the loaded payable when the URL has no ?kind=", async () => {
    vi.mocked(payablesApi.getPayable).mockResolvedValue(receivable);
    vi.mocked(payablesApi.listEligibleTransactions).mockResolvedValue([]);
    vi.mocked(payablesApi.listPayables).mockResolvedValue([]);
    vi.mocked(scenariosApi.listPayableScenarios).mockResolvedValue([]);
    render(
      <MemoryRouter initialEntries={[`/pendencias/${id}`]}>
        <QueryTestProvider>
          <Routes>
            <Route path="/pendencias/:id" element={<PayableDetailPage />} />
          </Routes>
        </QueryTestProvider>
      </MemoryRouter>,
    );

    expect(await screen.findByText("Restante a receber")).toBeVisible();
    await waitFor(() =>
      expect(vi.mocked(payablesApi.listEligibleTransactions).mock.calls.at(-1)?.[0]).toBe("receivable"),
    );
    expect(await screen.findByRole("button", { name: /Adicionar recebimento/ })).toBeVisible();
  });
});
