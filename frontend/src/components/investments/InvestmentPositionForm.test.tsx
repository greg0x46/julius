import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { InvestmentAccount, InvestmentAsset } from "../../api/contracts";
import { integratedAccount, manualAccount } from "../../test/investmentWorkspaceFixtures";
import { InvestmentPositionForm } from "./InvestmentPositionForm";

const classification = [
  { asset_class: "fixed_income", label: "Renda fixa", types: [{ name: "CDB", quote_market: null }] },
  { asset_class: "crypto", label: "Criptoativos", types: [{ name: "Criptomoeda", quote_market: "crypto" }] },
];

const bitcoin: InvestmentAsset = {
  id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb2", name: "Bitcoin", ticker: "BTC",
  asset_type: "Criptomoeda", asset_class: "crypto", currency_code: "BRL",
  quote_source: "crypto", quote_symbol: "BTC",
  created_at: "2026-09-01T12:00:00Z", updated_at: "2026-09-01T12:00:00Z",
};
const petrobras: InvestmentAsset = {
  ...bitcoin, id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb3", name: "Petrobras PN", ticker: "PETR4",
  asset_type: "Ação", asset_class: "variable_income", quote_source: "b3", quote_symbol: "PETR4",
};

const mocked = vi.hoisted(() => ({ assets: [] as unknown[], createAsset: vi.fn() }));

vi.mock("../../hooks/useInvestmentAssets", () => ({
  useInvestmentAssets: () => ({
    assets: mocked.assets, classification, isLoading: false, error: null,
    refetch: vi.fn(), createAsset: mocked.createAsset,
  }),
}));

beforeEach(() => {
  vi.clearAllMocks();
  mocked.assets = [bitcoin, petrobras];
});

function renderForm(accounts: InvestmentAccount[] = [manualAccount]) {
  const onCreate = vi.fn();
  render(
    <InvestmentPositionForm open position={null} accounts={accounts} portfolios={[]}
      submitting={false} submitError={null} onCreate={onCreate} onUpdate={vi.fn()} onCancel={vi.fn()} />,
  );
  return onCreate;
}

async function choose(label: string, option: string) {
  fireEvent.mouseDown(screen.getByRole("combobox", { name: label }));
  await userEvent.click(await screen.findByText(option, { selector: ".ant-select-item-option-content" }));
}

function search(text: string) {
  const input = screen.getByRole("combobox", { name: "Ativo" });
  fireEvent.mouseDown(input);
  fireEvent.change(input, { target: { value: text } });
}

function fillOpeningValue() {
  fireEvent.change(screen.getByRole("spinbutton", { name: "Quantidade inicial" }), { target: { value: "0,5" } });
  fireEvent.change(screen.getByRole("spinbutton", { name: "Custo unitário" }), { target: { value: "300000" } });
}

const save = () => userEvent.click(screen.getByRole("button", { name: "Salvar" }));

describe("asset selection from the catalog", () => {
  it.each(["BTC", "bitcoin"])("finds Bitcoin by %s and reuses it without creating an asset", async (text) => {
    const onCreate = renderForm();
    search(text);
    expect(screen.queryByText("PETR4 · Petrobras PN", { selector: ".ant-select-item-option-content" })).toBeNull();
    await userEvent.click(await screen.findByText("BTC · Bitcoin", { selector: ".ant-select-item-option-content" }));
    expect(screen.getByText("Criptoativos · Criptomoeda · BRL · Cotação automática")).toBeInTheDocument();
    fillOpeningValue();
    await save();
    await waitFor(() => expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({
      account_id: manualAccount.id, asset_id: bitcoin.id, name: "Bitcoin", ticker: "BTC", asset_type: "Criptomoeda",
      initial_quantity: "0.5", initial_unit_cost: "300000",
    })));
    expect(mocked.createAsset).not.toHaveBeenCalled();
  });

  it("requires an asset instead of free text", async () => {
    const onCreate = renderForm();
    expect(screen.queryByRole("textbox", { name: "Nome" })).toBeNull();
    await save();
    expect(await screen.findByText("Escolha um ativo ou cadastre um novo.")).toBeInTheDocument();
    expect(onCreate).not.toHaveBeenCalled();
  });

  it("preserves entered position data when accounts refresh", async () => {
    const onCreate = vi.fn();
    const secondAccount = { ...manualAccount, id: "second-account", name: "Outra conta" };
    const props = {
      open: true, position: null, accounts: [manualAccount, secondAccount], portfolios: [],
      submitting: false, submitError: null, onCreate, onUpdate: vi.fn(), onCancel: vi.fn(),
    };
    const view = render(<InvestmentPositionForm {...props} />);
    await choose("Conta de investimento", secondAccount.name);
    await choose("Ativo", "BTC · Bitcoin");
    fillOpeningValue();
    view.rerender(<InvestmentPositionForm {...props} accounts={[{ ...manualAccount, cash_balance: "550.00" }, { ...secondAccount }]} />);
    await save();
    await waitFor(() => expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({
      account_id: secondAccount.id, asset_id: bitcoin.id,
      initial_quantity: "0.5", initial_unit_cost: "300000",
    })));
  });
});

describe("new asset inside the position form", () => {
  it("creates Criptoativos → Criptomoeda → BTC with the crypto quote market, then the position", async () => {
    mocked.assets = [petrobras];
    const created = { ...bitcoin, id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb9" };
    mocked.createAsset.mockResolvedValue(created);
    const onCreate = renderForm();
    search("BTC");
    fillOpeningValue();
    await userEvent.click(screen.getByRole("button", { name: "+ Cadastrar novo ativo" }));
    expect(screen.getByRole("group", { name: "Novo ativo" })).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Código (opcional)" })).toHaveValue("BTC");
    await choose("Classe", "Criptoativos");
    await choose("Tipo", "Criptomoeda");
    expect(screen.getByRole("switch", { name: "Cotação automática" })).toBeChecked();
    fireEvent.change(screen.getByRole("textbox", { name: "Nome" }), { target: { value: "Bitcoin" } });
    // The opening values typed before switching to a new asset survive.
    expect(screen.getByRole("spinbutton", { name: "Quantidade inicial" })).toHaveValue("0,5");
    await save();
    await waitFor(() => expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({
      account_id: manualAccount.id, asset_id: created.id, initial_quantity: "0.5",
    })));
    expect(mocked.createAsset).toHaveBeenCalledWith({
      name: "Bitcoin", ticker: "BTC", asset_type: "Criptomoeda", asset_class: "crypto",
      currency_code: "BRL", quote_source: "crypto", quote_symbol: null,
    });
  });

  it("keeps a type without a quote market on manual valuation", async () => {
    mocked.assets = [];
    mocked.createAsset.mockResolvedValue({ ...bitcoin, name: "CDB Banco 2028", ticker: null, asset_type: "CDB" });
    const onCreate = renderForm();
    // An empty catalog opens straight on the new asset, with no way back to the picker.
    expect(screen.queryByRole("button", { name: "Escolher ativo existente" })).toBeNull();
    await choose("Classe", "Renda fixa");
    await choose("Tipo", "CDB");
    expect(screen.getByRole("switch", { name: "Cotação automática" })).toBeDisabled();
    fireEvent.change(screen.getByRole("textbox", { name: "Nome" }), { target: { value: "CDB Banco 2028" } });
    await save();
    await waitFor(() => expect(onCreate).toHaveBeenCalled());
    expect(mocked.createAsset).toHaveBeenCalledWith(expect.objectContaining({
      asset_class: "fixed_income", asset_type: "CDB", quote_source: null, quote_symbol: null,
    }));
  });

  it("selects the existing catalog asset instead of creating a duplicate", async () => {
    const onCreate = renderForm();
    await userEvent.click(screen.getByRole("button", { name: "+ Cadastrar novo ativo" }));
    await choose("Classe", "Criptoativos");
    await choose("Tipo", "Criptomoeda");
    fireEvent.change(screen.getByRole("textbox", { name: "Nome" }), { target: { value: "Bitcoin" } });
    fireEvent.change(screen.getByRole("textbox", { name: "Código" }), { target: { value: "btc" } });
    await save();
    expect(await screen.findByText(/Já existe o ativo BTC · Bitcoin no catálogo/)).toBeInTheDocument();
    expect(mocked.createAsset).not.toHaveBeenCalled();
    expect(onCreate).not.toHaveBeenCalled();
    expect(screen.getByText("Criptoativos · Criptomoeda · BRL · Cotação automática")).toBeInTheDocument();
    await save();
    await waitFor(() => expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({ asset_id: bitcoin.id })));
    expect(mocked.createAsset).not.toHaveBeenCalled();
  });

  it("reuses the asset it created when the position is saved again", async () => {
    mocked.createAsset.mockResolvedValue({ ...bitcoin, id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb9", ticker: "ETH", name: "Ethereum" });
    const onCreate = renderForm();
    await userEvent.click(screen.getByRole("button", { name: "+ Cadastrar novo ativo" }));
    await choose("Classe", "Criptoativos");
    await choose("Tipo", "Criptomoeda");
    fireEvent.change(screen.getByRole("textbox", { name: "Nome" }), { target: { value: "Ethereum" } });
    fireEvent.change(screen.getByRole("textbox", { name: "Código" }), { target: { value: "ETH" } });
    await save();
    await waitFor(() => expect(onCreate).toHaveBeenCalledTimes(1));
    await save();
    await waitFor(() => expect(onCreate).toHaveBeenCalledTimes(2));
    expect(mocked.createAsset).toHaveBeenCalledTimes(1);
  });

  it("retries a position from an empty catalog after account invalidation without recreating its asset", async () => {
    mocked.assets = [];
    const onCreate = vi.fn();
    const props = {
      open: true, position: null, accounts: [manualAccount], portfolios: [],
      submitting: false, submitError: null, onCreate, onUpdate: vi.fn(), onCancel: vi.fn(),
    };
    const view = render(<InvestmentPositionForm {...props} />);
    mocked.createAsset.mockImplementation(async () => {
      view.rerender(<InvestmentPositionForm {...props} accounts={[{ ...manualAccount, cash_balance: "550.00" }]} />);
      return bitcoin;
    });
    await choose("Classe", "Criptoativos");
    await choose("Tipo", "Criptomoeda");
    fireEvent.change(screen.getByRole("textbox", { name: "Nome" }), { target: { value: "Bitcoin" } });
    fireEvent.change(screen.getByRole("textbox", { name: "Código" }), { target: { value: "BTC" } });
    fillOpeningValue();
    fireEvent.click(screen.getByRole("button", { name: "Salvar" }));
    await waitFor(() => expect(onCreate).toHaveBeenCalledTimes(1));
    view.rerender(<InvestmentPositionForm {...props} accounts={[{ ...manualAccount, cash_balance: "550.00" }]} submitError="Falha ao salvar posição" />);
    expect(screen.queryByRole("group", { name: "Novo ativo" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Salvar" }));
    await waitFor(() => expect(onCreate).toHaveBeenCalledTimes(2));
    expect(onCreate).toHaveBeenLastCalledWith(expect.objectContaining({
      account_id: manualAccount.id, asset_id: bitcoin.id,
      initial_quantity: "0.5", initial_unit_cost: "300000",
    }));
    expect(mocked.createAsset).toHaveBeenCalledTimes(1);
  }, 30_000);
});

describe("account selection", () => {
  it("offers only manual accounts and preselects the first", async () => {
    const onCreate = renderForm([integratedAccount, manualAccount]);
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Conta de investimento" }));
    expect(await screen.findAllByText(manualAccount.name)).not.toHaveLength(0);
    expect(screen.queryByText(integratedAccount.name, { selector: ".ant-select-item-option-content" })).toBeNull();
    await choose("Ativo", "BTC · Bitcoin");
    await save();
    await waitFor(() => expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({ account_id: manualAccount.id })));
  });

  it("refuses to save without a manual account", async () => {
    const onCreate = renderForm([integratedAccount]);
    await choose("Ativo", "BTC · Bitcoin");
    await save();
    expect(await screen.findByText("Selecione uma conta de investimento manual.")).toBeInTheDocument();
    expect(onCreate).not.toHaveBeenCalled();
  });
});
