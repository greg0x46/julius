import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "../../api/recurringCommitments";
import * as accounts from "../../api/accounts";
import type { RecurrenceSuggestion, SuggestionReview } from "../../api/contracts/recurrenceSuggestions";
import type { Category } from "../../api/contracts";
import { QueryTestProvider } from "../../test/QueryTestProvider";
import * as compactScreen from "../shared/useCompactScreen";
import { RecurrenceDiscovery } from "./RecurrenceDiscovery";

vi.mock("../../api/recurringCommitments");
vi.mock("../../api/accounts");
const originalMatchMedia = window.matchMedia;
const category: Category = { id: "22222222-2222-4222-8222-222222222222", name: "Serviços", kind: "expense", is_active: true, icon: "home", color: "#495057", created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" };
const candidate: RecurrenceSuggestion = { key: "a".repeat(64), draft: { name: "Internet", kind: "expense", amount: "100.1234", category_id: category.id, account_id: null, cadence: "monthly", day_of_month: 5, month_of_year: null, start_date: "2026-12-05", end_date: null, is_active: true }, currency_code: "BRL", amount_kind: "fixed", minimum_amount: "100.1234", maximum_amount: "100.1234", confidence: "high", reasons: ["consistent_calendar", "fixed_amount"], transaction_ids: ["synthetic"], card_number: "", evidence: [{ id: "synthetic", date: "2026-10-05", description: "Internet histórica", amount: "-100.1234" }] };
const preview: SuggestionReview = { token: "b".repeat(64), conditions: [{ field: "description", operator: "equals", value: "Internet" }], warnings: [], needs_review: false, history: [{ id: "synthetic", date: "2026-10-05", description: "Internet histórica", amount: "-100.1234", currency_code: "BRL", evidence: true, status: "matched", reasons: ["conditions_and_month_match"], failed_conditions: [], rule_ids: [], scenario_ids: [] }] };
function mount(onBottomBarChange?: (visible: boolean) => void) { render(<QueryTestProvider><RecurrenceDiscovery categories={[category]} onBottomBarChange={onBottomBarChange} /></QueryTestProvider>); }
// The stacked layout of a phone: no breakpoint matches, so the table gives way to rows.
function usePhone() {
  vi.spyOn(compactScreen, "useCompactScreen").mockReturnValue(true);
  window.matchMedia = ((query: string) => ({ matches: false, media: query, onchange: null, addListener: () => undefined, removeListener: () => undefined, addEventListener: () => undefined, removeEventListener: () => undefined, dispatchEvent: () => false })) as typeof window.matchMedia;
}
async function open(name = "Internet") {
  fireEvent.click(await screen.findByRole("button", { name: `Revisar ${name}` }));
  return screen.getByRole("dialog");
}
// Opening already simulates the rule; the review is done once creation is enabled.
async function review(name = "Internet") {
  const drawer = await open(name);
  await waitFor(() => expect(within(drawer).getByRole("button", { name: "Criar recorrência e regra" })).toBeEnabled());
  return drawer;
}
const close = (drawer: HTMLElement) => fireEvent.click(within(drawer).getByRole("button", { name: "Fechar" }));
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(api.discoverRecurrences).mockResolvedValue([candidate]);
  vi.mocked(api.reviewRecurrenceSuggestion).mockResolvedValue(preview);
  vi.mocked(api.acceptRecurrenceSuggestions).mockImplementation(async (items) => items.map((item) => ({ key: item.key, scenario_id: "created", status: 201, error: "" })));
  vi.mocked(api.dismissRecurrenceSuggestion).mockResolvedValue(undefined);
  vi.mocked(accounts.listAccounts).mockResolvedValue([]);
});
afterEach(() => {
  vi.restoreAllMocks();
  window.matchMedia = originalMatchMedia;
});

describe("recurrence discovery", () => {
  it("shows evidence in the review and requires review before individual creation", async () => {
    mount();
    const row = await screen.findByRole("row", { name: /Internet/ });
    expect(within(row).getByRole("checkbox", { name: "Selecionar Internet para criação" })).toBeDisabled();
    const drawer = await open();
    expect(within(drawer).getByText(/Internet histórica/)).toBeInTheDocument();
    expect(within(drawer).getByRole("button", { name: "Criar recorrência e regra" })).toBeDisabled();
    await waitFor(() => expect(within(drawer).getByRole("button", { name: "Criar recorrência e regra" })).toBeEnabled());
    expect(api.reviewRecurrenceSuggestion).toHaveBeenCalledTimes(1);
    expect(api.acceptRecurrenceSuggestions).not.toHaveBeenCalled();
    fireEvent.click(within(drawer).getByRole("button", { name: "Criar recorrência e regra" }));
    await waitFor(() => expect(api.acceptRecurrenceSuggestions).toHaveBeenCalledWith([expect.objectContaining({ key: candidate.key, token: preview.token, draft: expect.objectContaining({ amount: "100.1234" }), conditions: preview.conditions })]));
    expect(await screen.findByText("Nenhuma sugestão encontrada")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
  it("explains the way in and simulates the rule as the review opens", async () => {
    mount();
    const row = await screen.findByRole("row", { name: /Internet/ });
    expect(screen.getByText(/Clique em Revisar para ver a regra simulada/)).toBeInTheDocument();
    fireEvent.click(within(row).getByRole("button", { name: "Revisar Internet" }));
    const drawer = screen.getByRole("dialog");
    expect(within(drawer).getByRole("status")).toHaveTextContent("Simulando a regra no histórico");
    await waitFor(() => expect(within(drawer).getByRole("status")).toHaveTextContent("Regra simulada"));
    expect(api.reviewRecurrenceSuggestion).toHaveBeenCalledWith(expect.objectContaining({ key: candidate.key, draft: candidate.draft }));
    expect(within(drawer).getByRole("button", { name: "Criar recorrência e regra" })).toHaveClass("ui-btn-primary");
    expect(within(drawer).queryByRole("button", { name: "Concluir revisão" })).not.toBeInTheDocument();
    close(drawer);
    expect(screen.queryByText(/Clique em Revisar para ver a regra simulada/)).not.toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Selecionar Internet para criação" })).toBeEnabled();
  });
  it("asks for a category first and simulates only once the owner asks", async () => {
    vi.mocked(api.discoverRecurrences).mockResolvedValue([{ ...candidate, draft: { ...candidate.draft, category_id: "" } }]);
    mount(); fireEvent.click(await screen.findByRole("button", { name: "Revisar Internet" }));
    expect(screen.getByRole("status")).toHaveTextContent("Escolha uma categoria ativa");
    expect(api.reviewRecurrenceSuggestion).not.toHaveBeenCalled();
  });
  it("keeps the step line true when the automatic simulation fails", async () => {
    vi.mocked(api.reviewRecurrenceSuggestion).mockRejectedValueOnce(new Error("falhou"));
    mount(); const drawer = await open();
    expect(await within(drawer).findByRole("alert")).toBeInTheDocument();
    expect(within(drawer).getByRole("status")).toHaveTextContent("Clique em Atualizar prévia para simular a regra.");
    expect(within(drawer).getByRole("status")).not.toHaveTextContent("Dados alterados");
    fireEvent.click(within(drawer).getByRole("button", { name: "Atualizar prévia" }));
    await waitFor(() => expect(within(drawer).getByRole("button", { name: "Criar recorrência e regra" })).toBeEnabled());
  });
  it("asks for another simulation when creating fails and the review is dropped", async () => {
    vi.mocked(api.acceptRecurrenceSuggestions).mockResolvedValue([{ key: candidate.key, scenario_id: "", status: 409, error: "Os dados desta sugestão mudaram. Atualize a descoberta e revise novamente." }]);
    mount(); const drawer = await review();
    fireEvent.click(within(drawer).getByRole("button", { name: "Criar recorrência e regra" }));
    expect(await within(drawer).findByText(/Os dados desta sugestão mudaram/)).toBeInTheDocument();
    expect(within(drawer).getByRole("status")).toHaveTextContent("Dados alterados. Clique em Atualizar prévia");
    expect(within(drawer).getByRole("button", { name: "Criar recorrência e regra" })).toBeDisabled();
  });
  it("lists only the transactions the rule finds, plus evidence it misses", async () => {
    const row = preview.history[0];
    vi.mocked(api.reviewRecurrenceSuggestion).mockResolvedValue({ ...preview, history: [
      row,
      { ...row, id: "outside", description: "Mercado", evidence: false, status: "unmatched", reasons: ["conditions_failed"] },
      { ...row, id: "missed", description: "Internet perdida", evidence: true, status: "unmatched", reasons: ["conditions_failed"] },
    ] });
    mount(); const drawer = await review();
    const list = within(drawer).getByRole("list", { name: "Transações que a regra encontra" });
    expect(within(list).getAllByRole("listitem")).toHaveLength(2);
    expect(within(list).queryByText("Mercado")).not.toBeInTheDocument();
    expect(within(list).getByText("Internet perdida")).toBeInTheDocument();
  });
  it("asks to confirm the warnings before an ambiguous preview can be created", async () => {
    vi.mocked(api.reviewRecurrenceSuggestion).mockResolvedValue({ ...preview, needs_review: true, warnings: ["multiple_matches_per_occurrence"], history: [{ ...preview.history[0], status: "ambiguous" }] });
    mount(); const drawer = await open();
    expect(await within(drawer).findByText("Confirme os alertas da prévia para liberar a criação.")).toBeInTheDocument();
    fireEvent.click(within(drawer).getByRole("checkbox", { name: /Revisei os alertas/ }));
    expect(within(drawer).getByText(/Regra simulada/)).toBeInTheDocument();
  });
  it("invalidates a preview when a custom select changes", async () => {
    mount(); const drawer = await review();
    fireEvent.click(within(drawer).getByLabelText("Tipo"));
    fireEvent.click(screen.getByRole("option", { name: "Entrada" }));
    expect(within(drawer).getByRole("button", { name: "Criar recorrência e regra" })).toBeDisabled();
    expect(screen.getByRole("checkbox", { name: "Selecionar Internet para criação" })).toBeDisabled();
  });
  it("requires an active category before requesting a preview", async () => {
    vi.mocked(api.discoverRecurrences).mockResolvedValue([{ ...candidate, draft: { ...candidate.draft, category_id: "" } }]);
    mount(); fireEvent.click(await screen.findByRole("button", { name: "Revisar Internet" }));
    fireEvent.click(screen.getByRole("button", { name: "Atualizar prévia" }));
    expect(api.reviewRecurrenceSuggestion).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Criar recorrência e regra" })).toBeDisabled();
  });
  it("shows ambiguity, requires acknowledgment, and blocks unsupported conditions", async () => {
    vi.mocked(api.reviewRecurrenceSuggestion).mockResolvedValue({ ...preview, needs_review: true, warnings: ["multiple_matches_per_occurrence"], history: [{ ...preview.history[0], status: "ambiguous" }] });
    mount(); const drawer = await open();
    expect(await within(drawer).findByText(/Correspondência ambígua/)).toBeInTheDocument();
    expect(within(drawer).getByRole("button", { name: "Criar recorrência e regra" })).toBeDisabled();
    fireEvent.click(within(drawer).getByRole("checkbox", { name: /Revisei os alertas/ }));
    expect(within(drawer).getByRole("button", { name: "Criar recorrência e regra" })).toBeEnabled();
    vi.mocked(api.reviewRecurrenceSuggestion).mockResolvedValue({ ...preview, needs_review: true, warnings: ["cross_month_unsupported"] });
    await waitFor(() => expect(within(drawer).getByRole("button", { name: "Atualizar prévia" })).toBeEnabled());
    fireEvent.click(within(drawer).getByRole("button", { name: "Atualizar prévia" }));
    expect(await within(drawer).findByText(/Conciliação entre meses não suportada/)).toBeInTheDocument();
    expect(within(drawer).getByRole("button", { name: "Criar recorrência e regra" })).toBeDisabled();
  });
  it("bulk accepts reviewed items and retains failures for another review", async () => {
    const second = { ...candidate, key: "c".repeat(64), draft: { ...candidate.draft, name: "Música" } };
    vi.mocked(api.discoverRecurrences).mockResolvedValue([candidate, second]);
    vi.mocked(api.acceptRecurrenceSuggestions).mockResolvedValue([{ key: candidate.key, scenario_id: "created", status: 201, error: "" }, { key: second.key, scenario_id: "", status: 409, error: "Os dados desta sugestão mudaram. Atualize a descoberta e revise novamente." }]);
    mount();
    for (const name of ["Internet", "Música"]) { const drawer = await review(name); close(drawer); }
    fireEvent.click(screen.getByRole("button", { name: "Selecionar sugestões revisadas" }));
    fireEvent.click(screen.getByRole("button", { name: "Criar selecionadas (2)" }));
    const failure = await screen.findByText(/Os dados desta sugestão mudaram/);
    expect(failure.closest("[role=alert]")).toHaveTextContent("Música");
    expect(screen.queryByRole("button", { name: "Revisar Internet" })).not.toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Selecionar Música para criação" })).toBeDisabled();
    expect(screen.getByText("1 recorrências criadas. Revise as sugestões pendentes.")).toBeInTheDocument();
    expect(api.acceptRecurrenceSuggestions).toHaveBeenCalledWith(expect.arrayContaining([expect.objectContaining({ key: candidate.key }), expect.objectContaining({ key: second.key })]));
  });
  it("refreshes discovery and requires another review", async () => {
    mount(); const drawer = await review();
    close(drawer);
    expect(screen.getByRole("checkbox", { name: "Selecionar Internet para criação" })).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "Atualizar descoberta" }));
    await waitFor(() => expect(api.discoverRecurrences).toHaveBeenCalledTimes(2));
    expect(screen.getByRole("checkbox", { name: "Selecionar Internet para criação" })).toBeDisabled();
  });
  it("dismisses an unwanted pattern from its review without creation", async () => {
    mount(); const drawer = await open();
    fireEvent.click(within(drawer).getByRole("button", { name: "Dispensar" }));
    await waitFor(() => expect(api.dismissRecurrenceSuggestion).toHaveBeenCalledWith(candidate.key));
    expect(await screen.findByText("Nenhuma sugestão encontrada")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(api.acceptRecurrenceSuggestions).not.toHaveBeenCalled();
  });
  it("keeps a failed dismissal in the review", async () => {
    vi.mocked(api.dismissRecurrenceSuggestion).mockRejectedValue(new Error("falhou"));
    mount(); const drawer = await open();
    fireEvent.click(within(drawer).getByRole("button", { name: "Dispensar" }));
    expect(await within(drawer).findByRole("alert")).toBeInTheDocument();
    expect(screen.getByRole("dialog")).toBe(drawer);
  });
  it("shows loading and empty states", async () => {
    let finish!: (value: RecurrenceSuggestion[]) => void;
    vi.mocked(api.discoverRecurrences).mockReturnValue(new Promise((resolve) => { finish = resolve; }));
    mount(); expect(screen.getByRole("status")).toHaveTextContent("Buscando padrões");
    finish([]); expect(await screen.findByText("Nenhuma sugestão encontrada")).toBeInTheDocument();
  });
  it("shows a load failure with a retry", async () => {
    vi.mocked(api.discoverRecurrences).mockRejectedValueOnce(new Error("falhou"));
    mount();
    expect(await screen.findByText("Não foi possível carregar as sugestões. Tente novamente.")).toBeInTheDocument();
    expect(screen.queryByText("Nenhuma sugestão encontrada")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Tentar novamente" }));
    expect(await screen.findByRole("button", { name: "Revisar Internet" })).toBeInTheDocument();
  });
});

describe("recurrence discovery list", () => {
  const salary = { ...candidate, key: "d".repeat(64), confidence: "medium" as const, amount_kind: "variable" as const, minimum_amount: "8000.00", maximum_amount: "9000.00", draft: { ...candidate.draft, name: "Salário", kind: "income" as const, amount: "8500.00" } };

  it("summarises the candidates with monthly totals, outflow in the text colour and inflow in green", async () => {
    vi.mocked(api.discoverRecurrences).mockResolvedValue([candidate, salary]);
    mount();
    const summary = await screen.findByRole("region", { name: "Resumo das candidatas" });
    expect(summary).toHaveTextContent("2 candidatas");
    const outflow = within(summary).getByText(/^-R\$\s100,12$/);
    const inflow = within(summary).getByText(/^\+R\$\s8\.500,00$/);
    expect(outflow).toHaveClass("money-neutral");
    expect(inflow).toHaveClass("money-positive");
    expect(summary).toHaveTextContent("Saídas por mês");
    expect(summary).toHaveTextContent("Entradas por mês");
  });
  it("tags only non-default states: lower confidence and a review already done", async () => {
    vi.mocked(api.discoverRecurrences).mockResolvedValue([candidate, salary]);
    mount();
    const internet = await screen.findByRole("row", { name: /Internet/ });
    const pay = screen.getByRole("row", { name: /Salário/ });
    expect(within(internet).queryByText(/Confiança/)).not.toBeInTheDocument();
    expect(within(internet).queryByText("Revisada")).not.toBeInTheDocument();
    expect(within(pay).getByText("Confiança média")).toHaveClass("status-tag-warning");
    expect(within(pay).getByText(/varia de R\$\s8\.000,00 a R\$\s9\.000,00/)).toBeInTheDocument();
    const drawer = await review();
    close(drawer);
    expect(within(screen.getByRole("row", { name: /Internet/ })).getByText("Revisada")).toHaveClass("status-tag-success");
    expect(within(screen.getByRole("row", { name: /Salário/ })).queryByText("Revisada")).not.toBeInTheDocument();
  });
  it("opens the review from the row, but not from its checkbox", async () => {
    mount();
    const row = await screen.findByRole("row", { name: /Internet/ });
    fireEvent.click(within(row).getByText("Serviços"));
    const drawer = screen.getByRole("dialog");
    expect(drawer).toHaveAccessibleName("Revisar: Internet");
    await waitFor(() => expect(within(drawer).getByRole("button", { name: "Criar recorrência e regra" })).toBeEnabled());
    close(drawer);
    fireEvent.click(within(screen.getByRole("row", { name: /Internet/ })).getByRole("checkbox", { name: "Selecionar Internet para criação" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Criar selecionadas (1)" })).toBeEnabled();
  });
  it("stacks into rows on a phone, with the checkbox beside the row button and the bulk actions in the bottom bar", async () => {
    usePhone();
    const onBottomBarChange = vi.fn();
    vi.mocked(api.discoverRecurrences).mockResolvedValue([candidate, salary]);
    mount(onBottomBarChange);
    const row = await screen.findByRole("button", { name: "Revisar Internet" });
    expect(screen.getByRole("button", { name: "Revisar Salário" })).toHaveTextContent(/Entrada · todo dia 5 · varia de R\$\s8\.000,00 a R\$\s9\.000,00/);
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    expect(row).toHaveTextContent("Saída · todo dia 5 · 1 transação");
    expect(row).toHaveTextContent(/-R\$\s100,12/);
    const checkbox = screen.getByRole("checkbox", { name: "Selecionar Internet para criação" });
    expect(row.contains(checkbox)).toBe(false);
    expect(checkbox.closest("li")).toBe(row.closest("li"));
    expect(screen.queryByRole("button", { name: "Selecionar sugestões revisadas" })).not.toBeInTheDocument();
    const drawer = await review();
    close(drawer);
    await waitFor(() => expect(onBottomBarChange).toHaveBeenLastCalledWith(true));
    fireEvent.click(screen.getByRole("button", { name: "Selecionar sugestões revisadas" }));
    expect(screen.getByRole("button", { name: "Criar selecionadas (1)" })).toBeEnabled();
  });
});
