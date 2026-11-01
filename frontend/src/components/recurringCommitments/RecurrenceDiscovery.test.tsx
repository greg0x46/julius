import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "../../api/recurringCommitments";
import * as accounts from "../../api/accounts";
import type { RecurrenceSuggestion, SuggestionReview } from "../../api/contracts/recurrenceSuggestions";
import type { Category } from "../../api/contracts";
import { QueryTestProvider } from "../../test/QueryTestProvider";
import { RecurrenceDiscovery } from "./RecurrenceDiscovery";

vi.mock("../../api/recurringCommitments");
vi.mock("../../api/accounts");
const category: Category = { id: "22222222-2222-4222-8222-222222222222", name: "Serviços", kind: "expense", is_active: true, icon: "home", color: "#495057", created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" };
const candidate: RecurrenceSuggestion = { key: "a".repeat(64), draft: { name: "Internet", kind: "expense", amount: "100.1234", category_id: category.id, account_id: null, cadence: "monthly", day_of_month: 5, month_of_year: null, start_date: "2026-12-05", end_date: null, is_active: true }, currency_code: "BRL", amount_kind: "fixed", minimum_amount: "100.1234", maximum_amount: "100.1234", confidence: "high", reasons: ["consistent_calendar", "fixed_amount"], transaction_ids: ["synthetic"], card_number: "", evidence: [{ id: "synthetic", date: "2026-10-05", description: "Internet histórica", amount: "-100.1234" }] };
const preview: SuggestionReview = { token: "b".repeat(64), conditions: [{ field: "description", operator: "equals", value: "Internet" }], warnings: [], needs_review: false, history: [{ id: "synthetic", date: "2026-10-05", description: "Internet histórica", amount: "-100.1234", currency_code: "BRL", evidence: true, status: "matched", reasons: ["conditions_and_month_match"], failed_conditions: [], rule_ids: [], scenario_ids: [] }] };
function mount() { render(<QueryTestProvider><RecurrenceDiscovery categories={[category]} /></QueryTestProvider>); }
async function review(name = "Internet") {
  const card = (await screen.findByRole("heading", { name })).closest("section")!;
  fireEvent.click(within(card).getByRole("button", { name: "Revisar" }));
  const drawer = screen.getByRole("dialog");
  fireEvent.click(within(drawer).getByRole("button", { name: "Atualizar prévia" }));
  await waitFor(() => expect(within(drawer).getByRole("button", { name: "Concluir revisão" })).toBeEnabled());
  return drawer;
}
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(api.discoverRecurrences).mockResolvedValue([candidate]);
  vi.mocked(api.reviewRecurrenceSuggestion).mockResolvedValue(preview);
  vi.mocked(api.acceptRecurrenceSuggestions).mockImplementation(async (items) => items.map((item) => ({ key: item.key, scenario_id: "created", status: 201, error: "" })));
  vi.mocked(api.dismissRecurrenceSuggestion).mockResolvedValue(undefined);
  vi.mocked(accounts.listAccounts).mockResolvedValue([]);
});

describe("recurrence discovery", () => {
  it("shows evidence and requires review before individual creation", async () => {
    mount();
    expect(await screen.findByText(/Internet histórica/)).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Selecionar Internet para criação" })).toBeDisabled();
    const drawer = await review();
    fireEvent.click(within(drawer).getByRole("button", { name: "Criar recorrência e regra" }));
    await waitFor(() => expect(api.acceptRecurrenceSuggestions).toHaveBeenCalledWith([expect.objectContaining({ key: candidate.key, token: preview.token, draft: expect.objectContaining({ amount: "100.1234" }), conditions: preview.conditions })]));
    expect(await screen.findByText("Nenhuma sugestão encontrada")).toBeInTheDocument();
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
    mount(); fireEvent.click(await screen.findByRole("button", { name: "Revisar" }));
    fireEvent.click(screen.getByRole("button", { name: "Atualizar prévia" }));
    expect(api.reviewRecurrenceSuggestion).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Criar recorrência e regra" })).toBeDisabled();
  });
  it("shows ambiguity, requires acknowledgment, and blocks unsupported conditions", async () => {
    vi.mocked(api.reviewRecurrenceSuggestion).mockResolvedValue({ ...preview, needs_review: true, warnings: ["multiple_matches_per_occurrence"], history: [{ ...preview.history[0], status: "ambiguous" }] });
    mount(); fireEvent.click(await screen.findByRole("button", { name: "Revisar" }));
    const drawer = screen.getByRole("dialog");
    fireEvent.click(within(drawer).getByRole("button", { name: "Atualizar prévia" }));
    expect(await within(drawer).findByText(/Correspondência ambígua/)).toBeInTheDocument();
    expect(within(drawer).getByRole("button", { name: "Criar recorrência e regra" })).toBeDisabled();
    fireEvent.click(within(drawer).getByRole("checkbox", { name: /Revisei os alertas/ }));
    expect(within(drawer).getByRole("button", { name: "Criar recorrência e regra" })).toBeEnabled();
    vi.mocked(api.reviewRecurrenceSuggestion).mockResolvedValue({ ...preview, needs_review: true, warnings: ["cross_month_unsupported"] });
    fireEvent.click(within(drawer).getByRole("button", { name: "Atualizar prévia" }));
    expect(await within(drawer).findByText(/Conciliação entre meses não suportada/)).toBeInTheDocument();
    expect(within(drawer).getByRole("button", { name: "Criar recorrência e regra" })).toBeDisabled();
  });
  it("bulk accepts reviewed items and retains failures for another review", async () => {
    const second = { ...candidate, key: "c".repeat(64), draft: { ...candidate.draft, name: "Música" } };
    vi.mocked(api.discoverRecurrences).mockResolvedValue([candidate, second]);
    vi.mocked(api.acceptRecurrenceSuggestions).mockResolvedValue([{ key: candidate.key, scenario_id: "created", status: 201, error: "" }, { key: second.key, scenario_id: "", status: 409, error: "Os dados desta sugestão mudaram. Atualize a descoberta e revise novamente." }]);
    mount();
    for (const name of ["Internet", "Música"]) { const drawer = await review(name); fireEvent.click(within(drawer).getByRole("button", { name: "Concluir revisão" })); }
    fireEvent.click(screen.getByRole("button", { name: "Selecionar sugestões revisadas" }));
    fireEvent.click(screen.getByRole("button", { name: "Criar selecionadas (2)" }));
    expect(await screen.findByText(/Os dados desta sugestão mudaram/)).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Internet" })).not.toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: "Selecionar Música para criação" })).toBeDisabled();
    expect(api.acceptRecurrenceSuggestions).toHaveBeenCalledWith(expect.arrayContaining([expect.objectContaining({ key: candidate.key }), expect.objectContaining({ key: second.key })]));
  });
  it("dismisses an unwanted pattern without creation", async () => {
    mount(); fireEvent.click(await screen.findByRole("button", { name: "Dispensar" }));
    await waitFor(() => expect(api.dismissRecurrenceSuggestion).toHaveBeenCalledWith(candidate.key));
    expect(await screen.findByText("Nenhuma sugestão encontrada")).toBeInTheDocument();
    expect(api.acceptRecurrenceSuggestions).not.toHaveBeenCalled();
  });
  it("shows loading, empty and retryable load errors", async () => {
    let finish!: (value: RecurrenceSuggestion[]) => void;
    vi.mocked(api.discoverRecurrences).mockReturnValue(new Promise((resolve) => { finish = resolve; }));
    mount(); expect(screen.getByRole("status")).toHaveTextContent("Buscando padrões");
    finish([]); expect(await screen.findByText("Nenhuma sugestão encontrada")).toBeInTheDocument();
  });
});
