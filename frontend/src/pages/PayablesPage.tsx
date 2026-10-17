import { Alert, Button } from "antd";
import { useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";

import type { Payable, PayableKind } from "../api/contracts";
import {
  CreateActionMenu,
  DataCard,
  EmptyState,
  ListToolbar,
  Page,
  PageTabs,
  SearchField,
  SortSelect,
  SummaryStrip,
} from "../components/layout";
import { PayableForm } from "../components/payables/PayableForm";
import { PayableList } from "../components/payables/PayableList";
import { Money } from "../components/shared/Money";
import { useFeedback } from "../components/shared/useFeedback";
import { usePayables } from "../hooks/usePayables";
import { errorMessage } from "../presentation/errors";
import { sumBRL } from "../presentation/money";
import { payableDetailPath, payableVocabulary } from "../presentation/payableLabels";

type PayableView = PayableKind | "all";
type PayableSort = "recent" | "name" | "remaining";

const viewOptions: { label: string; value: PayableView }[] = [
  { label: "Todas", value: "all" },
  { label: "Dívidas", value: "debt" },
  { label: "A receber", value: "receivable" },
];

const sortOptions: { label: string; value: PayableSort }[] = [
  { label: "Mais recentes", value: "recent" },
  { label: "Nome", value: "name" },
  { label: "Maior valor restante", value: "remaining" },
];

const comparators: Record<PayableSort, (left: Payable, right: Payable) => number> = {
  recent: (left, right) => right.created_at.localeCompare(left.created_at),
  name: (left, right) => left.name.localeCompare(right.name, "pt-BR"),
  remaining: (left, right) => Number(right.remaining_amount) - Number(left.remaining_amount),
};

/** Search and sort happen here: the list is small and already fully loaded. */
function arrange(payables: Payable[], search: string, sort: PayableSort): Payable[] {
  const needle = search.trim().toLocaleLowerCase("pt-BR");
  const matched = needle
    ? payables.filter((payable) => payable.name.toLocaleLowerCase("pt-BR").includes(needle))
    : payables;
  return [...matched].sort(comparators[sort]);
}

/** What is still open of one kind: a settled payable owes (or is owed) nothing, whatever its stored remainder. */
function remainingTotal(payables: Payable[], kind: PayableKind): string {
  return sumBRL(
    payables
      .filter((payable) => payable.kind === kind && payable.status === "open")
      .map((payable) => payable.remaining_amount),
  );
}

/**
 * What is still open. The debts and the receivables are never added together
 * (what you owe and what you are owed are not one number), so on "Todas" the
 * two figures sit side by side and a single tab shows only its own.
 */
function PayablesSummary({ view, payables }: { view: PayableView; payables: Payable[] }) {
  const owed = <Money value={remainingTotal(payables, "debt")} tone="neutral" />;
  const toReceive = <Money value={remainingTotal(payables, "receivable")} tone="neutral" />;
  if (view === "all") {
    return (
      <SummaryStrip
        className="payables-summary"
        label={payableVocabulary.debt.summaryLabel}
        value={owed}
        items={[{ label: payableVocabulary.receivable.summaryLabel, value: toReceive }]}
      />
    );
  }
  return (
    <SummaryStrip
      className="payables-summary"
      label={payableVocabulary[view].summaryLabel}
      value={view === "debt" ? owed : toReceive}
    />
  );
}

export function PayablesPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const kindParam = searchParams.get("kind");
  const filter: PayableView = kindParam === "debt" || kindParam === "receivable" ? kindParam : "all";
  const payables = usePayables(filter === "all" ? null : filter);
  const navigate = useNavigate();
  const feedback = useFeedback();
  const [search, setSearch] = useState("");
  const [sort, setSort] = useState<PayableSort>("recent");
  const [formOpen, setFormOpen] = useState(false);
  const [formKind, setFormKind] = useState<PayableKind>("debt");
  const [saveError, setSaveError] = useState<string | null>(null);

  const setFilter = (value: PayableView) => {
    if (value === "all") {
      searchParams.delete("kind");
    } else {
      searchParams.set("kind", value);
    }
    setSearchParams(searchParams, { replace: true });
  };

  const openCreate = (kind: PayableKind) => {
    setFormKind(kind);
    setSaveError(null);
    setFormOpen(true);
  };

  const closeForm = () => setFormOpen(false);

  const submit = async (write: {
    name: string;
    total_amount: string;
    initial_remaining_amount: string | null;
  }) => {
    setSaveError(null);
    try {
      await payables.createPayable({
        kind: formKind,
        name: write.name,
        total_amount: write.total_amount,
        initial_remaining_amount: write.initial_remaining_amount,
      });
      setFormOpen(false);
      feedback.success(formKind === "debt" ? "Dívida criada" : "Conta a receber criada");
    } catch (error) {
      setSaveError(errorMessage(error, "Não foi possível salvar a pendência."));
    }
  };

  const visible = arrange(payables.payables, search, sort);
  const nothingYet = payables.payables.length === 0;
  // A failed load with nothing to show is not "nothing yet": it only says it failed.
  const loadFailed = payables.error !== null && nothingYet;

  const createOptions = [
    {
      key: "debt",
      label: "Nova dívida",
      description: "Valor que você precisa pagar",
      icon: payableVocabulary.debt.icon,
      onClick: () => openCreate("debt"),
    },
    {
      key: "receivable",
      label: "Nova conta a receber",
      description: "Valor que outra pessoa precisa pagar a você",
      icon: payableVocabulary.receivable.icon,
      onClick: () => openCreate("receivable"),
    },
  ];

  // "Nothing yet" says where to start; "no results" only says the search
  // missed. On "Todas" the title row's own "Nova pendência" is the way out, so
  // no second button repeats it; a kind's tab offers its own, more specific one.
  const empty = nothingYet ? (
    <EmptyState
      title={filter === "all" ? "Nenhuma pendência ainda" : payableVocabulary[filter].emptyTitle}
      hint="Registre o que você deve ou tem a receber para acompanhar o quanto já foi quitado."
      action={
        filter === "all" ? undefined : (
          <Button onClick={() => openCreate(filter)}>{payableVocabulary[filter].newTitle}</Button>
        )
      }
    />
  ) : (
    <EmptyState title="Nada encontrado para essa busca" hint="Tente outro nome." />
  );

  return (
    <Page
      title="Pendências"
      width="narrow"
      compactMobileHeader
      actions={<CreateActionMenu label="Nova pendência" shortLabel="Nova" options={createOptions} />}
      tabs={<PageTabs label="Tipo de pendência" options={viewOptions} value={filter} onChange={setFilter} />}
    >
      {payables.error && (
        <Alert
          type="error"
          showIcon
          message="Não foi possível carregar as pendências"
          action={<Button onClick={() => payables.refetch()}>Tentar novamente</Button>}
          style={{ marginBottom: 16 }}
        />
      )}

      {!payables.isLoading && !nothingYet && <PayablesSummary view={filter} payables={payables.payables} />}

      {!nothingYet && (
        <ListToolbar
          label="Controles das pendências"
          start={
            <SearchField
              id="payables-search"
              label="Buscar pendências"
              placeholder="Buscar por nome…"
              value={search}
              onChange={setSearch}
            />
          }
          end={<SortSelect id="payables-sort" value={sort} options={sortOptions} onChange={setSort} />}
        />
      )}

      {!loadFailed && (
        <DataCard flush className="payables-card">
          <PayableList
            payables={visible}
            isLoading={payables.isLoading}
            empty={empty}
            showKind={filter === "all"}
            onOpen={(payable) => navigate(payableDetailPath(payable))}
          />
        </DataCard>
      )}
      <PayableForm
        kind={formKind}
        open={formOpen}
        payable={null}
        submitting={payables.isSaving}
        submitError={saveError}
        onSubmit={submit}
        onCancel={closeForm}
      />
    </Page>
  );
}
