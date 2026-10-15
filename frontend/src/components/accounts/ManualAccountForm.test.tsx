import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import * as api from "../../api/accounts";
import { QueryTestProvider } from "../../test/QueryTestProvider";
import { bankAccount } from "../../test/accountFixtures";
import * as compactScreen from "../shared/useCompactScreen";
import { ManualAccountForm } from "./ManualAccountForm";
import { ManualAccountActions } from "./ManualAccountActions";

vi.mock("../../api/accounts");
const coverage = { status: "complete" as const, source: "frankfurter-v2-blended", currencies: [{ code: "BRL" }, { code: "EUR" }, { code: "USD", start_date: "1971-01-04", end_date: "2026-10-14" }, { code: "ARS", start_date: "1998-07-07", end_date: "2026-10-14" }] };
const manual = { ...bankAccount, origin: "manual" as const, external_id: null, name: "Conta USD", currency_code: "USD", opening_balance: "100", balance: "110", has_movements: true };

describe("manual bank accounts", () => {
  it.each([true, false])("creates an exact opening balance in a mobile=%s drawer", async (compact) => {
    vi.spyOn(compactScreen, "useCompactScreen").mockReturnValue(compact);
    vi.mocked(api.listAccountCurrencies).mockResolvedValue(coverage);
    vi.mocked(api.writeManualAccount).mockResolvedValue(manual);
    const saved = vi.fn();
    const user = userEvent.setup();
    render(<QueryTestProvider><ManualAccountForm onClose={vi.fn()} onSaved={saved} /></QueryTestProvider>);
    expect(screen.getByRole("dialog")).toBeVisible();
    await user.type(screen.getByLabelText("Nome"), "Conta USD");
    await user.click(screen.getByLabelText("Moeda"));
    await user.click(await screen.findByText(/USD —/));
    await user.clear(screen.getByLabelText("Saldo inicial"));
    await user.type(screen.getByLabelText("Saldo inicial"), "100,25");
    await user.click(screen.getByRole("button", { name: "Criar conta" }));
    await waitFor(() => expect(api.writeManualAccount).toHaveBeenCalledWith(null, { name: "Conta USD", currency_code: "USD", opening_balance: "100.25", archived: false }));
    await waitFor(() => expect(saved).toHaveBeenCalledWith(manual));
    vi.restoreAllMocks();
  });

  it("locks currency and opening balance after movements and retains a failed draft", async () => {
    vi.mocked(api.listAccountCurrencies).mockResolvedValue(coverage);
    vi.mocked(api.writeManualAccount).mockRejectedValue(new Error("Tente novamente"));
    const user = userEvent.setup();
    render(<QueryTestProvider><ManualAccountForm account={manual} onClose={vi.fn()} onSaved={vi.fn()} /></QueryTestProvider>);
    expect(screen.getByLabelText("Moeda")).toBeDisabled();
    expect(screen.getByLabelText("Saldo inicial")).toBeDisabled();
    expect(screen.getByText(/Esta conta já tem movimentações/)).toBeVisible();
    await waitFor(() => expect(screen.getByRole("button", { name: "Salvar" })).toBeEnabled());
    await user.click(screen.getByRole("button", { name: "Salvar" }));
    expect(await screen.findByText("Tente novamente")).toBeVisible();
    expect(screen.getByLabelText("Nome")).toHaveValue("Conta USD");
  });

  it("registers a signed native adjustment and explains its reporting effect", async () => {
    vi.mocked(api.adjustManualAccount).mockResolvedValue(manual);
    const user = userEvent.setup();
    render(<QueryTestProvider><ManualAccountForm account={manual} adjusting onClose={vi.fn()} onSaved={vi.fn()} /></QueryTestProvider>);
    expect(screen.getByText(/Este ajuste não é uma receita ou despesa/)).toBeVisible();
    await user.type(screen.getByLabelText("Valor do ajuste"), "-5,50");
    await user.type(screen.getByLabelText("Motivo do ajuste"), "Correção");
    await user.click(screen.getByRole("button", { name: "Registrar ajuste" }));
    await waitFor(() => expect(api.adjustManualAccount).toHaveBeenCalledWith(manual.id, "-5.50", "Correção"));
  });

  it("shows archive confirmation and blocks adjustments until reactivation", async () => {
    vi.mocked(api.writeManualAccount).mockResolvedValue(manual);
    const user = userEvent.setup();
    const view = render(<QueryTestProvider><ManualAccountActions account={manual} /></QueryTestProvider>);
    await user.click(screen.getByRole("button", { name: "Arquivar conta" }));
    await waitFor(() => expect(screen.getByText("Arquivar conta?")).toBeVisible());
    await user.click(screen.getByRole("button", { name: "Arquivar" }));
    await waitFor(() => expect(api.writeManualAccount).toHaveBeenCalledWith(manual.id, expect.objectContaining({ archived: true })));
    view.unmount();
    render(<QueryTestProvider><ManualAccountActions account={{ ...manual, archived_at: "2026-10-01T00:00:00Z" }} /></QueryTestProvider>);
    expect(screen.queryByRole("button", { name: "Ajustar saldo" })).toBeNull();
    expect(screen.getByText(/Conta arquivada/)).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Reativar conta" }));
    await waitFor(() => expect(api.writeManualAccount).toHaveBeenCalledWith(manual.id, expect.objectContaining({ archived: false })));
  });
  it("defaults to BRL and allows native-only accounts with an explicit warning", async () => {
    vi.mocked(api.listAccountCurrencies).mockResolvedValue(coverage);
    vi.mocked(api.writeManualAccount).mockResolvedValue({ ...manual, currency_code: "XTS" });
    const user = userEvent.setup();
    render(<QueryTestProvider><ManualAccountForm onClose={vi.fn()} onSaved={vi.fn()} /></QueryTestProvider>);
    expect(screen.getByLabelText("Saldo inicial").parentElement).toHaveTextContent("R$");
    await user.type(screen.getByLabelText("Nome"), "Moeda de teste");
    await user.click(screen.getByRole("checkbox", { name: "Usar outra moeda" }));
    await user.type(screen.getByLabelText("Código da moeda"), "xts");
    expect(await screen.findByText("Sem conversão para BRL para XTS.")).toBeVisible();
    expect(screen.getByText(/sincronizar não resolve/)).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Criar conta" }));
    await waitFor(() => expect(api.writeManualAccount).toHaveBeenCalledWith(null, expect.objectContaining({ currency_code: "XTS" })));
  });

  it("shows broader coverage and historical limitations before saving", async () => {
    vi.mocked(api.listAccountCurrencies).mockResolvedValue(coverage);
    const user = userEvent.setup();
    render(<QueryTestProvider><ManualAccountForm onClose={vi.fn()} onSaved={vi.fn()} /></QueryTestProvider>);
    await user.click(screen.getByLabelText("Moeda"));
    await user.click(await screen.findByText(/ARS —/));
    expect(screen.getByText(/de 07\/07\/1998 até 14\/10\/2026/)).toBeVisible();
    expect(screen.getByText(/Não há garantia de cotação para todas as datas/)).toBeVisible();
  });

  it.each(["unavailable", "stale"] as const)("keeps BRL usable with %s coverage", async (status) => {
    vi.mocked(api.listAccountCurrencies).mockResolvedValue({ ...coverage, status, currencies: status === "unavailable" ? [{ code: "BRL" }] : coverage.currencies });
    vi.mocked(api.writeManualAccount).mockResolvedValue({ ...manual, currency_code: "BRL" });
    const user = userEvent.setup();
    render(<QueryTestProvider><ManualAccountForm onClose={vi.fn()} onSaved={vi.fn()} /></QueryTestProvider>);
    expect(await screen.findByText(status === "stale" ? /Usando a última lista disponível/ : /BRL continua disponível/)).toBeVisible();
    await user.type(screen.getByLabelText("Nome"), "Conta offline");
    await user.click(screen.getByRole("button", { name: "Criar conta" }));
    await waitFor(() => expect(api.writeManualAccount).toHaveBeenCalledWith(null, expect.objectContaining({ currency_code: "BRL" })));
  });

  it("preserves an existing unsupported currency and locked balance", async () => {
    vi.mocked(api.listAccountCurrencies).mockResolvedValue(coverage);
    render(<QueryTestProvider><ManualAccountForm account={{ ...manual, currency_code: "XTS" }} onClose={vi.fn()} onSaved={vi.fn()} /></QueryTestProvider>);
    expect(await screen.findByText("Sem conversão para BRL para XTS.")).toBeVisible();
    expect(screen.getByLabelText("Moeda")).toBeDisabled();
    expect(screen.getByLabelText("Saldo inicial")).toHaveValue("100");
  });

  it("does not call an unverified currency unsupported during an outage", async () => {
    vi.mocked(api.listAccountCurrencies).mockResolvedValue({ status: "unavailable", source: "frankfurter-v2-blended", currencies: [{ code: "BRL" }] });
    const user = userEvent.setup();
    render(<QueryTestProvider><ManualAccountForm onClose={vi.fn()} onSaved={vi.fn()} /></QueryTestProvider>);
    await user.click(screen.getByRole("checkbox", { name: "Usar outra moeda" }));
    await user.type(screen.getByLabelText("Código da moeda"), "XTS");
    expect(await screen.findByText("Conversão de XTS para BRL não confirmada.")).toBeVisible();
    expect(screen.queryByText(/sincronizar não resolve/)).toBeNull();
  });

});
