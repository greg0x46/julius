import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

import type {
  Investment,
  InvestmentAccount,
  InvestmentOperation,
  InvestmentPortfolio,
  InvestmentPosition,
  InvestmentSummary,
} from "../../api/contracts";
import { InvestmentAccountCard } from "./InvestmentAccountCard";
import { InvestmentGoalCard } from "./InvestmentGoalCard";
import { InvestmentWorkspaceSummary } from "./InvestmentWorkspaceSummary";

const account = {
  id: "a1",
  name: "Nubank",
  kind: "integrated",
  active: true,
  cash_balance: "0.00",
  financial_account_id: null,
  updated_at: "2026-09-30T10:00:00Z",
} as unknown as InvestmentAccount;

// A holding the institution reports: it states the gain and withholds IR/IOF.
const lca = {
  id: "i1",
  yield_value: "120.50",
  amount_original: "1000.00",
  taxes: "15.00",
  taxes2: "2.00",
} as unknown as Investment;

const syncedPosition = {
  id: "p1",
  source: "synced",
  account_id: "a1",
  asset_id: null,
  portfolio_id: "g1",
  name: "LCA Nubank",
  ticker: null,
  asset_type: "LCA",
  quantity: "1",
  average_cost: null,
  current_value: "1120.50",
  current_unit_price: null,
  valued_on: "2026-09-30",
  currency_code: "BRL",
  closed: false,
  linked_investment_id: "i1",
  notes: null,
  valuation_basis: "provider_balance",
} as InvestmentPosition;

// A manual position with a cost basis: 10 × R$ 100 bought, now worth R$ 1.200.
const manualPosition = {
  ...syncedPosition,
  id: "p2",
  source: "manual",
  name: "Fundo X",
  asset_type: "Fundo",
  quantity: "10",
  average_cost: "100.00",
  current_value: "1200.00",
  linked_investment_id: null,
  valuation_basis: "manual_valuation",
} as InvestmentPosition;

const fee = {
  id: "o1",
  account_id: "a1",
  position_id: "p2",
  kind: "buy",
  amount: "1000.00",
  fees: "5.00",
  taxes: null,
  occurred_on: "2026-09-01",
  source: "manual",
  is_editable: true,
  notes: null,
} as unknown as InvestmentOperation;

const goal = { id: "g1", name: "Reserva", target_amount: null, progress: null, current_value: "2320.50" } as unknown as InvestmentPortfolio;
const linked = new Map([[lca.id, lca]]);

function renderAccountCard() {
  render(
    <MemoryRouter>
      <InvestmentAccountCard
        account={account}
        summary={undefined}
        positions={[syncedPosition, manualPosition]}
        allPositions={[syncedPosition, manualPosition]}
        operations={[fee]}
        portfolios={[goal]}
        linked={linked}
        onRename={vi.fn()}
        onRemove={vi.fn()}
        onNewPosition={vi.fn()}
        onNewOperation={vi.fn()}
        onEditPosition={vi.fn()}
        onRemovePosition={vi.fn()}
        onEditOperation={vi.fn()}
        onRemoveOperation={vi.fn()}
        onAssignGoal={vi.fn()}
        busy={false}
      />
    </MemoryRouter>,
  );
}

describe("investment cards", () => {
  it("shows the rendimento líquido and the IR/Taxas on the account, net of what the institution withheld", () => {
    renderAccountCard();
    const figures = screen.getByRole("region", { name: "Nubank" });
    // Synced: 120,50 of gain − 17,00 IR/IOF. Manual: 1.200 − (10 × 100) − 5,00 of fees = 195,00. Total 298,50.
    expect(within(figures).getByText("+R$ 298,50")).toBeVisible();
    // 17,00 withheld by the institution + 5,00 of fees.
    expect(within(figures).getByRole("button", { name: "IR/Taxas" })).toBeInTheDocument();
    expect(within(figures).getAllByText("R$ 22,00").length).toBeGreaterThan(0);
  });

  it("keeps the account's movements behind a disclosure and the menu items main defined", () => {
    renderAccountCard();
    expect(screen.getByText("Movimentações (1)")).toBeVisible();
    expect(screen.getByRole("button", { name: "Ações de Nubank" })).toBeVisible();
  });

  it("lists each position with its own rendimento and the IR/Taxas that were deducted from it", () => {
    renderAccountCard();
    const table = screen.getByRole("table", { name: "Posições" });
    const lcaRow = within(table).getByText("LCA Nubank").closest("tr")!;
    expect(within(lcaRow).getByText("+R$ 103,50")).toBeVisible();
    expect(within(lcaRow).getByText("IR/Taxas R$ 17,00")).toBeVisible();
    const fundRow = within(table).getByText("Fundo X").closest("tr")!;
    expect(within(fundRow).getByText("+R$ 195,00")).toBeVisible();
    expect(within(fundRow).getByText("Custo médio R$ 100,00")).toBeVisible();
  });

  it("offers a new manual position on an integrated account but keeps the account itself read-only", async () => {
    renderAccountCard();
    await userEvent.click(screen.getByRole("button", { name: "Ações de Nubank" }));
    expect(await screen.findByRole("menuitem", { name: "Nova posição" })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Editar conta" })).toBeNull();
    expect(screen.queryByRole("menuitem", { name: "Excluir conta" })).toBeNull();
  });

  it("tags the manual holding of an integrated account and lets it be edited and deleted", async () => {
    renderAccountCard();
    const table = screen.getByRole("table", { name: "Posições" });
    expect(within(within(table).getByText("Fundo X").closest("tr")!).getByText(/· Manual/)).toBeVisible();
    expect(within(within(table).getByText("LCA Nubank").closest("tr")!).queryByText(/· Manual/)).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Ações de Fundo X" }));
    expect(await screen.findByRole("menuitem", { name: "Editar" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Excluir" })).toBeInTheDocument();
  });

  it("keeps the provider holding of an integrated account read-only", async () => {
    renderAccountCard();
    await userEvent.click(screen.getByRole("button", { name: "Ações de LCA Nubank" }));
    expect(await screen.findByRole("menuitem", { name: "Ver detalhes" })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Editar" })).toBeNull();
    expect(screen.queryByRole("menuitem", { name: "Excluir" })).toBeNull();
  });

  it("gives the goal the same figures but no cash balance, which a goal does not hold", () => {
    render(
      <MemoryRouter>
        <InvestmentGoalCard
          portfolio={goal}
          summary={undefined}
          positions={[syncedPosition, manualPosition]}
          operations={[fee]}
          portfolios={[goal]}
          linked={linked}
          accountNameOf={() => "Nubank"}
          onEdit={vi.fn()}
          onRemove={vi.fn()}
          onAssignGoal={vi.fn()}
          busy={false}
        />
      </MemoryRouter>,
    );
    const section = screen.getByRole("region", { name: "Reserva" });
    expect(within(section).getByRole("button", { name: "Rendimento líquido" })).toBeInTheDocument();
    expect(within(section).getByRole("button", { name: "IR/Taxas" })).toBeInTheDocument();
    expect(within(section).queryByText(/Saldo para investir|Caixa disponível/)).toBeNull();
  });

  it("puts the rendimento and the IR/Taxas beside the workspace total", () => {
    render(
      <InvestmentWorkspaceSummary
        summary={{ total_value: "2320.50", cash_balance: "0.00", synced_value: "1120.50" } as unknown as InvestmentSummary}
        positions={[syncedPosition, manualPosition]}
        operations={[fee]}
        linked={linked}
      />,
    );
    expect(screen.getByText("Rendimento líquido")).toBeVisible();
    expect(screen.getByText("+R$ 298,50")).toBeVisible();
    expect(screen.getByText("IR/Taxas")).toBeVisible();
  });
});
