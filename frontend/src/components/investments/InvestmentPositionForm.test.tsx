import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";

import type { InvestmentAsset } from "../../api/contracts";
import { listInvestmentAssets } from "../../api/investmentPortfolio";
import { integratedAccount, manualAccount } from "../../test/investmentWorkspaceFixtures";
import { InvestmentPositionForm } from "./InvestmentPositionForm";

vi.mock("../../api/investmentPortfolio", () => ({ listInvestmentAssets: vi.fn() }));

it("creates a position from the catalog even when the asset has no positions", async () => {
  const asset: InvestmentAsset = {
    id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb2", name: "Bitcoin", ticker: "BTC",
    asset_type: "Criptomoeda", asset_class: "crypto", currency_code: "BRL",
    quote_source: "crypto", quote_symbol: "BTC",
    created_at: "2026-09-01T12:00:00Z", updated_at: "2026-09-01T12:00:00Z",
  };
  vi.mocked(listInvestmentAssets).mockResolvedValue([asset]);
  const onCreate = vi.fn();
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <InvestmentPositionForm open position={null} accounts={[manualAccount]} positions={[]} portfolios={[]}
        submitting={false} submitError={null} onCreate={onCreate} onUpdate={vi.fn()} onCancel={vi.fn()} />
    </QueryClientProvider>,
  );
  await screen.findByText("Ativo existente (opcional)");
  fireEvent.mouseDown(await screen.findByRole("combobox", { name: "Ativo existente (opcional)" }));
  await userEvent.click(await screen.findByText("BTC · Bitcoin", { selector: ".ant-select-item-option-content" }));
  expect(screen.getByRole("textbox", { name: "Nome" })).toHaveValue("Bitcoin");
  await userEvent.click(screen.getByRole("button", { name: "Salvar" }));
  await waitFor(() => expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({
    account_id: manualAccount.id, asset_id: asset.id, name: "Bitcoin", ticker: "BTC", asset_type: "Criptomoeda",
  })));
  client.clear();
});

it("offers integrated accounts too, so a manual holding can sit next to the provider's", async () => {
  vi.mocked(listInvestmentAssets).mockResolvedValue([]);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <InvestmentPositionForm open position={null} accounts={[manualAccount, integratedAccount]} positions={[]} portfolios={[]}
        submitting={false} submitError={null} onCreate={vi.fn()} onUpdate={vi.fn()} onCancel={vi.fn()} />
    </QueryClientProvider>,
  );
  fireEvent.mouseDown(await screen.findByRole("combobox", { name: "Conta de investimento" }));
  expect(await screen.findByText(integratedAccount.name, { selector: ".ant-select-item-option-content" })).toBeInTheDocument();
  client.clear();
});
