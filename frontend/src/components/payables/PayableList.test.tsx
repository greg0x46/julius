import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Payable } from "../../api/contracts";
import { settledPercent } from "../../presentation/payableLabels";
import * as compactScreen from "../shared/useCompactScreen";
import { PayableList } from "./PayableList";

const originalMatchMedia = window.matchMedia;

afterEach(() => {
  vi.restoreAllMocks();
  window.matchMedia = originalMatchMedia;
});

/** The list stacks below `lg` through the viewport hook, so a phone needs no breakpoint to match. */
function stubViewport(compact: boolean) {
  vi.spyOn(compactScreen, "useCompactScreen").mockReturnValue(compact);
  if (!compact) return;
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

function payable(overrides: Partial<Payable>): Payable {
  return {
    id: "p1",
    kind: "debt",
    name: "Empréstimo pessoal",
    total_amount: "1000.00",
    starting_settled_amount: "0.00",
    settled_amount: "420.00",
    remaining_amount: "580.00",
    status: "open",
    link_count: 0,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

const payables = [
  payable({}),
  payable({
    id: "p2",
    kind: "receivable",
    name: "Empréstimo para Ana",
    settled_amount: "1000.00",
    remaining_amount: "0.00",
    status: "settled",
  }),
];

function renderList(compact: boolean, onOpen = vi.fn(), showKind = true) {
  stubViewport(compact);
  render(
    <MemoryRouter>
      <PayableList payables={payables} isLoading={false} empty={<p>vazio</p>} showKind={showKind} onOpen={onOpen} />
    </MemoryRouter>,
  );
  return onOpen;
}

describe("PayableList", () => {
  it("shows name, remaining amount and paid share on a phone, with a tag only for a settled one", async () => {
    const onOpen = renderList(true);
    const rows = within(screen.getByRole("list", { name: "Pendências" })).getAllByRole("listitem");
    expect(rows).toHaveLength(2);
    expect(within(rows[0]).getByText("Empréstimo pessoal")).toBeVisible();
    expect(within(rows[0]).getByText("R$ 580,00")).toBeVisible();
    expect(within(rows[0]).getByText("Dívida · pago 42%")).toBeVisible();
    expect(within(rows[0]).queryByText("Aberta")).toBeNull();
    expect(within(rows[1]).getByText("A receber · recebido 100%")).toBeVisible();
    expect(within(rows[1]).getByText("Recebida")).toBeVisible();

    expect(screen.queryByRole("button", { name: "Editar" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Excluir" })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Abrir Empréstimo pessoal" }));
    expect(onOpen).toHaveBeenCalledWith(payables[0]);
  });

  it("is one table without per-row actions on a wide screen", () => {
    renderList(false);
    const table = screen.getByRole("table");
    expect(within(table).getByRole("link", { name: "Empréstimo pessoal" })).toHaveAttribute(
      "href",
      "/pendencias/p1?kind=debt",
    );
    expect(within(table).queryByText("Aberta")).toBeNull();
    expect(within(table).getByText("Recebida")).toBeVisible();
    expect(within(table).queryByText("Editar")).toBeNull();
  });

  it("drops the kind from rows once the tab already says it", () => {
    renderList(true, vi.fn(), false);
    expect(screen.getByText("pago 42%")).toBeVisible();
    expect(screen.queryByText(/Dívida · /)).toBeNull();
  });

  it("keeps the Tipo column only while the kinds are mixed", () => {
    renderList(false, vi.fn(), false);
    expect(screen.queryByRole("columnheader", { name: "Tipo" })).toBeNull();
  });

  it("reads 100% only when everything is settled", () => {
    expect(settledPercent("1000.00", "999.99")).toBe(99);
    expect(settledPercent("1000.00", "1000.00")).toBe(100);
    expect(settledPercent("0.00", "0.00")).toBe(0);
  });
});
