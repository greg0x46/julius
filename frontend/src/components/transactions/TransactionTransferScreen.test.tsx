import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import * as transfers from "../../api/transfers";
import * as transactions from "../../api/transactions";
import type { TransactionItem } from "../../api/contracts";
import { QueryTestProvider } from "../../test/QueryTestProvider";
import { transactionResult } from "../../test/transactionFixtures";
import { TransactionTransferScreen } from "./TransactionTransferScreen";

vi.mock("../../api/transfers");
vi.mock("../../api/transactions");
const source: TransactionItem = { ...transactionResult.items[0], description: "Envio BRL", amount_in_account_currency: "-540" };
const destination: TransactionItem = { ...source, id: "66666666-6666-4666-8666-666666666666", description: "Recebimento USD", amount_in_account_currency: "100", classification: "inflow", account: { ...source.account, id: "77777777-7777-4777-8777-777777777777", name: "Conta USD", currency_code: "USD" } };
const linked: transfers.AccountTransfer = { id: "88888888-8888-4888-8888-888888888888", source: { transaction_id: source.id, account_id: source.account.id, account_name: "Conta BRL", currency_code: "BRL", amount: "-540" }, destination: { transaction_id: destination.id, account_id: destination.account.id, account_name: "Conta USD", currency_code: "USD", amount: "100" }, effective_rate: "5.4", fee: null };
function show(item = source) { render(<QueryTestProvider><TransactionTransferScreen item={item} /></QueryTestProvider>); }

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(transfers.getTransactionTransfer).mockResolvedValue(null);
  vi.mocked(transactions.queryTransactions).mockImplementation(async (query) => ({ ...transactionResult, items: query.filters.classification === "inflow" ? [destination] : [source] }));
});
describe("TransactionTransferScreen", () => {
  it("pairs existing native movements and displays the effective rate without inferring fees", async () => {
    const user = userEvent.setup();
    let saved = false;
    vi.mocked(transfers.getTransactionTransfer).mockImplementation(async () => saved ? linked : null);
    vi.mocked(transfers.createTransfer).mockImplementation(async () => { saved = true; return linked; });
    show();
    await user.click(await screen.findByRole("combobox", { name: "Entrada na conta de destino" }));
    await user.click(await screen.findByText("Recebimento USD"));
    expect(await screen.findByText("1 USD = 5,4 BRL")).toBeVisible();
    expect(screen.getByText(/Conta USD · USD\s100,00/)).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Vincular transferência" }));
    await waitFor(() => expect(transfers.createTransfer).toHaveBeenCalledWith({ source_transaction_id: source.id, destination_transaction_id: destination.id }));
    expect(await screen.findByRole("button", { name: "Desvincular transferência" })).toBeVisible();
    expect(screen.getByText("Nenhuma")).toBeVisible();
  });
  it("shows a read failure and retry instead of presenting an unlinked form", async () => {
    vi.mocked(transfers.getTransactionTransfer).mockRejectedValue(new Error("offline"));
    show();
    expect(await screen.findByText("Não foi possível consultar a transferência")).toBeVisible();
    expect(screen.queryByRole("button", { name: "Vincular transferência" })).toBeNull();
    expect(screen.getByRole("button", { name: "Tentar novamente" })).toBeVisible();
  });
  it("shows unavailable corrected amounts and still allows unlinking", async () => {
    const user = userEvent.setup();
    let removed = false;
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const corrected = { ...linked, destination: { ...linked.destination, amount: null }, effective_rate: "" };
    vi.mocked(transfers.getTransactionTransfer).mockImplementation(async () => removed ? null : corrected);
    vi.mocked(transfers.deleteTransfer).mockImplementation(async () => { removed = true; });
    show(source);
    expect(await screen.findByText("Conta USD · Valor indisponível")).toBeVisible();
    expect(screen.getByText("Indisponível após alteração dos valores")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Desvincular transferência" }));
    await waitFor(() => expect(transfers.deleteTransfer).toHaveBeenCalledWith(linked.id));
    expect(transactions.deleteManualTransaction).not.toHaveBeenCalled();
  });
  it("unlinks without removing either transaction", async () => {
    const user = userEvent.setup();
    let removed = false;
    vi.spyOn(window, "confirm").mockReturnValue(true);
    vi.mocked(transfers.getTransactionTransfer).mockImplementation(async () => removed ? null : linked);
    vi.mocked(transfers.deleteTransfer).mockImplementation(async () => { removed = true; });
    show(destination);
    await user.click(await screen.findByRole("button", { name: "Desvincular transferência" }));
    await waitFor(() => expect(transfers.deleteTransfer).toHaveBeenCalledWith(linked.id));
    expect(await screen.findByRole("button", { name: "Vincular transferência" })).toBeDisabled();
    expect(transactions.deleteManualTransaction).not.toHaveBeenCalled();
  });
});
