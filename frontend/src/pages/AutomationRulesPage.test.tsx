import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { RecurringCommitmentWrite } from "../api/contracts";
import type { AutomationEntrySubmitPayload } from "../components/automationRules/AutomationEntryForm";
import { AutomationRulesPage } from "./AutomationRulesPage";

const commitmentWrite: RecurringCommitmentWrite = {
  name: "Aluguel", kind: "expense", amount: "1500", category_id: "cat", account_id: null,
  cadence: "monthly", day_of_month: 5, month_of_year: null, start_date: "2026-01-01", end_date: null, is_active: true,
};
const payload: AutomationEntrySubmitPayload = {
  name: "Conciliar aluguel", isActive: true, logicOperator: "and", conditions: [],
  actionTypes: ["reconcile"], applyRetroactively: false, categoryId: null,
  reconcile: { commitmentMode: "new", existingCommitmentId: null, commitment: commitmentWrite },
};

const rules = vi.hoisted(() => ({ createRule: vi.fn(), updateRule: vi.fn() }));
const commitments = vi.hoisted(() => ({ createCommitment: vi.fn(), updateCommitment: vi.fn() }));

vi.mock("../hooks/useAutomationRules", () => ({
  useAutomationRules: () => ({
    rules: [], conditionOptions: { accounts: [], cards: [] }, areConditionOptionsLoading: false,
    isLoading: false, error: null, refetch: vi.fn(), toggleRule: vi.fn(), deleteRule: vi.fn(), ...rules,
  }),
}));
vi.mock("../hooks/useRecurringCommitments", () => ({
  useRecurringCommitments: () => ({ commitments: [], isLoading: false, error: null, refetch: vi.fn(), ...commitments }),
}));
vi.mock("../hooks/useCategories", () => ({ useCategories: () => ({ categories: [] }) }));
vi.mock("../components/shared/useFeedback", () => ({ useFeedback: () => ({ success: vi.fn(), error: vi.fn() }) }));
vi.mock("../components/automationRules/AutomationEntryList", () => ({ AutomationEntryList: () => null }));
vi.mock("../components/automationRules/AutomationEntryForm", () => ({
  AutomationEntryForm: ({ open, submitError, onSubmit }: { open: boolean; submitError: string | null; onSubmit: (p: unknown) => void }) =>
    open ? (
      <div>
        <button onClick={() => onSubmit(payload)}>Salvar</button>
        {submitError && <p>{submitError}</p>}
      </div>
    ) : null,
}));

describe("AutomationRulesPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    commitments.createCommitment.mockResolvedValue({ id: "commitment-1" });
    commitments.updateCommitment.mockResolvedValue({ id: "commitment-1" });
  });

  it("reuses the commitment it already created when the rule write fails and is retried", async () => {
    const user = userEvent.setup();
    rules.createRule
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce({ retroactive_apply: null });
    render(<MemoryRouter><AutomationRulesPage /></MemoryRouter>);

    await user.click(screen.getAllByRole("button", { name: /Nova/ })[0]!);
    await user.click(screen.getByRole("button", { name: "Salvar" }));
    expect(await screen.findByText("offline")).toBeVisible();

    await user.click(screen.getByRole("button", { name: "Salvar" }));
    await waitFor(() => expect(rules.createRule).toHaveBeenCalledTimes(2));

    expect(commitments.createCommitment).toHaveBeenCalledTimes(1);
    expect(commitments.updateCommitment).toHaveBeenCalledWith({ commitmentId: "commitment-1", write: commitmentWrite });
    expect(rules.createRule).toHaveBeenLastCalledWith(
      expect.objectContaining({ actions: [{ type: "reconcile", scenario_id: "commitment-1", category_id: null }] }),
    );
  });
});
