import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import dayjs from "dayjs";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as accountsApi from "../api/accounts";
import * as investmentsApi from "../api/investments";
import * as portfolioApi from "../api/investmentPortfolio";
import { QueryTestProvider } from "../test/QueryTestProvider";
import {
  closedPosition,
  integratedAccount,
  investmentPeriodFixture,
  manualAccount,
  manualPosition,
  periodTesouro,
  periodUnavailable,
  summary,
  syncedPosition,
} from "../test/investmentWorkspaceFixtures";
import { InvestmentsPage } from "./InvestmentsPage";

vi.mock("../api/accounts");
vi.mock("../api/investments");
vi.mock("../api/investmentPortfolio");

const desktopMatchMedia = window.matchMedia;

function storePeriod(value: unknown) {
  window.localStorage.setItem("julius.periodo", JSON.stringify(value));
}

function renderPage() {
  return render(
    <MemoryRouter>
      <QueryTestProvider>
        <InvestmentsPage />
      </QueryTestProvider>
    </MemoryRouter>,
  );
}

function summaryOf() {
  return screen.getByRole("region", { name: /Carteira em/ });
}

async function chooseGrouping(label: string) {
  await userEvent.click(screen.getByText(label, { selector: ".ant-segmented-item-label" }));
}

// Groups start collapsed; most checks need to see their rows.
async function expandGroups() {
  for (const toggle of document.querySelectorAll<HTMLElement>(".investment-group-toggle[aria-expanded=false]")) {
    await userEvent.click(toggle);
  }
}

/** Renders the page by custody with every group expanded (the fixture's richest shape). */
async function renderCustodyRows() {
  renderPage();
  await screen.findByRole("region", { name: /Carteira em/ });
  await chooseGrouping("Custódia");
  await screen.findByText("Corretora XP", { selector: ".investment-group-name" });
  await expandGroups();
}

beforeEach(() => {
  window.localStorage.clear();
  storePeriod({ from: "2025-09-01", to: "2025-09-30" });
  vi.mocked(accountsApi.listAccounts).mockResolvedValue([]);
  vi.mocked(investmentsApi.listInvestments).mockResolvedValue([]);
  vi.mocked(portfolioApi.listInvestmentAccounts).mockResolvedValue([manualAccount, integratedAccount]);
  vi.mocked(portfolioApi.listInvestmentPositions).mockResolvedValue([manualPosition, closedPosition, syncedPosition]);
  vi.mocked(portfolioApi.listInvestmentOperations).mockResolvedValue([]);
  vi.mocked(portfolioApi.listInvestmentReconciliations).mockResolvedValue([]);
  vi.mocked(portfolioApi.getInvestmentSummary).mockResolvedValue(summary);
  vi.mocked(portfolioApi.getInvestmentPeriod).mockImplementation(async (params) =>
    investmentPeriodFixture({
      group_by: params.groupBy,
      from: params.from,
      to: params.to,
      source: params.source,
    }),
  );
});

afterEach(() => {
  window.matchMedia = desktopMatchMedia;
  window.localStorage.clear();
});

describe("InvestmentsPage period view", () => {
  it("asks the backend for the chosen period, grouped by class", async () => {
    renderPage();
    await screen.findByRole("region", { name: /Carteira em 30\/09\/2025/ });
    expect(portfolioApi.getInvestmentPeriod).toHaveBeenCalledWith(
      { from: "2025-09-01", to: "2025-09-30", groupBy: "class", source: null },
      expect.anything(),
    );
  });

  it("omits from for the whole period and ends it today", async () => {
    storePeriod({ preset: "all" });
    renderPage();
    await screen.findByRole("region", { name: /Carteira em/ });
    expect(portfolioApi.getInvestmentPeriod).toHaveBeenCalledWith(
      { from: null, to: dayjs().format("YYYY-MM-DD"), groupBy: "class", source: null },
      expect.anything(),
    );
  });

  it("sends a running period's own end and lets the server cut it at today", async () => {
    storePeriod({ from: "2025-09-01", to: "2999-12-31" });
    vi.mocked(portfolioApi.getInvestmentPeriod).mockImplementation(async (params) =>
      investmentPeriodFixture({ group_by: params.groupBy, to: "2025-09-30", to_clamped: true }),
    );
    renderPage();
    const strip = await screen.findByRole("region", { name: "Carteira em 30/09/2025" });
    expect(vi.mocked(portfolioApi.getInvestmentPeriod).mock.calls[0][0]).toMatchObject({ to: "2999-12-31" });
    expect(within(strip).getByText(/Período em andamento: considerado até hoje, 30\/09\/2025/)).toBeVisible();
  });

  it("sends no request for a period that has not started", async () => {
    storePeriod({ from: "2999-01-01", to: "2999-01-31" });
    renderPage();
    expect(await screen.findByText("Este período ainda não começou")).toBeVisible();
    expect(portfolioApi.getInvestmentPeriod).not.toHaveBeenCalled();
  });

  it("states the backend's totals and how they reconcile", async () => {
    renderPage();
    const strip = await screen.findByRole("region", { name: "Carteira em 30/09/2025" });
    expect(within(strip).getByText("R$ 1.500,00")).toBeVisible();
    expect(within(strip).getByText("+R$ 240,00")).toBeVisible();
    expect(within(strip).getByText("+13,48%")).toBeVisible();
    expect(within(strip).getByText("R$ 520,00")).toBeVisible();

    const toggle = within(strip).getByText("Entenda o resultado do período");
    expect(toggle.closest("details")).not.toHaveAttribute("open");
    await userEvent.click(toggle);
    expect(toggle.closest("details")).toHaveAttribute("open");
    const reconcile = strip.querySelector("#investment-reconcile") as HTMLElement;
    expect(within(reconcile).getByText("R$ 1.780,00")).toBeVisible();
    expect(within(reconcile).getByText("R$ 1.500,00")).toBeVisible();
  });

  it("separates the held positions from the ones closed in the period", async () => {
    renderPage();
    const strip = await screen.findByRole("region", { name: /Carteira em/ });
    await userEvent.click(within(strip).getByText("Entenda o resultado do período"));
    const origin = within(strip).getByRole("group", { name: "De onde vem o resultado do período" });
    expect(within(origin).getByText("Resultado do período = mantidas + encerradas")).toBeVisible();
    const held = within(origin).getByText("Posições mantidas em 30/09/2025").closest("div")!;
    expect(within(held).getByText("+R$ 220,00")).toBeVisible();
    expect(within(held).getByText("+17,19%")).toBeVisible();
    const closed = within(origin).getByText("Encerradas no período (resultado realizado)").closest("div")!;
    expect(within(closed).getByText("+R$ 20,00")).toBeVisible();
  });

  it("hides the closed line when nothing was closed in the period", async () => {
    const base = investmentPeriodFixture({ group_by: "none" });
    const zero = { start_value: "0", end_value: "0", contributions: "0", withdrawals: "0", yield: "0", capital_base: "0", yield_rate: null };
    vi.mocked(portfolioApi.getInvestmentPeriod).mockResolvedValue({
      ...base,
      totals: { ...base.totals, closed: zero },
    });
    renderPage();
    const strip = await screen.findByRole("region", { name: /Carteira em/ });
    await userEvent.click(within(strip).getByText("Entenda o resultado do período"));
    expect(within(strip).getByText(/Posições mantidas em/)).toBeVisible();
    expect(within(strip).queryByText(/Encerradas no período/)).toBeNull();
  });

  it("flags a partial period instead of counting unavailable positions as zero", async () => {
    renderPage();
    const strip = await screen.findByRole("region", { name: /Carteira em/ });
    expect(within(strip).getByText(/Parcial: 1 de 4 posições sem histórico suficiente/)).toBeVisible();
    expect(within(strip).getByRole("button", { name: /1× Sem histórico/ })).toBeInTheDocument();
  });

  it("shows an unavailable position as — with its reason, never as R$ 0,00", async () => {
    await renderCustodyRows();
    const row = (await screen.findByText("Fundo Sem Histórico")).closest("tr")!;
    expect(within(row).getAllByText("—").length).toBeGreaterThanOrEqual(3);
    expect(within(row).queryByText(/R\$/)).toBeNull();
    expect(within(row).getByRole("button", { name: /Sem histórico: Nenhuma movimentação registrada/ })).toBeInTheDocument();
  });

  it("shows an unknown invested amount as — with the reason, not zero", async () => {
    await renderCustodyRows();
    const row = (await screen.findByText("CDB Banco Teste")).closest("tr")!;
    expect(within(row).getByRole("button", { name: /Valor investido indisponível: O histórico não inclui a primeira aplicação/ })).toBeInTheDocument();
    expect(within(row).getByText("Início parcial")).toBeVisible();
    expect(within(row).getByText("Histórico incompleto")).toBeVisible();
  });

  it("keeps the institution's own profit as a conference figure in the position detail", async () => {
    await renderCustodyRows();
    const row = (await screen.findByText("CDB Banco Teste")).closest("tr")!;
    await userEvent.click(within(row).getByRole("button", { name: "Detalhes do período de CDB Banco Teste" }));
    expect(await screen.findByText("Informado pela instituição")).toBeInTheDocument();
    expect(screen.getByText(/R\$\s21,50 em 30\/09\/2025/)).toBeInTheDocument();
    expect(screen.getByText("IR/IOF provisionado (não deduzido)")).toBeInTheDocument();
  });

  it("shows server subtotals and shares for each custody group", async () => {
    await renderCustodyRows();
    const group = (await screen.findByText("Corretora XP", { selector: ".investment-group-name" })).closest("tr")!;
    expect(within(group).getByText("R$ 1.200,00")).toBeVisible();
    expect(within(group).getByText("+R$ 220,00")).toBeVisible();
    expect(within(group).getByText("80,00%")).toBeVisible();
    const synced = screen.getByText("Banco Teste", { selector: ".investment-group-name" }).closest("tr")!;
    expect(within(synced).getByText("20,00%")).toBeVisible();
    expect(within(synced).getByText("Parcial")).toBeVisible();
    expect(within(synced).getByRole("button", { name: /Soma só das posições com valor investido conhecido; 1 não tem/ })).toBeInTheDocument();
  });

  it("changes grouping on the backend and leaves the summary untouched", async () => {
    renderPage();
    const strip = await screen.findByRole("region", { name: /Carteira em/ });
    const before = strip.textContent;
    await screen.findByText("Renda fixa", { selector: ".investment-group-name" });
    await chooseGrouping("Custódia");
    await screen.findByText("Corretora XP", { selector: ".investment-group-name" });
    expect(summaryOf().textContent).toBe(before);

    await chooseGrouping("Sem agrupamento");
    await waitFor(() =>
      expect(portfolioApi.getInvestmentPeriod).toHaveBeenLastCalledWith(
        expect.objectContaining({ groupBy: "none" }),
        expect.anything(),
      ),
    );
    // Without groups every row is listed and carries the server's share of the portfolio.
    const row = (await screen.findByText("Tesouro Selic 2029")).closest("tr")!;
    expect(within(row).getByText("80,00%")).toBeVisible();
    expect(summaryOf().textContent).toBe(before);
  });

  it("shows each position's own share, and — when there is none", async () => {
    await renderCustodyRows();
    const row = (await screen.findByText("CDB Banco Teste")).closest("tr")!;
    expect(within(row).getByText("20,00%")).toBeVisible();
    const unavailable = screen.getByText("Fundo Sem Histórico").closest("tr")!;
    expect(within(unavailable).queryByText(/%/)).toBeNull();
  });

  it("hides closed positions by default and says the totals still include them", async () => {
    await renderCustodyRows();
    expect(screen.queryByText("LCI Banco Antigo")).toBeNull();
    expect(screen.getByText("Totais e subtotais incluem 1 encerrada oculta.")).toBeVisible();
    const before = summaryOf().textContent;
    const callsBefore = vi.mocked(portfolioApi.getInvestmentPeriod).mock.calls.length;

    await userEvent.click(screen.getByRole("switch", { name: "Mostrar encerradas" }));
    expect(screen.getByText("LCI Banco Antigo")).toBeVisible();
    expect(screen.getByText("Encerrada")).toBeVisible();
    expect(screen.queryByText(/encerrada oculta/)).toBeNull();
    // A client-side filter: no new request and the same summary and subtotal.
    expect(vi.mocked(portfolioApi.getInvestmentPeriod).mock.calls.length).toBe(callsBefore);
    expect(summaryOf().textContent).toBe(before);
    const group = screen.getByText("Corretora XP", { selector: ".investment-group-name" }).closest("tr")!;
    expect(within(group).getByText("R$ 1.200,00")).toBeVisible();
  });

  it("hides a closed position that has no figures too, and says it is outside the totals", async () => {
    const base = investmentPeriodFixture({ group_by: "none" });
    const [group] = base.groups;
    const gone = { ...periodUnavailable, id: "00000000-0000-4000-8000-0000000000aa", name: "Fundo Encerrado", closed_as_of_to: true };
    vi.mocked(portfolioApi.getInvestmentPeriod).mockResolvedValue({
      ...base,
      groups: [{ ...group, positions: [periodTesouro, gone] }],
    });
    renderPage();
    await screen.findByText("Tesouro Selic 2029");
    expect(screen.queryByText("Fundo Encerrado")).toBeNull();
    expect(screen.getByText(/1 encerrada oculta está sem valor no período e fora dos totais/)).toBeVisible();
    expect(screen.queryByText(/Totais e subtotais incluem/)).toBeNull();

    await userEvent.click(screen.getByRole("switch", { name: "Mostrar encerradas" }));
    expect(screen.getByText("Fundo Encerrado")).toBeVisible();
  });

  it("searches rows by name or ticker without touching totals or subtotals", async () => {
    await renderCustodyRows();
    await screen.findByText("Tesouro Selic 2029");
    const before = summaryOf().textContent;
    const callsBefore = vi.mocked(portfolioApi.getInvestmentPeriod).mock.calls.length;
    await userEvent.type(screen.getByRole("textbox", { name: "Buscar ativo" }), "cdb");
    expect(screen.queryByText("Tesouro Selic 2029")).toBeNull();
    expect(screen.getByText("CDB Banco Teste")).toBeVisible();
    expect(screen.getByText(/A busca só esconde linhas/)).toBeVisible();
    // The synced group keeps its full subtotal even though one of its rows is hidden.
    const group = screen.getByText("Banco Teste", { selector: ".investment-group-name" }).closest("tr")!;
    expect(within(group).getByText("1 de 2 posições")).toBeVisible();
    expect(within(group).getByText("R$ 300,00")).toBeVisible();
    expect(summaryOf().textContent).toBe(before);
    expect(vi.mocked(portfolioApi.getInvestmentPeriod).mock.calls.length).toBe(callsBefore);
  });

  it("reports a search with no match", async () => {
    await renderCustodyRows();
    await screen.findByText("Tesouro Selic 2029");
    await userEvent.type(screen.getByRole("textbox", { name: "Buscar ativo" }), "zzz");
    expect(await screen.findByText("Nenhuma posição encontrada")).toBeVisible();
  });

  it("has no manual/synced split: no origin control, no tab, no origin tag", async () => {
    await renderCustodyRows();
    expect(screen.queryByText("Origem no cálculo")).toBeNull();
    expect(screen.queryByText("Sincronizados")).toBeNull();
    expect(screen.queryByText("Sincronizadas")).toBeNull();
    expect(screen.queryByText(/· Manual ·/)).toBeNull();
    expect(portfolioApi.getInvestmentPeriod).not.toHaveBeenCalledWith(
      expect.objectContaining({ source: expect.stringMatching(/manual|synced/) }),
      expect.anything(),
    );
  });

  it("shows each group's server share beside its subtotal", async () => {
    renderPage();
    await screen.findByRole("region", { name: /Carteira em/ });
    await chooseGrouping("Custódia");
    const group = (await screen.findByText("Corretora XP", { selector: ".investment-group-name" })).closest("tr")!;
    expect(within(group).getByText("80,00%")).toBeVisible();
    expect(within(group).getByText("R$ 1.200,00")).toBeVisible();
  });

  it("keeps account and position management reachable from the period view", async () => {
    await renderCustodyRows();
    await screen.findByText("Tesouro Selic 2029");
    await userEvent.click(screen.getByRole("button", { name: "Ações de Corretora XP" }));
    expect(await screen.findByRole("menuitem", { name: "Registrar movimentação" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Nova posição" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Editar conta" })).toBeInTheDocument();
    fireEvent.keyDown(document.body, { key: "Escape" });

    await userEvent.click(screen.getByRole("button", { name: "Ações de Tesouro Selic 2029" }));
    expect(await screen.findByRole("menuitem", { name: "Editar" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Excluir" })).toBeInTheDocument();
  });

  it("keeps a synced position read-only but links to its detail", async () => {
    await renderCustodyRows();
    const row = (await screen.findByText("CDB Banco Teste")).closest("tr")!;
    await userEvent.click(within(row).getByRole("button", { name: "Ações de CDB Banco Teste" }));
    expect(await screen.findByRole("menuitem", { name: "Ver detalhes" })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Editar" })).toBeNull();
  });

  it("lists the accounts below the positions, with their movements", async () => {
    renderPage();
    const section = await screen.findByRole("region", { name: "Contas e movimentações" });
    expect(within(section).getByText("Corretora XP")).toBeVisible();
    expect(within(section).getAllByText(/Histórico de movimentações \(0\)/)).toHaveLength(2);
    expect(within(section).getByRole("button", { name: "Ações da conta Corretora XP" })).toBeInTheDocument();
  });

  it("offers a way to create an account when there is none", async () => {
    vi.mocked(portfolioApi.listInvestmentAccounts).mockResolvedValue([]);
    vi.mocked(portfolioApi.listInvestmentPositions).mockResolvedValue([]);
    vi.mocked(portfolioApi.getInvestmentPeriod).mockResolvedValue(investmentPeriodFixture({ groups: [] }));
    renderPage();
    expect(await screen.findByText("Nenhuma posição neste período")).toBeVisible();
    expect(await screen.findByText("Nenhuma conta de investimento ainda")).toBeVisible();
  });

  it("starts groups collapsed, subtotal visible, and expands them to show the rows", async () => {
    renderPage();
    await screen.findByRole("region", { name: /Carteira em/ });
    await chooseGrouping("Custódia");
    const toggle = await screen.findByRole("button", { name: /Corretora XP/, expanded: false });
    expect(screen.queryByText("Tesouro Selic 2029")).toBeNull();
    // Collapsed hides rows only: the group's subtotal and share stay.
    expect(within(toggle.closest("tr")!).getByText("R$ 1.200,00")).toBeVisible();
    await userEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("Tesouro Selic 2029")).toBeVisible();
    await userEvent.click(toggle);
    expect(screen.queryByText("Tesouro Selic 2029")).toBeNull();
  });

  it("keeps the groups open while searching, so no match is hidden behind a collapsed header", async () => {
    renderPage();
    await screen.findByRole("region", { name: /Carteira em/ });
    await chooseGrouping("Custódia");
    await screen.findByText("Corretora XP", { selector: ".investment-group-name" });
    await userEvent.type(screen.getByRole("textbox", { name: "Buscar ativo" }), "selic");
    expect(screen.getByText("Tesouro Selic 2029")).toBeVisible();
  });

  it("lists every position in one table when ungrouped", async () => {
    renderPage();
    await screen.findByRole("region", { name: /Carteira em/ });
    await chooseGrouping("Sem agrupamento");
    expect(await screen.findByText("Tesouro Selic 2029")).toBeVisible();
    expect(screen.getByText("CDB Banco Teste")).toBeVisible();
    expect(screen.queryByRole("button", { expanded: false, name: /posições/ })).toBeNull();
  });
});

describe("InvestmentsPage period view on a phone", () => {
  beforeEach(() => {
    window.matchMedia = ((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => undefined,
      removeListener: () => undefined,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      dispatchEvent: () => false,
    })) as typeof window.matchMedia;
  });

  it("stacks the positions under collapsible group headers", async () => {
    renderPage();
    await screen.findByRole("region", { name: /Carteira em/ });
    await chooseGrouping("Custódia");
    const toggle = await screen.findByRole("button", { name: /Corretora XP/, expanded: false });
    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.queryByRole("list", { name: "Posições de Corretora XP" })).toBeNull();
    await userEvent.click(toggle);
    const list = screen.getByRole("list", { name: "Posições de Corretora XP" });
    expect(within(list).getByText("Tesouro Selic 2029")).toBeVisible();
    expect(toggle).toHaveAttribute("aria-expanded", "true");
  });

  it("never shows an unavailable position as zero", async () => {
    renderPage();
    await screen.findByRole("region", { name: /Carteira em/ });
    await chooseGrouping("Custódia");
    await userEvent.click(await screen.findByRole("button", { name: /Banco Teste/, expanded: false }));
    const list = await screen.findByRole("list", { name: "Posições de Banco Teste" });
    const row = within(list).getByText("Fundo Sem Histórico").closest(".list-row") as HTMLElement;
    expect(within(row).getAllByText("—").length).toBeGreaterThanOrEqual(2);
    expect(within(row).queryByText(/R\$/)).toBeNull();
  });
});
