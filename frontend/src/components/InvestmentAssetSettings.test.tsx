import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { InvestmentAssetSettings } from "./InvestmentAssetSettings";

const mocked = vi.hoisted(() => ({ createAsset: vi.fn(), updateAsset: vi.fn() }));
const classification = [
  { asset_class: "fixed_income", label: "Renda fixa", types: [{ name: "CDB", quote_market: null }, { name: "ETF de renda fixa", quote_market: "b3" }] },
  { asset_class: "crypto", label: "Criptoativos", types: [{ name: "Criptomoeda", quote_market: "crypto" }, { name: "ETF de criptoativos", quote_market: "b3" }] },
];

vi.mock("../hooks/useInvestmentAssets", () => ({
  useInvestmentAssets: () => ({
    assets: [], classification, isLoading: false, error: null,
    createAsset: mocked.createAsset, updateAsset: mocked.updateAsset,
    isSaving: false, isDeleting: false,
  }),
}));

async function choose(user: ReturnType<typeof userEvent.setup>, form: ReturnType<typeof within>, label: string, option: string) {
  fireEvent.mouseDown(form.getByRole("combobox", { name: label }));
  await user.click(await screen.findByText(option, { selector: ".ant-select-item-option-content" }));
}

beforeEach(() => { vi.clearAllMocks(); });

describe("financial asset creation", () => {
  it.each([
    ["Criptoativos", "Criptomoeda", "BTC", "crypto", "crypto"],
    ["Criptoativos", "ETF de criptoativos", "HASH11", "crypto", "b3"],
    ["Renda fixa", "ETF de renda fixa", "LFTB11", "fixed_income", "b3"],
  ])("routes %s / %s without a provider selector", async (label, type, ticker, assetClass, market) => {
    const user = userEvent.setup();
    render(<InvestmentAssetSettings creating onCreatingClose={vi.fn()} />);
    const form = within(await screen.findByRole("dialog"));
    await choose(user, form, "Classe", label);
    await choose(user, form, "Tipo", type);
    fireEvent.change(form.getByLabelText("Nome"), { target: { value: "Ativo de teste" } });
    fireEvent.change(form.getByLabelText("Código"), { target: { value: ticker } });
    expect(form.getByRole("switch", { name: "Cotação automática" })).toBeChecked();
    await user.click(form.getByRole("button", { name: "Salvar" }));
    await waitFor(() => expect(mocked.createAsset).toHaveBeenCalledWith({
      name: "Ativo de teste", ticker, asset_type: type, asset_class: assetClass,
      currency_code: "BRL", quote_source: market, quote_symbol: null,
    }));
  });

  it("saves fixed income without automatic prices and filters instrument types by class", async () => {
    const user = userEvent.setup();
    render(<InvestmentAssetSettings creating onCreatingClose={vi.fn()} />);
    const form = within(await screen.findByRole("dialog"));
    await choose(user, form, "Classe", "Renda fixa");
    await choose(user, form, "Tipo", "CDB");
    fireEvent.change(form.getByLabelText("Nome"), { target: { value: "CDB Banco 2028" } });
    expect(form.getByRole("switch", { name: "Cotação automática" })).toBeDisabled();
    await user.click(form.getByRole("button", { name: "Salvar" }));
    await waitFor(() => expect(mocked.createAsset).toHaveBeenCalledWith(expect.objectContaining({
      asset_class: "fixed_income", asset_type: "CDB", quote_source: null, quote_symbol: null,
    })));
  });
});
