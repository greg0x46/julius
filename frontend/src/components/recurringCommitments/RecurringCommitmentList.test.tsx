import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { App } from "../ui";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { RecurringCommitment } from "../../api/contracts";
import * as compactScreen from "../shared/useCompactScreen";
import { RecurringCommitmentList } from "./RecurringCommitmentList";

vi.mock("../../hooks/useRecurrenceOccurrences", () => ({
  useRecurrenceOccurrences: () => ({
    occurrences: [],
    isLoading: false,
    error: null,
    retry: () => undefined,
    reconcile: () => Promise.resolve(),
    detach: () => Promise.resolve(),
    restoreAutomatic: () => Promise.resolve(),
  }),
  useReconciliationCandidates: () => ({ search: "", setSearch: () => undefined, candidates: [], isSearching: false }),
}));

const originalMatchMedia = window.matchMedia;

afterEach(() => {
  vi.restoreAllMocks();
  window.matchMedia = originalMatchMedia;
});

const rent: RecurringCommitment = {
  id: "c1",
  name: "Aluguel",
  kind: "expense",
  amount: "1800.00",
  category_id: "cat1",
  account_id: null,
  cadence: "monthly",
  day_of_month: 5,
  month_of_year: null,
  start_date: "2026-01-01",
  end_date: null,
  is_active: true,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

function renderList({ compact = false, onEdit = vi.fn(), onDelete = vi.fn() } = {}) {
  vi.spyOn(compactScreen, "useCompactScreen").mockReturnValue(compact);
  if (compact) {
    // Below `lg` the list stacks: no breakpoint matches.
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
  render(
    <App>
      <RecurringCommitmentList
        commitments={[rent]}
        categories={[]}
        isLoading={false}
        togglingCommitmentId={null}
        empty={<p>vazio</p>}
        onEdit={onEdit}
        onToggle={vi.fn()}
        onDelete={onDelete}
      />
    </App>,
  );
  return { onEdit, onDelete };
}

describe("RecurringCommitmentList", () => {
  it("opens the editor on a click on the row, with no coloured Editar link", async () => {
    const { onEdit } = renderList();

    expect(screen.queryByRole("button", { name: "Editar" })).toBeNull();
    await userEvent.click(screen.getByText("Aluguel"));
    expect(onEdit).toHaveBeenCalledWith(rent);
  });

  it("keeps Editar and Excluir behind one menu, which does not also trigger the row", async () => {
    const { onEdit } = renderList();

    await userEvent.click(screen.getByRole("button", { name: "Ações de Aluguel" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Editar" }));
    expect(onEdit).toHaveBeenCalledTimes(1);
  });

  it("does not open the editor when the switch is used", async () => {
    const { onEdit } = renderList();

    await userEvent.click(screen.getByRole("switch", { name: "Pausar recorrência Aluguel" }));
    expect(onEdit).not.toHaveBeenCalled();
  });

  it("stacks into tappable rows below the table's breakpoint", async () => {
    const { onEdit } = renderList({ compact: true });

    expect(screen.queryByRole("table")).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Abrir Aluguel" }));
    expect(onEdit).toHaveBeenCalledWith(rent);
  });
});
