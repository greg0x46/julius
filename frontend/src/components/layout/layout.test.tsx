import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as compactScreen from "../shared/useCompactScreen";
import { DetailPage, EmptyState, FilterButton, ListRow, Page, PageAction, ResponsiveList, Section, SummaryStrip } from ".";
import type { DetailFreshness } from ".";

function renderInRouter(node: React.ReactNode) {
  return render(<MemoryRouter>{node}</MemoryRouter>);
}

const useCompact = (compact: boolean) =>
  vi.spyOn(compactScreen, "useCompactScreen").mockReturnValue(compact);

afterEach(() => vi.restoreAllMocks());

describe("Page", () => {
  it("renders a single h1, the back chevron and the actions in the title row", () => {
    useCompact(true);
    const { container } = renderInRouter(
      <Page title="Nu Pagamentos" backTo="/contas" backLabel="Voltar para contas" actions={<button>Nova</button>}>
        conteúdo
      </Page>,
    );
    expect(screen.getAllByRole("heading", { level: 1 })).toHaveLength(1);
    expect(screen.getByRole("heading", { name: "Nu Pagamentos" })).toBeVisible();
    expect(screen.getByRole("link", { name: "Voltar para contas" })).toHaveAttribute("href", "/contas");
    expect(screen.getByRole("button", { name: "Nova" }).closest(".page-header .page-actions")).not.toBeNull();
    expect(container.querySelector(".bottom-action-bar")).toBeNull();
  });

  it("does not repeat the actions in the title row when a bottom bar owns them on a phone", () => {
    useCompact(true);
    renderInRouter(
      <Page title="Importar" actions={<button>Confirmar</button>} hasBottomActionBar>
        conteúdo
      </Page>,
    );
    expect(screen.queryByRole("button", { name: "Confirmar" })).toBeNull();
  });

  it("hides the description on a phone only with compactMobileHeader", () => {
    useCompact(true);
    const { rerender } = renderInRouter(
      <Page title="Pendências" description="Uma linha">
        x
      </Page>,
    );
    expect(screen.getByText("Uma linha")).toBeVisible();
    rerender(
      <MemoryRouter>
        <Page title="Pendências" description="Uma linha" compactMobileHeader>
          x
        </Page>
      </MemoryRouter>,
    );
    expect(screen.queryByText("Uma linha")).toBeNull();
  });
});

describe("PageAction", () => {
  it("shows the short label on a phone and keeps the full name accessible", async () => {
    useCompact(true);
    const onClick = vi.fn();
    render(<PageAction label="Nova transação" shortLabel="Nova" onClick={onClick} />);
    const button = screen.getByRole("button", { name: "Nova transação" });
    expect(button).toHaveTextContent("Nova");
    expect(button).not.toHaveTextContent("Nova transação");
    await userEvent.click(button);
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it("shows the full label on a wide screen", () => {
    useCompact(false);
    render(<PageAction label="Nova transação" shortLabel="Nova" />);
    expect(screen.getByRole("button", { name: "Nova transação" })).toHaveTextContent("Nova transação");
  });
});

describe("ResponsiveList", () => {
  const items = [{ id: "1", name: "Conta Corrente" }];
  const list = () => (
    <ResponsiveList
      label="Contas"
      items={items}
      getKey={(item) => item.id}
      row={(item) => ({ title: item.name, ariaLabel: `Abrir ${item.name}`, onClick: () => {} })}
      wide={<table aria-label="Tabela de contas" />}
    />
  );

  it("stacks rows on a phone", () => {
    useCompact(true);
    renderInRouter(list());
    expect(screen.getByRole("list", { name: "Contas" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Abrir Conta Corrente" })).toBeVisible();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("keeps the wide layout from md up", () => {
    useCompact(false);
    renderInRouter(list());
    expect(screen.getByRole("table", { name: "Tabela de contas" })).toBeInTheDocument();
    expect(screen.queryByRole("list")).toBeNull();
  });

  it("renders the empty state instead of either layout when there is nothing", () => {
    useCompact(true);
    renderInRouter(
      <ResponsiveList
        label="Contas"
        items={[]}
        getKey={() => ""}
        row={() => ({ title: "", ariaLabel: "" })}
        wide={<table />}
        empty={<EmptyState title="Nenhuma conta ainda" hint="Importe um extrato." />}
      />,
    );
    expect(screen.getByText("Nenhuma conta ainda")).toBeVisible();
    expect(screen.getByText("Importe um extrato.")).toBeVisible();
  });
});

describe("ListRow", () => {
  it("is a link when given an href", () => {
    renderInRouter(
      <ul>
        <ListRow title="Itaú" meta="Conta corrente" trailing="R$ 10,00" status="Atrasada" href="/contas/1" ariaLabel="Abrir Itaú" />
      </ul>,
    );
    expect(screen.getByRole("link", { name: "Abrir Itaú" })).toHaveAttribute("href", "/contas/1");
    expect(screen.getByText("Atrasada")).toBeVisible();
  });

  it("is plain text, with no accessible name of its own, when it is not interactive", () => {
    renderInRouter(
      <ul>
        <ListRow title="Cartão final 1234" meta="3 transações" />
      </ul>,
    );
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.queryByRole("link")).toBeNull();
    expect(screen.getByText("Cartão final 1234").closest(".list-row")).not.toHaveAttribute("aria-label");
  });

  it("marks title and meta for single-line truncation with `truncate`", () => {
    renderInRouter(
      <ul>
        <ListRow title="Nome muito longo" meta="Instituição" truncate onClick={() => undefined} ariaLabel="Abrir" />
      </ul>,
    );
    expect(screen.getByRole("button", { name: "Abrir" })).toHaveClass("list-row-truncate");
  });

  it("keeps a leading control beside the row button, never inside it", async () => {
    const onClick = vi.fn();
    renderInRouter(
      <ul>
        <ListRow title="Internet" onClick={onClick} ariaLabel="Abrir Internet" leading={<input type="checkbox" aria-label="Selecionar Internet" />} />
      </ul>,
    );
    const row = screen.getByRole("button", { name: "Abrir Internet" });
    const checkbox = screen.getByRole("checkbox", { name: "Selecionar Internet" });
    expect(row.contains(checkbox)).toBe(false);
    expect(checkbox.closest("li")).toBe(row.closest("li"));
    await userEvent.click(checkbox);
    expect(onClick).not.toHaveBeenCalled();
  });
});

describe("FilterButton", () => {
  it("announces the active count in its name and shows it as a badge", () => {
    render(<FilterButton activeCount={2} onClick={() => undefined} />);
    const button = screen.getByRole("button", { name: "Filtros 2" });
    expect(button.querySelector(".list-toolbar-filter-count")).toHaveTextContent("2");
  });

  it("shows no badge without active filters", () => {
    render(<FilterButton onClick={() => undefined} />);
    expect(screen.getByRole("button", { name: "Filtros" }).querySelector(".list-toolbar-filter-count")).toBeNull();
  });
});

describe("Section and SummaryStrip", () => {
  it("names a section by its heading and keeps the trailing bit on the header", () => {
    render(
      <Section title="Cartões" trailing={<a href="/x">Ver todos</a>}>
        corpo
      </Section>,
    );
    const region = screen.getByRole("region", { name: "Cartões" });
    expect(region.querySelector(".section-header")).toContainElement(screen.getByRole("link", { name: "Ver todos" }));
  });

  it("lays out the hero figure and the secondary figures as a definition list", () => {
    render(
      <SummaryStrip
        label="Saldo em contas"
        value="R$ 22.590,67"
        items={[{ label: "Fatura dos cartões", value: "R$ 18.073,44" }]}
        note="Só contas em reais"
      />,
    );
    expect(screen.getByRole("region", { name: "Saldo em contas" })).toHaveTextContent("R$ 22.590,67");
    expect(screen.getByText("Fatura dos cartões").tagName).toBe("DT");
    expect(screen.getByText("Só contas em reais")).toBeVisible();
  });

  it("renders a children slot (meter, status) under the hero instead of in the note", () => {
    render(
      <SummaryStrip label="Falta pagar" value="R$ 800,00">
        <span>Quitado 20%</span>
      </SummaryStrip>,
    );
    const strip = screen.getByRole("region", { name: "Falta pagar" });
    expect(strip.querySelector(".summary-strip-hero .summary-strip-extra")).toHaveTextContent("Quitado 20%");
    expect(strip.querySelector(".summary-strip-note")).toBeNull();
  });
});

describe("DetailPage", () => {
  const renderDetail = (freshness: DetailFreshness, snapshot: { name: string } | null) =>
    renderInRouter(
      <DetailPage
        title="Pendência"
        backTo="/pendencias"
        backLabel="Voltar para pendências"
        state={{ snapshot, freshness, retrying: false }}
        retry={() => undefined}
        recordTitle={(record) => record.name}
        notFoundMessage="Não encontrada"
        notFoundDescription="Nada com este identificador."
        unavailableMessage="Indisponível"
        actions={<button>Mais ações</button>}
      >
        {(record) => <p>corpo de {record.name}</p>}
      </DetailPage>,
    );

  it("puts the page actions in the title row once the record is loaded", () => {
    useCompact(false);
    renderDetail("fresh", { name: "Financiamento" });
    expect(screen.getByRole("heading", { level: 1, name: "Financiamento" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Mais ações" }).closest(".page-header .page-actions")).not.toBeNull();
    expect(screen.getByText("corpo de Financiamento")).toBeVisible();
  });

  it("offers no actions while loading or when the record does not exist", () => {
    useCompact(false);
    const { unmount } = renderDetail("loading", null);
    expect(screen.queryByRole("button", { name: "Mais ações" })).toBeNull();
    unmount();
    renderDetail("not_found", null);
    expect(screen.queryByRole("button", { name: "Mais ações" })).toBeNull();
    expect(screen.getByText("Não encontrada")).toBeVisible();
  });
});
