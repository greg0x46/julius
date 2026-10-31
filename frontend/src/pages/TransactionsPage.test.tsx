import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import * as accountsApi from "../api/accounts";
import * as categoriesApi from "../api/categories";
import * as transactionsApi from "../api/transactions";
import { QueryTestProvider } from "../test/QueryTestProvider";
import { accountId, transactionId, transactionResult } from "../test/transactionFixtures";
import { TransactionsPage } from "./TransactionsPage";

vi.mock("../api/accounts");
vi.mock("../api/transactions");
vi.mock("../api/categories");

const emptyResult = {
  ...transactionResult,
  items: [],
  groups: [],
  totals: [],
  page: { number: 1, size: 50, total_items: 0, total_pages: 1 },
};

function renderPage(search = "?period=all") {
  return render(
    <MemoryRouter initialEntries={[`/transacoes${search}`]}>
      <QueryTestProvider>
        <TransactionsPage />
      </QueryTestProvider>
    </MemoryRouter>,
  );
}

describe("TransactionsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(accountsApi.listAccounts).mockResolvedValue([]);
    vi.mocked(categoriesApi.listCategories).mockResolvedValue([]);
    vi.mocked(transactionsApi.getTransactionReconciliation).mockResolvedValue({
      current: null,
      options: [],
    } as never);
  });

  it("keeps the panel open when a write moves the line out of the current results", async () => {
    const user = userEvent.setup();
    // Once the line is ignored the server no longer returns it for this
    // view (as it would under a filter), so the refreshed list is empty.
    let written = false;
    vi.mocked(transactionsApi.queryTransactions).mockImplementation(async () =>
      written ? emptyResult : transactionResult,
    );
    vi.mocked(transactionsApi.setTransactionInclusion).mockImplementation(async () => {
      written = true;
      return { transaction_id: transactionId, state: "ignored", changed_at: "2026-07-30T13:00:00Z" };
    });

    renderPage();
    await user.click(await screen.findByRole("button", { name: /Ver detalhes de Mercado/ }));
    const panel = within(await screen.findByRole("dialog"));
    const toggle = await panel.findByRole("switch", { name: "Considerar nos totais" });
    expect(toggle).toBeChecked();

    await user.click(toggle);

    // The list refetched and no longer has the line…
    await waitFor(() => expect(screen.queryByRole("button", { name: /Ver detalhes de Mercado/ })).toBeNull());
    // …but the panel the person is working in is still there, showing the new decision.
    expect(panel.getByRole("heading", { name: "Transação" })).toBeVisible();
    expect(await panel.findByRole("switch", { name: "Considerar nos totais" })).not.toBeChecked();
  });

  it("keeps showing the saved decision while the refetch after the write is still in flight", async () => {
    const user = userEvent.setup();
    let written = false;
    let releaseRefetch: () => void = () => {};
    vi.mocked(transactionsApi.queryTransactions).mockImplementation(async () => {
      if (!written) return transactionResult;
      await new Promise<void>((resolve) => {
        releaseRefetch = resolve;
      });
      return emptyResult;
    });
    vi.mocked(transactionsApi.setTransactionInclusion).mockImplementation(async () => {
      written = true;
      return { transaction_id: transactionId, state: "ignored", changed_at: "2026-07-30T13:00:00Z" };
    });

    renderPage();
    await user.click(await screen.findByRole("button", { name: /Ver detalhes de Mercado/ }));
    const panel = within(await screen.findByRole("dialog"));
    await user.click(await panel.findByRole("switch", { name: "Considerar nos totais" }));

    // The write is confirmed but the list still holds the old copy of the line.
    await waitFor(() => expect(transactionsApi.queryTransactions).toHaveBeenCalledTimes(2));
    expect(screen.getByRole("button", { name: /Ver detalhes de Mercado/ })).toBeInTheDocument();
    expect(panel.getByRole("switch", { name: "Considerar nos totais" })).not.toBeChecked();

    releaseRefetch();
    await waitFor(() => expect(screen.queryByRole("button", { name: /Ver detalhes de Mercado/ })).toBeNull());
    expect(panel.getByRole("switch", { name: "Considerar nos totais" })).not.toBeChecked();
  });

  it("shows a failed write inside the panel, next to the control, with a retry", async () => {
    const user = userEvent.setup();
    vi.mocked(transactionsApi.queryTransactions).mockResolvedValue(transactionResult);
    vi.mocked(transactionsApi.setTransactionInclusion).mockRejectedValue(new Error("Servidor indisponível."));

    renderPage();
    await user.click(await screen.findByRole("button", { name: /Ver detalhes de Mercado/ }));
    const panel = within(await screen.findByRole("dialog"));
    await user.click(await panel.findByRole("switch", { name: "Considerar nos totais" }));

    expect(await panel.findByText("Não foi possível salvar a decisão")).toBeVisible();
    expect(panel.getByText("Servidor indisponível.")).toBeVisible();
    expect(panel.getByRole("button", { name: "Tentar novamente" })).toBeVisible();
  });
  it("says the recurrence lookup failed instead of 'no recurrence nearby', and offers a retry", async () => {
    const user = userEvent.setup();
    vi.mocked(transactionsApi.queryTransactions).mockResolvedValue(transactionResult);
    vi.mocked(transactionsApi.getTransactionReconciliation).mockRejectedValue(new Error("falha"));

    renderPage();
    await user.click(await screen.findByRole("button", { name: /Ver detalhes de Mercado/ }));
    const panel = within(await screen.findByRole("dialog"));
    const row = await panel.findByRole("button", { name: /Conciliar com recorrência/ });
    await waitFor(() => expect(row).toHaveTextContent("Não foi possível consultar"));
    expect(row).not.toHaveTextContent("Nenhuma recorrência por perto");

    await user.click(row);
    expect(await panel.findByText("Não foi possível consultar as recorrências")).toBeVisible();
    expect(panel.getByRole("button", { name: "Tentar novamente" })).toBeVisible();
  });

  it("marks and focuses the first invalid field of the new-transaction form", async () => {
    const user = userEvent.setup();
    vi.mocked(transactionsApi.queryTransactions).mockResolvedValue(transactionResult);
    vi.mocked(accountsApi.listAccounts).mockResolvedValue([
      { id: accountId, name: "Conta corrente", institution_name: "Banco Teste", number: null } as never,
    ]);

    renderPage();
    await screen.findByRole("button", { name: /Ver detalhes de Mercado/ });
    await user.click(screen.getAllByRole("button", { name: /Nova/ })[0]!);
    const form = within(await screen.findByRole("dialog"));
    await user.click(form.getByRole("button", { name: "Salvar" }));

    await screen.findByText("Informe uma descrição.");
    const description = document.getElementById("manual-transaction-description")!;
    expect(description).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByText("Informe uma descrição.")).toBeVisible();
    await waitFor(() => expect(description).toHaveFocus());
  });

  it("falls back to the last page when the URL asks for a page past the end", async () => {
    const lastPage = { number: 2, size: 50, total_items: 60, total_pages: 2 };
    vi.mocked(transactionsApi.queryTransactions).mockImplementation(async (query) =>
      query.page > lastPage.total_pages
        ? { ...emptyResult, page: { ...lastPage, number: query.page } }
        : { ...transactionResult, page: { ...lastPage, number: query.page } },
    );

    renderPage("?period=all&page=9");

    await screen.findByRole("button", { name: /Ver detalhes de Mercado/ });
    expect(transactionsApi.queryTransactions).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 2 }),
      expect.anything(),
    );
    expect(screen.getByText("Página 2 de 2")).toBeVisible();
  });
});

it("translates transaction filters, rows and the manual form when the language changes", async () => {
  const { I18nProvider } = await import("../i18n/I18nProvider");
  const { useTranslation } = await import("../i18n/context");
  function LanguageSwitch() {
    const { setLanguage } = useTranslation();
    return <button onClick={() => setLanguage("en")}>English</button>;
  }
  vi.mocked(accountsApi.listAccounts).mockResolvedValue([]);
  vi.mocked(categoriesApi.listCategories).mockResolvedValue([]);
  vi.mocked(transactionsApi.queryTransactions).mockResolvedValue(transactionResult);
  localStorage.removeItem("julius.language");
  render(<I18nProvider><MemoryRouter initialEntries={["/transacoes?period=all"]}><QueryTestProvider><LanguageSwitch /><TransactionsPage /></QueryTestProvider></MemoryRouter></I18nProvider>);
  await screen.findByRole("button", { name: /Ver detalhes de Mercado/ });
  await userEvent.click(screen.getByRole("button", { name: "English" }));
  expect(screen.getByRole("heading", { name: "Transactions" })).toBeVisible();
  expect(screen.getByRole("button", { name: /View details of Mercado/ })).toBeVisible();
  expect(screen.getByRole("button", { name: "Filters" })).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: /View details of Mercado/ }));
  const panel = within(await screen.findByRole("dialog"));
  expect(panel.getByRole("heading", { name: "Transaction" })).toBeVisible();
  expect(panel.getByRole("switch", { name: "Include in totals" })).toBeVisible();
  await userEvent.click(panel.getByRole("button", { name: "Close" }));
  await userEvent.click(screen.getByRole("button", { name: "New transaction" }));
  const dialog = within(await screen.findByRole("dialog"));
  expect(dialog.getByLabelText("Description")).toBeVisible();
  await userEvent.click(dialog.getByRole("button", { name: "Save" }));
  expect(dialog.getByText("Select an account.")).toBeVisible();
  localStorage.removeItem("julius.language");
});
