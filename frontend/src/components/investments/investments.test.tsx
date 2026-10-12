import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { InvestmentPosition } from "../../api/contracts";
import { integratedAccount } from "../../test/investmentWorkspaceFixtures";
import * as compactScreen from "../shared/useCompactScreen";
import { InvestmentFigures } from "./InvestmentFigures";
import { InvestmentOperationFields } from "./InvestmentOperationForm";
import { InvestmentPositionsTable } from "./InvestmentPositionsTable";
import { fromMoneyInput, toMoneyInput } from "./moneyDraft";

const desktopMatchMedia = window.matchMedia;
afterEach(() => {
  vi.restoreAllMocks();
  window.matchMedia = desktopMatchMedia;
});

/** A phone: no min-width query matches (the table stacks below `lg`, which reads the real breakpoints). */
function pretendPhone() {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => undefined,
    removeListener: () => undefined,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    dispatchEvent: () => false,
  })) as typeof window.matchMedia;
}

const position: InvestmentPosition = {
  id: "p1",
  source: "manual",
  account_id: "a1",
  asset_id: null,
  portfolio_id: null,
  name: "Tesouro Selic 2029",
  ticker: null,
  asset_type: "Tesouro",
  quantity: "3.2",
  average_cost: "10000.00",
  current_value: "44180.21",
  current_unit_price: null,
  valued_on: "2026-09-30",
  currency_code: "BRL",
  closed: false,
  linked_investment_id: null,
  notes: null,
  valuation_basis: "manual_valuation",
};

function renderTable(compact: boolean, onDelete = vi.fn()) {
  vi.spyOn(compactScreen, "useCompactScreen").mockReturnValue(compact);
  if (compact) pretendPhone();
  render(
    <MemoryRouter>
      <InvestmentPositionsTable
        positions={[position]}
        portfolios={[]}
        costs={{}}
        linked={new Map()}
        onAssignGoal={vi.fn()}
        onEdit={vi.fn()}
        onDelete={onDelete}
        busy={false}
        emptyTitle="Nenhuma posição"
      />
    </MemoryRouter>,
  );
  return onDelete;
}

describe("InvestmentOperationFields", () => {
  it("offers trades for the manual holdings of an integrated account, but no cash opening balance", async () => {
    render(
      <InvestmentOperationFields
        formId="operation"
        operation={null}
        accounts={[integratedAccount]}
        positions={[]}
        submitError={null}
        onSubmit={vi.fn()}
      />,
    );
    fireEvent.mouseDown(screen.getByRole("combobox", { name: "Tipo" }));
    const option = (label: string) => screen.queryByText(label, { selector: ".ant-select-item-option-content" });
    expect(await screen.findByText("Compra", { selector: ".ant-select-item-option-content" })).toBeInTheDocument();
    expect(option("Venda")).toBeInTheDocument();
    expect(option("Cotação manual")).toBeInTheDocument();
    expect(option("Saldo inicial")).toBeNull();
  });
});

describe("InvestmentPositionsTable", () => {
  it("is a stack of rows with a record menu on a phone, with no table to scroll sideways", async () => {
    renderTable(true);
    const list = screen.getByRole("list", { name: "Posições" });
    expect(within(list).getByText("Tesouro Selic 2029")).toBeVisible();
    expect(screen.queryByRole("table")).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Ações de Tesouro Selic 2029" }));
    expect(await screen.findByText("Editar")).toBeInTheDocument();
    // Deleting a record says "Excluir"; "Remover" is only for unlinking.
    expect(screen.getByText("Excluir")).toBeInTheDocument();
    expect(screen.queryByText("Remover")).toBeNull();
  });

  it("is a flat table from lg up", () => {
    renderTable(false);
    expect(screen.getByRole("table", { name: "Posições" })).toBeVisible();
  });

  it("shows the goal as plain text and changes it from the record menu, not from a select on every row", async () => {
    vi.spyOn(compactScreen, "useCompactScreen").mockReturnValue(false);
    const onAssignGoal = vi.fn();
    render(
      <MemoryRouter>
        <InvestmentPositionsTable
          positions={[{ ...position, portfolio_id: "g1" }]}
          portfolios={[{ id: "g1", name: "Aposentadoria" } as never, { id: "g2", name: "Viagem" } as never]}
          costs={{}}
          linked={new Map()}
          onAssignGoal={onAssignGoal}
          busy={false}
          emptyTitle="Nenhuma posição"
        />
      </MemoryRouter>,
    );
    expect(screen.getByText("Aposentadoria")).toBeVisible();
    expect(screen.queryByRole("combobox")).toBeNull();
    // Quantities are written the pt-BR way.
    expect(screen.getByText("3,2")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Ações de Tesouro Selic 2029" }));
    expect(await screen.findByText("Mudar objetivo")).toBeInTheDocument();
  });

  it("names a value derived from the market quote and states its gain like any valued position", () => {
    vi.spyOn(compactScreen, "useCompactScreen").mockReturnValue(false);
    render(
      <MemoryRouter>
        <InvestmentPositionsTable
          positions={[{ ...position, valuation_basis: "market_quote", current_unit_price: "13806.32" }]}
          portfolios={[]}
          costs={{}}
          linked={new Map()}
          onAssignGoal={vi.fn()}
          busy={false}
          emptyTitle="Nenhuma posição"
        />
      </MemoryRouter>,
    );
    expect(screen.getByText(/Cotação de mercado/)).toBeVisible();
    // 44.180,21 over 3,2 × 10.000,00 of cost: a valued position states its gain
    // instead of the cost-basis explanation.
    expect(screen.getByText("+38,1%")).toBeVisible();
    expect(screen.queryByText(/Sem cotação registrada/)).toBeNull();
  });
});

describe("InvestmentFigures", () => {
  it("opens a figure's explanation on tap, not only on hover", async () => {
    render(<InvestmentFigures figures={[{ label: "Aportes", value: "R$ 1,00", hint: "Dinheiro que entrou." }]} />);
    await userEvent.click(screen.getByRole("button", { name: "Aportes" }));
    expect(await screen.findByText("Dinheiro que entrou.")).toBeInTheDocument();
  });
});

describe("moneyDraft", () => {
  it("round-trips a decimal string through the money field without drifting", () => {
    expect(toMoneyInput("1234.50")).toBe(1234.5);
    expect(fromMoneyInput(1234.5)).toBe("1234.50");
    expect(toMoneyInput(null)).toBeNull();
    expect(fromMoneyInput(null)).toBeNull();
  });
});
