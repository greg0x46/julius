import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import * as compactScreen from "../components/shared/useCompactScreen";
import { describe, expect, it, vi } from "vitest";

import * as accountsApi from "../api/accounts";
import { ApiError } from "../api/problems";
import { bankAccount, creditAccount } from "../test/accountFixtures";
import { QueryTestProvider } from "../test/QueryTestProvider";
import { AccountsPage } from "./AccountsPage";

vi.mock("../api/accounts");

function renderPage() {
  return render(
    <MemoryRouter>
      <QueryTestProvider>
        <AccountsPage />
      </QueryTestProvider>
    </MemoryRouter>,
  );
}

// The headings render before the query resolves, so waiting on one doesn't
// mean the rows are in yet.
const findRow = (text: string) => screen.findByText(text, {}, { timeout: 5000 });

describe("AccountsPage", () => {
  it.each([
    ["on a phone (title row)", true],
    ["on a wide screen (title row)", false],
  ])("offers the statement import %s", async (_label, compact) => {
    vi.spyOn(compactScreen, "useCompactScreen").mockReturnValue(compact);
    vi.mocked(accountsApi.listAccounts).mockResolvedValue([bankAccount]);
    renderPage();

    expect(await findRow("Conta Corrente")).toBeVisible();
    expect(screen.getAllByRole("button", { name: /Importar extrato/ })).toHaveLength(1);
    vi.restoreAllMocks();
  });

  it.each([true, false])("shows native manual balances on mobile=%s", async (compact) => {
    vi.spyOn(compactScreen, "useCompactScreen").mockReturnValue(compact);
    vi.mocked(accountsApi.listAccounts).mockResolvedValue([{ ...bankAccount, origin: "manual", external_id: null, currency_code: "USD", balance: "110", opening_balance: "100", institution_name: null }]);
    renderPage();
    expect(await findRow("USD\u00a0110,00")).toBeVisible();
    expect(screen.getByText("Conta manual · USD")).toBeVisible();
    expect(screen.getByRole("button", { name: "Nova conta" })).toBeVisible();
    expect(screen.getByText("Soma só as contas em reais.")).toBeVisible();
    vi.restoreAllMocks();
  });

  it("splits bank accounts and credit cards into their own sections", async () => {
    vi.mocked(accountsApi.listAccounts).mockResolvedValue([bankAccount, creditAccount]);
    renderPage();

    expect(await findRow("Conta Corrente")).toBeVisible();
    expect(screen.getByRole("heading", { name: "Contas bancárias" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "Cartões de crédito" })).toBeVisible();
    expect(screen.getByText("Cartão Platinum")).toBeVisible();
  });

  it("shows each account's balance and the card's limit usage", async () => {
    vi.mocked(accountsApi.listAccounts).mockResolvedValue([bankAccount, creditAccount]);
    renderPage();

    await findRow("R$ 3.765,44 disponível · 24,7% usado");
    // Each figure shows twice: once in its row, once in the summary.
    expect(screen.getAllByText("R$ 2.500,00").length).toBeGreaterThan(0);
    expect(screen.getAllByText("R$ 1.234,56").length).toBeGreaterThan(0);
    expect(screen.getByText("Saldo em contas")).toBeVisible();
    expect(screen.getByText("Fatura atual dos cartões")).toBeVisible();
    expect(screen.getByText("R$ 3.765,44 disponível · 24,7% usado")).toBeVisible();
  });

  it("shows no meta line, and never a lone dash, for an account with nothing to say", async () => {
    vi.mocked(accountsApi.listAccounts).mockResolvedValue([
      {
        ...bankAccount,
        name: "Conta de arquivo",
        number: null,
        account_subtype: null,
        institution_name: null,
      },
    ]);
    renderPage();

    expect(await findRow("Conta de arquivo")).toBeVisible();
    expect(screen.queryByText("—")).toBeNull();
  });

  it("says there are no accounts once, not once per section", async () => {
    vi.mocked(accountsApi.listAccounts).mockResolvedValue([]);
    renderPage();

    expect(await findRow("Nenhuma conta ainda")).toBeVisible();
    expect(screen.queryByRole("heading", { name: "Cartões de crédito" })).toBeNull();
    expect(screen.queryByText(/sincronizad/)).toBeNull();
  });

  it("treats an account with no type as a bank account", async () => {
    vi.mocked(accountsApi.listAccounts).mockResolvedValue([
      { ...bankAccount, account_type: null, name: "Conta sem tipo" },
    ]);
    renderPage();

    expect(await findRow("Conta sem tipo")).toBeVisible();
    expect(screen.getByText("Nenhum cartão de crédito")).toBeVisible();
  });

  it("offers a retry when the accounts cannot be loaded", async () => {
    const user = userEvent.setup();
    vi.mocked(accountsApi.listAccounts).mockRejectedValue(
      new ApiError("response", "Não foi possível carregar as contas."),
    );
    renderPage();

    await user.click(await screen.findByRole("button", { name: "Tentar novamente" }));
    await waitFor(() => expect(accountsApi.listAccounts).toHaveBeenCalledTimes(2));
  });

  it("says only that the load failed — no empty sections, no 'nothing yet' — when the accounts cannot be loaded", async () => {
    vi.mocked(accountsApi.listAccounts).mockRejectedValue(
      new ApiError("response", "Não foi possível carregar as contas."),
    );
    renderPage();

    expect(await screen.findByText("Não foi possível carregar as contas")).toBeVisible();
    expect(screen.queryByText("Nenhuma conta bancária")).toBeNull();
    expect(screen.queryByText("Nenhum cartão de crédito")).toBeNull();
    expect(screen.queryByText("Nenhuma conta ainda")).toBeNull();
    expect(screen.queryByRole("heading", { name: "Contas bancárias" })).toBeNull();
  });

  it("sums the card limit over the cards in reais only", async () => {
    vi.mocked(accountsApi.listAccounts).mockResolvedValue([
      creditAccount,
      { ...creditAccount, id: "66666666-6666-4666-8666-666666666666", name: "Cartão USD", credit_limit: "9000.00", currency_code: "USD" },
    ]);
    renderPage();

    expect(await screen.findByText("Limite total em reais R$ 5.000,00")).toBeVisible();
  });

  it("opens the card with the same name the row shows", async () => {
    vi.mocked(accountsApi.listAccounts).mockResolvedValue([
      { ...bankAccount, institution_name: "Nu Pagamentos S.A. - Instituição de Pagamento" },
    ]);
    renderPage();

    expect(await findRow("Conta Corrente")).toBeVisible();
    expect(screen.getByText("Conta corrente · •••• 3456 · Nu Pagamentos")).toBeVisible();
  });
});
