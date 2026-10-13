import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "../api/problems";
import * as statementApi from "../api/statementImports";
import type { ImportHistoryItem, ImportPreview, ImportRow } from "../api/statementImports";
import * as compactScreen from "../components/shared/useCompactScreen";
import { QueryTestProvider } from "../test/QueryTestProvider";
import { StatementImportPage } from "./StatementImportPage";

vi.mock("../api/statementImports");
afterEach(() => vi.clearAllMocks());

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}

function row(overrides: Partial<ImportRow> = {}): ImportRow {
  return {
    line_number: 2, occurred_at: "2026-09-08T06:17:00.000000000Z", description: "Depósito", amount: "50.00", currency: "BRL",
    payment_method: "Depósito", balance: "50.00", status: "new", errors: [], warnings: [], ...overrides,
  };
}

function preview(rows: ImportRow[] = [row()], overrides: Partial<ImportPreview> = {}): ImportPreview {
  const count = (status: ImportRow["status"]) => rows.filter((item) => item.status === status).length;
  return {
    format: "flash_csv", format_version: "1", sha256: "abc", preview_fingerprint: "fingerprint", filename: "sample.csv",
    period_start: "2026-09-08", period_end: "2026-09-25", currency: "BRL",
    counts: { total: rows.length, new: count("new"), duplicate: count("duplicate"), invalid: count("invalid") },
    warnings: [], rows, ...overrides,
  };
}

const invalidRow = row({ line_number: 3, occurred_at: null, description: "", amount: null, currency: null, payment_method: null, balance: null, status: "invalid", errors: ["data inválida"] });
const saved = { run_id: "run", account_id: "acc", counts: { total: 1, new: 1, duplicate: 0, invalid: 0 } };
const historyItem: ImportHistoryItem = {
  run_id: "run-1", account_id: "acc-1", account_name: "Flash", filename: "extrato.csv", format: "flash_csv",
  created_at: "2026-09-10T12:00:00Z", counts: { total: 3, new: 2, duplicate: 1, invalid: 0 },
};

const csv = (name = "sample.csv") => new File(["synthetic"], name, { type: "text/csv" });

function renderPage() {
  return render(
    <MemoryRouter>
      <QueryTestProvider>
        <StatementImportPage />
      </QueryTestProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.mocked(statementApi.listImportAccounts).mockResolvedValue([]);
  vi.mocked(statementApi.listStatementImports).mockResolvedValue([]);
});

describe("StatementImportPage", () => {
  it("shows invalid rows and requires explicit partial import approval", async () => {
    const user = userEvent.setup();
    vi.mocked(statementApi.previewStatement).mockResolvedValue(preview([row(), invalidRow]));
    vi.mocked(statementApi.confirmStatement).mockResolvedValue({ ...saved, counts: { total: 2, new: 1, duplicate: 0, invalid: 1 } });
    renderPage();
    await user.upload(screen.getByLabelText("Arquivo CSV"), csv());
    expect(await screen.findByText("data inválida")).toBeVisible();
    expect(screen.getByText("data inválida")).toHaveClass("statement-row-error");
    // A new row is the default: only the rows that will not import as they are carry a tag.
    expect(screen.queryByText("Nova")).toBeNull();
    expect(screen.getByText("Inválida")).toBeVisible();
    expect(screen.getByText("08/09/2026 a 25/09/2026")).toBeVisible();
    await user.type(screen.getByLabelText("Nome da nova conta"), "Flash");
    const confirm = screen.getByRole("button", { name: "Importar 1 transação" });
    expect(confirm).toBeDisabled();
    await user.click(screen.getByRole("checkbox", { name: /Importar somente as linhas válidas/ }));
    await user.click(confirm);
    await waitFor(() => expect(statementApi.confirmStatement).toHaveBeenCalledWith(expect.any(File), expect.any(Object), { newAccountName: "Flash" }, true, {}));
    expect(await screen.findByText("Extrato importado")).toBeVisible();
    expect(screen.getByRole("link", { name: "Ver conta" })).toHaveAttribute("href", "/contas-e-cartoes/acc");
    // Nothing is left to confirm once the import is done.
    expect(screen.queryByRole("button", { name: /Importar 1 transação/ })).toBeNull();
  });

  it("asks for the account before the import can be confirmed", async () => {
    const user = userEvent.setup();
    vi.mocked(statementApi.previewStatement).mockResolvedValue(preview());
    renderPage();
    await user.upload(screen.getByLabelText("Arquivo CSV"), csv());
    expect(await screen.findByText("Informe a conta para confirmar.")).toBeVisible();
    expect(screen.getByRole("button", { name: "Importar 1 transação" })).toBeDisabled();
    await user.type(screen.getByLabelText("Nome da nova conta"), "Flash");
    expect(screen.queryByText("Informe a conta para confirmar.")).toBeNull();
    expect(screen.getByRole("button", { name: "Importar 1 transação" })).toBeEnabled();
  });

  it("registers a re-sent file without new entries", async () => {
    const user = userEvent.setup();
    vi.mocked(statementApi.previewStatement).mockResolvedValue(preview([row({ status: "duplicate" })]));
    renderPage();
    await user.upload(screen.getByLabelText("Arquivo CSV"), csv());
    expect(await screen.findByText("Este extrato já foi importado nesta conta. Nenhuma transação nova será criada.")).toBeVisible();
    expect(screen.getByRole("button", { name: "Registrar reenvio sem novas transações" })).toBeDisabled();
  });

  it("requires an explicit choice for each same-looking row from another file", async () => {
    const user = userEvent.setup();
    const ambiguous = row({ status: "ambiguous", warnings: ["movimentação idêntica em outro arquivo"] });
    vi.mocked(statementApi.listImportAccounts).mockResolvedValue([{ id: "acc", name: "Flash", currency_code: "BRL" }]);
    vi.mocked(statementApi.previewStatement).mockResolvedValue(preview([ambiguous]));
    vi.mocked(statementApi.confirmStatement).mockResolvedValue({ ...saved, counts: { total: 1, new: 1, duplicate: 0, invalid: 0 } });
    renderPage();
    await user.click(screen.getByRole("radio", { name: "Usar conta existente" }));
    const account = screen.getByRole("combobox", { name: "Conta de arquivo" });
    await user.click(account);
    await user.click(await screen.findByText("Flash (BRL)"));
    await user.upload(screen.getByLabelText("Arquivo CSV"), csv());
    expect(await screen.findByText("Precisa de decisão")).toBeVisible();
    const confirm = screen.getByRole("button", { name: "Registrar reenvio sem novas transações" });
    expect(confirm).toBeDisabled();
    await user.click(screen.getByRole("radio", { name: "Importar como outra compra" }));
    const importButton = screen.getByRole("button", { name: "Importar 1 transação" });
    expect(importButton).toBeEnabled();
    await user.click(importButton);
    await waitFor(() => expect(statementApi.confirmStatement).toHaveBeenCalledWith(
      expect.any(File), expect.any(Object), { accountId: "acc" }, false, { 2: "import" },
    ));
  });

  describe("layout", () => {
    afterEach(() => vi.restoreAllMocks());

    it("puts the confirm action in the page header on a wide screen", async () => {
      const user = userEvent.setup();
      vi.spyOn(compactScreen, "useCompactScreen").mockReturnValue(false);
      vi.mocked(statementApi.previewStatement).mockResolvedValue(preview());
      const { container } = renderPage();
      expect(container.querySelector(".page-actions")).toBeNull();
      await user.upload(screen.getByLabelText("Arquivo CSV"), csv());

      const confirm = await screen.findByRole("button", { name: "Importar 1 transação" });
      expect(screen.getAllByRole("button", { name: /Importar 1 transação/ })).toHaveLength(1);
      expect(confirm.closest(".page-actions")).not.toBeNull();
      expect(container.querySelector(".bottom-action-bar")).toBeNull();
      expect(screen.getByRole("link", { name: "Voltar para contas e cartões" })).toHaveAttribute("href", "/contas-e-cartoes");
    });

    it("moves it to the bottom bar on a phone, only while there is something to confirm", async () => {
      const user = userEvent.setup();
      const scrollIntoView = vi.fn();
      Object.defineProperty(Element.prototype, "scrollIntoView", { configurable: true, writable: true, value: scrollIntoView });
      try {
        vi.spyOn(compactScreen, "useCompactScreen").mockReturnValue(true);
        vi.mocked(statementApi.previewStatement).mockResolvedValue(preview());
        vi.mocked(statementApi.confirmStatement).mockResolvedValue(saved);
        const { container } = renderPage();

        // The chevron is the way back, and nothing reserves the bar yet.
        expect(screen.getByRole("link", { name: "Voltar para contas e cartões" })).toHaveAttribute("href", "/contas-e-cartoes");
        expect(container.querySelector(".bottom-action-bar")).toBeNull();
        expect(container.querySelector(".app-page-has-bottom-bar")).toBeNull();

        await user.upload(screen.getByLabelText("Arquivo CSV"), csv());
        const confirm = await screen.findByRole("button", { name: "Importar 1 transação" });
        expect(screen.getAllByRole("button", { name: /Importar 1 transação/ })).toHaveLength(1);
        expect(confirm.closest(".bottom-action-bar")).not.toBeNull();
        expect(container.querySelector(".app-page-has-bottom-bar")).not.toBeNull();
        expect(screen.getByRole("button", { name: /Baixar modelo/ })).toBeVisible();

        await user.type(screen.getByLabelText("Nome da nova conta"), "Flash");
        await user.click(confirm);
        expect(await screen.findByText("Extrato importado")).toBeVisible();
        expect(container.querySelector(".bottom-action-bar")).toBeNull();
        expect(container.querySelector(".app-page-has-bottom-bar")).toBeNull();
        expect(scrollIntoView).toHaveBeenCalledTimes(1);
      } finally {
        Reflect.deleteProperty(Element.prototype, "scrollIntoView");
      }
    });

    it("offers nothing to confirm while the preview is still loading", async () => {
      const user = userEvent.setup();
      vi.spyOn(compactScreen, "useCompactScreen").mockReturnValue(true);
      vi.mocked(statementApi.previewStatement).mockReturnValue(new Promise(() => {}));
      const { container } = renderPage();
      await user.upload(screen.getByLabelText("Arquivo CSV"), csv());

      expect(await screen.findByText("Processando extrato…")).toBeVisible();
      expect(screen.getByRole("status")).toHaveTextContent("Processando extrato…");
      expect(container.querySelector(".bottom-action-bar")).toBeNull();
    });
  });

  describe("preview rows", () => {
    it("pages through the rows, 20 at a time", async () => {
      const user = userEvent.setup();
      const rows = Array.from({ length: 45 }, (_, index) => row({ line_number: index + 2, description: `Movimento ${index + 1}` }));
      vi.mocked(statementApi.previewStatement).mockResolvedValue(preview(rows));
      renderPage();
      await user.upload(screen.getByLabelText("Arquivo CSV"), csv());

      const list = await screen.findByRole("list", { name: "Linhas do arquivo" });
      expect(within(list).getAllByRole("listitem")).toHaveLength(20);
      expect(screen.getByText(/Exibindo 1–20 de/)).toHaveTextContent("Exibindo 1–20 de 45 linhas");
      expect(screen.getByRole("button", { name: "Anterior" })).toBeDisabled();

      await user.click(screen.getByRole("button", { name: "Próxima" }));
      expect(screen.getByText("Movimento 21")).toBeVisible();
      expect(screen.queryByText("Movimento 20")).toBeNull();
      expect(screen.getByText("Página 2 de 3")).toBeVisible();

      await user.click(screen.getByRole("button", { name: "Próxima" }));
      expect(within(screen.getByRole("list", { name: "Linhas do arquivo" })).getAllByRole("listitem")).toHaveLength(5);
      expect(screen.getByText(/Exibindo 41–45 de/)).toBeVisible();
      expect(screen.getByRole("button", { name: "Próxima" })).toBeDisabled();
    });

    it("keeps warnings visible and does not paginate a short file", async () => {
      const user = userEvent.setup();
      vi.mocked(statementApi.previewStatement).mockResolvedValue(preview([row({ warnings: ["saldo divergente"] })]));
      renderPage();
      await user.upload(screen.getByLabelText("Arquivo CSV"), csv());

      const warning = await screen.findByText("saldo divergente");
      expect(warning).toBeVisible();
      expect(warning).not.toHaveClass("statement-row-error");
      expect(screen.queryByRole("navigation", { name: "Paginação das linhas" })).toBeNull();
    });
  });

  describe("conflict on confirmation", () => {
    it("clears the preview, shows the server's message and lets the preview be generated again", async () => {
      const user = userEvent.setup();
      vi.mocked(statementApi.previewStatement).mockResolvedValue(preview());
      vi.mocked(statementApi.confirmStatement).mockRejectedValue(new ApiError("conflict", "O extrato mudou desde a prévia."));
      renderPage();
      await user.upload(screen.getByLabelText("Arquivo CSV"), csv());
      await user.type(screen.getByLabelText("Nome da nova conta"), "Flash");
      await user.click(await screen.findByRole("button", { name: "Importar 1 transação" }));

      expect(await screen.findByText("O extrato mudou desde a prévia.")).toBeVisible();
      expect(screen.queryByRole("heading", { name: "Revise a prévia" })).toBeNull();
      expect(screen.queryByRole("button", { name: /Importar 1 transação/ })).toBeNull();

      await user.click(screen.getByRole("button", { name: "Gerar prévia novamente" }));
      await waitFor(() => expect(statementApi.previewStatement).toHaveBeenCalledTimes(2));
      expect(await screen.findByRole("heading", { name: "Revise a prévia" })).toBeVisible();
      expect(screen.queryByText("O extrato mudou desde a prévia.")).toBeNull();
      expect(screen.getByRole("button", { name: "Importar 1 transação" })).toBeEnabled();
    });

    it("keeps the preview when the confirmation fails for another reason", async () => {
      const user = userEvent.setup();
      vi.mocked(statementApi.previewStatement).mockResolvedValue(preview());
      vi.mocked(statementApi.confirmStatement).mockRejectedValue(new ApiError("transport", "Não foi possível comunicar com o servidor."));
      renderPage();
      await user.upload(screen.getByLabelText("Arquivo CSV"), csv());
      await user.type(screen.getByLabelText("Nome da nova conta"), "Flash");
      await user.click(await screen.findByRole("button", { name: "Importar 1 transação" }));

      expect(await screen.findByText("Não foi possível comunicar com o servidor.")).toBeVisible();
      expect(screen.getByRole("heading", { name: "Revise a prévia" })).toBeVisible();
      expect(screen.queryByRole("button", { name: "Gerar prévia novamente" })).toBeNull();
      await waitFor(() => expect(screen.getByRole("button", { name: "Importar 1 transação" })).toBeEnabled());
    });

    it("offers to generate the preview again when it could not be produced", async () => {
      const user = userEvent.setup();
      vi.mocked(statementApi.previewStatement)
        .mockRejectedValueOnce(new ApiError("response", "Arquivo em formato desconhecido."))
        .mockResolvedValueOnce(preview());
      renderPage();
      await user.upload(screen.getByLabelText("Arquivo CSV"), csv());

      expect(await screen.findByText("Arquivo em formato desconhecido.")).toBeVisible();
      await user.click(screen.getByRole("button", { name: "Gerar prévia novamente" }));
      expect(await screen.findByRole("heading", { name: "Revise a prévia" })).toBeVisible();
      expect(screen.queryByText("Arquivo em formato desconhecido.")).toBeNull();
    });
  });

  describe("list failures", () => {
    const region = (name: string) => within(screen.getByRole("region", { name }));

    it("shows the account list as unavailable, with a retry, while the history still loads", async () => {
      const user = userEvent.setup();
      vi.mocked(statementApi.listImportAccounts).mockRejectedValueOnce(new ApiError("transport", "offline")).mockResolvedValue([
        { id: "acc-1", name: "Flash", currency_code: "BRL" },
      ]);
      vi.mocked(statementApi.listStatementImports).mockResolvedValue([historyItem]);
      renderPage();

      expect(await screen.findByText("Não foi possível listar contas de arquivo")).toBeVisible();
      expect(screen.getByRole("radio", { name: "Usar conta existente" })).toBeDisabled();
      expect(await screen.findByText(/extrato\.csv/)).toBeVisible();
      expect(screen.queryByText("Não foi possível carregar o histórico")).toBeNull();

      await user.click(region("Escolha a conta").getByRole("button", { name: "Tentar novamente" }));
      await waitFor(() => expect(screen.queryByText("Não foi possível listar contas de arquivo")).toBeNull());
      expect(statementApi.listImportAccounts).toHaveBeenCalledTimes(2);
      expect(statementApi.listStatementImports).toHaveBeenCalledTimes(1);
      expect(screen.getByRole("radio", { name: "Usar conta existente" })).toBeEnabled();
    });

    it("shows the history as unavailable, not empty, and retries only the history", async () => {
      const user = userEvent.setup();
      vi.mocked(statementApi.listStatementImports).mockRejectedValueOnce(new ApiError("transport", "offline")).mockResolvedValue([historyItem]);
      renderPage();

      expect(await screen.findByText("Não foi possível carregar o histórico")).toBeVisible();
      expect(screen.queryByText("Nenhum extrato importado ainda")).toBeNull();
      expect(screen.queryByText("Não foi possível listar contas de arquivo")).toBeNull();

      await user.click(region("Importações recentes").getByRole("button", { name: "Tentar novamente" }));
      expect(await screen.findByText(/extrato\.csv/)).toBeVisible();
      expect(statementApi.listStatementImports).toHaveBeenCalledTimes(2);
      expect(statementApi.listImportAccounts).toHaveBeenCalledTimes(1);
    });

    it("reports both failures independently", async () => {
      vi.mocked(statementApi.listImportAccounts).mockRejectedValue(new ApiError("transport", "offline"));
      vi.mocked(statementApi.listStatementImports).mockRejectedValue(new ApiError("transport", "offline"));
      renderPage();

      expect(await screen.findByText("Não foi possível listar contas de arquivo")).toBeVisible();
      expect(await screen.findByText("Não foi possível carregar o histórico")).toBeVisible();
      expect(screen.getAllByRole("button", { name: "Tentar novamente" })).toHaveLength(2);
    });
  });

  describe("recent imports", () => {
    it("lists past imports as rows with a link to the account", async () => {
      vi.mocked(statementApi.listStatementImports).mockResolvedValue([historyItem, { ...historyItem, run_id: "run-2", filename: "outro.csv" }]);
      renderPage();

      const rows = within(await screen.findByRole("list", { name: "Extratos importados" })).getAllByRole("listitem");
      // No "2 importações" subtitle: the list is the count.
      expect(screen.queryByText("2 importações")).toBeNull();
      expect(rows).toHaveLength(2);
      expect(within(rows[0]).getByText("Flash")).toBeVisible();
      expect(within(rows[0]).getByText(/extrato\.csv/)).toBeVisible();
      expect(within(rows[0]).getByText("2 novos · 1 já importados · 0 inválidos")).toBeVisible();
      expect(within(rows[0]).getByRole("link", { name: "Ver conta Flash" })).toHaveAttribute("href", "/contas-e-cartoes/acc-1");
    });

    it("says so when nothing was imported yet", async () => {
      renderPage();
      expect(await screen.findByText("Nenhum extrato importado ainda")).toBeVisible();
    });

    it("refreshes both lists after an import", async () => {
      const user = userEvent.setup();
      vi.mocked(statementApi.previewStatement).mockResolvedValue(preview());
      vi.mocked(statementApi.confirmStatement).mockResolvedValue(saved);
      renderPage();
      await user.upload(screen.getByLabelText("Arquivo CSV"), csv());
      await user.type(screen.getByLabelText("Nome da nova conta"), "Flash");
      await user.click(await screen.findByRole("button", { name: "Importar 1 transação" }));

      expect(await screen.findByText("Extrato importado")).toBeVisible();
      expect(statementApi.listImportAccounts).toHaveBeenCalledTimes(2);
      expect(statementApi.listStatementImports).toHaveBeenCalledTimes(2);
    });
  });

  describe("stale previews", () => {
    it("never lets an older preview overwrite the one for the newer file", async () => {
      const user = userEvent.setup();
      const older = deferred<ImportPreview>();
      const newer = deferred<ImportPreview>();
      vi.mocked(statementApi.previewStatement).mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);
      renderPage();
      const input = screen.getByLabelText("Arquivo CSV");
      await user.upload(input, csv("antigo.csv"));
      await user.upload(input, csv("novo.csv"));
      await waitFor(() => expect(statementApi.previewStatement).toHaveBeenCalledTimes(2));

      await act(async () => { newer.resolve(preview([row({ description: "Do arquivo novo" })])); });
      expect(await screen.findByText("Do arquivo novo")).toBeVisible();

      await act(async () => {
        older.resolve(preview([row({ description: "Do arquivo antigo" })]));
        await new Promise((resolve) => setTimeout(resolve, 20));
      });
      expect(screen.queryByText("Do arquivo antigo")).toBeNull();
      expect(screen.getByText("Do arquivo novo")).toBeVisible();
    });

    it("keeps waiting for the newer preview when the older one answers first", async () => {
      const user = userEvent.setup();
      const older = deferred<ImportPreview>();
      const newer = deferred<ImportPreview>();
      vi.mocked(statementApi.previewStatement).mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);
      renderPage();
      const input = screen.getByLabelText("Arquivo CSV");
      await user.upload(input, csv("antigo.csv"));
      await user.upload(input, csv("novo.csv"));
      await waitFor(() => expect(statementApi.previewStatement).toHaveBeenCalledTimes(2));

      await act(async () => {
        older.resolve(preview([row({ description: "Do arquivo antigo" })]));
        await new Promise((resolve) => setTimeout(resolve, 20));
      });
      expect(screen.queryByText("Do arquivo antigo")).toBeNull();
      expect(screen.getByText("Processando extrato…")).toBeVisible();

      await act(async () => { newer.resolve(preview([row({ description: "Do arquivo novo" })])); });
      expect(await screen.findByText("Do arquivo novo")).toBeVisible();
      expect(screen.queryByText("Processando extrato…")).toBeNull();
    });

    it("drops a preview still in flight when the account choice changes", async () => {
      const user = userEvent.setup();
      const pending = deferred<ImportPreview>();
      vi.mocked(statementApi.listImportAccounts).mockResolvedValue([{ id: "acc-1", name: "Flash", currency_code: "BRL" }]);
      vi.mocked(statementApi.previewStatement).mockReturnValueOnce(pending.promise);
      renderPage();
      await waitFor(() => expect(screen.getByRole("radio", { name: "Usar conta existente" })).toBeEnabled());
      await user.upload(screen.getByLabelText("Arquivo CSV"), csv());
      expect(await screen.findByText("Processando extrato…")).toBeVisible();

      // No account is picked yet, so switching target previews nothing.
      await user.click(screen.getByRole("radio", { name: "Usar conta existente" }));
      expect(screen.queryByText("Processando extrato…")).toBeNull();

      await act(async () => {
        pending.resolve(preview([row({ description: "Sem conta escolhida" })]));
        await new Promise((resolve) => setTimeout(resolve, 20));
      });
      expect(screen.queryByText("Sem conta escolhida")).toBeNull();
      expect(screen.queryByRole("heading", { name: "Revise a prévia" })).toBeNull();
      expect(statementApi.previewStatement).toHaveBeenCalledTimes(1);
    });
  });

  describe("confirmation", () => {
    it("cannot be submitted twice", async () => {
      const user = userEvent.setup();
      const pending = deferred<typeof saved>();
      vi.mocked(statementApi.previewStatement).mockResolvedValue(preview());
      vi.mocked(statementApi.confirmStatement).mockReturnValue(pending.promise);
      renderPage();
      await user.upload(screen.getByLabelText("Arquivo CSV"), csv());
      await user.type(screen.getByLabelText("Nome da nova conta"), "Flash");
      const confirm = await screen.findByRole("button", { name: "Importar 1 transação" });

      // Two clicks before React can re-render: only the lock can stop the second.
      await act(async () => { confirm.click(); confirm.click(); });
      expect(statementApi.confirmStatement).toHaveBeenCalledTimes(1);
      // While loading, antd prefixes the button's name with its spinner's label.
      await waitFor(() => expect(screen.getByRole("button", { name: /Importar 1 transação/ })).toBeDisabled());
      await user.click(screen.getByRole("button", { name: /Importar 1 transação/ }));
      expect(statementApi.confirmStatement).toHaveBeenCalledTimes(1);

      await act(async () => { pending.resolve(saved); });
      expect(await screen.findByText("Extrato importado")).toBeVisible();
      expect(statementApi.confirmStatement).toHaveBeenCalledTimes(1);
    });
  });

  describe("template download", () => {
    it("downloads the template once, however many times it is clicked", async () => {
      const user = userEvent.setup();
      const pending = deferred<void>();
      vi.mocked(statementApi.downloadStatementTemplate).mockReturnValue(pending.promise);
      renderPage();
      expect(screen.getByText(/Não tem o arquivo\? Baixe o modelo no formato Flash/)).toBeVisible();
      const download = screen.getByRole("button", { name: /Baixar modelo \(CSV\)/ });

      await act(async () => { download.click(); download.click(); });
      expect(statementApi.downloadStatementTemplate).toHaveBeenCalledTimes(1);
      await waitFor(() => expect(download).toHaveClass("ant-btn-loading"));
      await user.click(download);
      expect(statementApi.downloadStatementTemplate).toHaveBeenCalledTimes(1);

      await act(async () => { pending.resolve(); });
      await waitFor(() => expect(download).not.toHaveClass("ant-btn-loading"));
      expect(screen.queryByRole("alert")).toBeNull();

      await user.click(download);
      expect(statementApi.downloadStatementTemplate).toHaveBeenCalledTimes(2);
    });

    it("shows the server's message when the download fails, and clears it on the next try", async () => {
      const user = userEvent.setup();
      vi.mocked(statementApi.downloadStatementTemplate)
        .mockRejectedValueOnce(new ApiError("response", "Modelo indisponível no momento."))
        .mockResolvedValueOnce(undefined);
      renderPage();

      await user.click(screen.getByRole("button", { name: /Baixar modelo \(CSV\)/ }));
      expect(await screen.findByText("Modelo indisponível no momento.")).toBeVisible();

      await user.click(screen.getByRole("button", { name: /Baixar modelo \(CSV\)/ }));
      await waitFor(() => expect(screen.queryByText("Modelo indisponível no momento.")).toBeNull());
    });
  });
});
