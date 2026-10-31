import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as payablesApi from "../api/payables";
import type { Payable } from "../api/contracts";
import { ApiError } from "../api/problems";
import { QueryTestProvider } from "../test/QueryTestProvider";
import { PayablesPage } from "./PayablesPage";

vi.mock("../api/payables");

afterEach(() => vi.restoreAllMocks());

function payable(overrides: Partial<Payable>): Payable {
  return {
    id: "p1",
    kind: "debt",
    name: "Empréstimo pessoal",
    total_amount: "1000.00",
    starting_settled_amount: "0.00",
    settled_amount: "400.00",
    remaining_amount: "600.00",
    status: "open",
    link_count: 0,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function renderPage() {
  return render(
    <MemoryRouter>
      <QueryTestProvider>
        <PayablesPage />
      </QueryTestProvider>
    </MemoryRouter>,
  );
}

describe("PayablesPage", () => {
  it("totals only what is still open", async () => {
    vi.mocked(payablesApi.listPayables).mockResolvedValue([
      payable({}),
      // A settled payable owes nothing, whatever remainder it still carries.
      payable({ id: "p2", name: "Quitada", status: "settled", settled_amount: "500.00", remaining_amount: "250.00" }),
      payable({ id: "p3", kind: "receivable", name: "Para Ana", remaining_amount: "80.00" }),
    ]);
    renderPage();

    const youOwe = (await screen.findByText("Você deve")).parentElement as HTMLElement;
    expect(within(youOwe).getByText("R$ 600,00")).toBeVisible();
    const owedToYou = screen.getByText("Te devem").parentElement as HTMLElement;
    expect(within(owedToYou).getByText("R$ 80,00")).toBeVisible();
  });

  it("says only that the load failed, never 'nothing yet' or zeroed totals, when the list cannot be loaded", async () => {
    vi.mocked(payablesApi.listPayables).mockRejectedValue(new ApiError("response", "falhou"));
    renderPage();

    expect(await screen.findByText("Não foi possível carregar as pendências")).toBeVisible();
    expect(screen.getByRole("button", { name: "Tentar novamente" })).toBeVisible();
    expect(screen.queryByText("Nenhuma pendência ainda")).toBeNull();
    expect(screen.queryByText("Você deve")).toBeNull();
  });

  it("does not repeat the title row's action under an empty list", async () => {
    vi.mocked(payablesApi.listPayables).mockResolvedValue([]);
    renderPage();

    expect(await screen.findByText("Nenhuma pendência ainda")).toBeVisible();
    // One "Nova pendência" (the title row's), not a second one in the empty state.
    expect(screen.getAllByRole("button", { name: /Nova pendência|Nova$/ })).toHaveLength(1);
  });
});

it("switches debt and receivable labels with the selected language", async () => {
  const { I18nProvider } = await import("../i18n/I18nProvider");
  const { useTranslation } = await import("../i18n/context");
  const userEvent = (await import("@testing-library/user-event")).default;
  function LanguageSwitch() {
    const { setLanguage } = useTranslation();
    return <button onClick={() => setLanguage("en")}>English</button>;
  }
  vi.mocked(payablesApi.listPayables).mockResolvedValue([payable({}), payable({ id: "p2", kind: "receivable", name: "Para Ana" })]);
  localStorage.removeItem("julius.language");
  render(<I18nProvider><MemoryRouter><QueryTestProvider><LanguageSwitch /><PayablesPage /></QueryTestProvider></MemoryRouter></I18nProvider>);
  await screen.findByText("Você deve");
  await userEvent.click(screen.getByRole("button", { name: "English" }));
  expect(screen.getByText("You owe")).toBeVisible();
  expect(screen.getByText("Owed to you")).toBeVisible();
  expect(screen.getByRole("radio", { name: "Debts" })).toBeVisible();
  localStorage.removeItem("julius.language");
});
