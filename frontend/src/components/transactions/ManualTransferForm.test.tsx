import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import * as accounts from "../../api/accounts";
import * as transfers from "../../api/transfers";
import { QueryTestProvider } from "../../test/QueryTestProvider";
import { bankAccount } from "../../test/accountFixtures";
import { ManualTransferForm } from "./ManualTransferForm";

vi.mock("../../api/accounts");
vi.mock("../../api/transfers");
const source = { ...bankAccount, origin: "manual" as const, name: "Conta BRL" };
const destination = { ...source, id: "77777777-7777-4777-8777-777777777777", name: "Conta USD", currency_code: "USD" };
beforeEach(() => vi.clearAllMocks());
describe("ManualTransferForm", () => {
  it("creates two exact native movements and a separate explicit fee", async () => {
    const user = userEvent.setup();
    const done = vi.fn();
    vi.mocked(accounts.listAccounts).mockResolvedValue([source, destination]);
    vi.mocked(transfers.createTransfer).mockResolvedValue({} as transfers.AccountTransfer);
    render(<QueryTestProvider><ManualTransferForm onClose={vi.fn()} onCreated={done} /></QueryTestProvider>);
    await user.click(await screen.findByRole("combobox", { name: "Conta de origem" }));
    await user.click(await screen.findByText("Conta BRL · BRL"));
    await user.click(screen.getByRole("spinbutton", { name: "Valor enviado" }));
    await user.paste("9007199254740993.01");
    await user.click(screen.getByRole("combobox", { name: "Conta de destino" }));
    await user.click((await screen.findAllByText("Conta USD · USD")).find((element) => !element.closest(".ant-select-dropdown-hidden"))!);
    await user.type(screen.getByRole("spinbutton", { name: "Valor recebido" }), "100");
    await user.type(screen.getByRole("spinbutton", { name: "Tarifa registrada (opcional)" }), "10");
    await user.click(screen.getByRole("button", { name: "Criar transferência" }));
    await waitFor(() => expect(transfers.createTransfer).toHaveBeenCalledWith({
      source: expect.objectContaining({ account_id: source.id, amount: "-9007199254740993.01" }),
      destination: expect.objectContaining({ account_id: destination.id, amount: "100" }),
      fee: expect.objectContaining({ account_id: source.id, amount: "-10", description: "Tarifa de câmbio" }),
    }));
    await waitFor(() => expect(done).toHaveBeenCalledOnce());
  });
  it("keeps whole digits when backspacing a grouped amount", async () => {
    const user = userEvent.setup();
    vi.mocked(accounts.listAccounts).mockResolvedValue([source, destination]);
    render(<QueryTestProvider><ManualTransferForm onClose={vi.fn()} onCreated={vi.fn()} /></QueryTestProvider>);
    for (const name of ["Valor enviado", "Valor recebido", "Tarifa registrada (opcional)"]) {
      const input = await screen.findByRole("spinbutton", { name });
      await user.type(input, "1234");
      expect(input).toHaveValue("1.234");
      await user.type(input, "{Backspace}");
      expect(input).toHaveValue("123");
    }
  });
  it("explains the prerequisite when fewer than two manual accounts exist", async () => {
    vi.mocked(accounts.listAccounts).mockResolvedValue([source, bankAccount]);
    render(<QueryTestProvider><ManualTransferForm onClose={vi.fn()} onCreated={vi.fn()} /></QueryTestProvider>);
    expect(await screen.findByText("Crie duas contas manuais para registrar uma transferência.")).toBeVisible();
    expect(screen.getByRole("button", { name: "Criar transferência" })).toBeDisabled();
  });
});
