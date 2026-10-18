import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

import { PayableTimeline } from "./PayableTimeline";

const generateInstallments = vi.hoisted(() => vi.fn());

vi.mock("../../hooks/usePayablePlan", () => ({
  usePayablePlan: () => ({
    plan: { id: "plan", transactions: [] },
    isLoading: false,
    error: null,
    isCreating: false,
    isGenerating: false,
    generateInstallments,
  }),
}));

describe("PayableTimeline plan generation", () => {
  it("never sends a fractional installment count", async () => {
    generateInstallments.mockResolvedValue(undefined);
    render(
      <MemoryRouter>
        <PayableTimeline
          payableId="44444444-4444-4444-8444-444444444444"
          kind="debt"
          links={[]}
          search=""
          onSearchChange={vi.fn()}
          candidates={[]}
          isSearching={false}
          isLinking={false}
          onLinkTransaction={vi.fn()}
          onUnlinkTransaction={vi.fn()}
        />
      </MemoryRouter>,
    );
    const input = screen.getByRole("spinbutton", { name: "Número de parcelas" });
    fireEvent.change(input, { target: { value: "6.5" } });
    fireEvent.blur(input);
    await userEvent.click(screen.getByRole("button", { name: "Gerar parcelas" }));
    await waitFor(() => expect(generateInstallments).toHaveBeenCalledOnce());
    expect(Number.isInteger(generateInstallments.mock.calls[0]![0].months)).toBe(true);
  });
});
