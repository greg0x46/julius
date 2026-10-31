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

  it("says why nothing was generated when the installment count is empty", async () => {
    generateInstallments.mockClear();
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
    await userEvent.clear(input);
    await userEvent.click(screen.getByRole("button", { name: "Gerar parcelas" }));
    expect(await screen.findByText("Informe o número de parcelas.")).toBeInTheDocument();
    expect(generateInstallments).not.toHaveBeenCalled();
  });
});

it("translates plan controls and validation when the language changes", async () => {
  const { I18nProvider } = await import("../../i18n/I18nProvider");
  const { useTranslation } = await import("../../i18n/context");
  function LanguageSwitch() {
    const { setLanguage } = useTranslation();
    return <button onClick={() => setLanguage("en")}>English</button>;
  }
  localStorage.removeItem("julius.language");
  render(<I18nProvider><MemoryRouter><LanguageSwitch /><PayableTimeline payableId="44444444-4444-4444-8444-444444444444" kind="debt" links={[]} search="" onSearchChange={vi.fn()} candidates={[]} isSearching={false} isLinking={false} onLinkTransaction={vi.fn()} onUnlinkTransaction={vi.fn()} /></MemoryRouter></I18nProvider>);
  await userEvent.click(screen.getByRole("button", { name: "English" }));
  expect(screen.getByRole("region", { name: "Timeline" })).toBeVisible();
  const input = screen.getByRole("spinbutton", { name: "Number of installments" });
  await userEvent.clear(input);
  await userEvent.click(screen.getByRole("button", { name: "Generate installments" }));
  expect(screen.getByText("Enter the number of installments.")).toBeVisible();
  localStorage.removeItem("julius.language");
});
