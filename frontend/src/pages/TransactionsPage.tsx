import { ReportValuation } from "../components/shared/ReportValuation";
import { Alert, Button, Skeleton } from "../components/ui";
import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";

import {
  isUuid,
  type ManualTransactionWrite,
  type TransactionFilters,
  type TransactionGrouping,
  type TransactionQueryResult,
} from "../api/contracts";
import { filtersToSearchParams, listFromSearchParams } from "../components/filters/filterUrl";
import { PeriodNavigator } from "../components/filters/PeriodNavigator";
import { DataCard, EmptyState, GroupBySelect, Page, PageAction } from "../components/layout";
import { useCompactScreen } from "../components/shared/useCompactScreen";
import { useFeedback } from "../components/shared/useFeedback";
import { useRowLayout } from "../components/transactions/rowLayout";
import { ManualTransferForm } from "../components/transactions/ManualTransferForm";
import { ManualTransactionForm } from "../components/transactions/ManualTransactionForm";
import { TransactionPanel } from "../components/transactions/TransactionPanel";
import { TransactionFilters as TransactionFilterBar } from "../components/transactions/TransactionFilters";
import { TransactionGroup } from "../components/transactions/TransactionGroup";
import { TransactionSummaryBar } from "../components/transactions/TransactionSummaryBar";
import { useRowFocusReturn } from "../components/transactions/useRowFocusReturn";
import { useSelectedTransaction } from "../components/transactions/useSelectedTransaction";
import { useTransactionPanelWrites } from "../components/transactions/useTransactionPanelWrites";
import { useAccounts } from "../hooks/useAccounts";
import { useCategories } from "../hooks/useCategories";
import { isValidPeriod, usePeriod, type Period } from "../hooks/usePeriod";
import { useTimezone } from "../hooks/useTimezone";
import { currentMonthFilters, useTransactions } from "../hooks/useTransactions";
import { manualTransactionErrorMessage } from "../presentation/manualTransactionErrors";

type VisibleGrouping = Exclude<TransactionGrouping, "year">;
const visibleGroupings: VisibleGrouping[] = ["none", "day", "week", "month"];
const classifications = ["inflow", "outflow", "unclassified"] as const;

/**
 * A link may carry its own window (`?period=all`, or an explicit
 * `date_from`/`date_to`). When it does, that window wins over the remembered
 * page context; otherwise the page opens on the period the user last chose
 * anywhere in the app.
 */
function periodFromUrl(searchParams: URLSearchParams): Period | null {
  if (searchParams.get("period") === "all") return { from: null, to: null };
  const from = searchParams.get("date_from");
  const to = searchParams.get("date_to");
  if (from === null || to === null) return null;
  if (!/^\d{4}-\d{2}-\d{2}$/.test(from) || !/^\d{4}-\d{2}-\d{2}$/.test(to)) return null;
  const period = { from, to };
  return isValidPeriod(period) ? period : null;
}

function initialState(searchParams: URLSearchParams, period: Period, timezone: string): {
  filters: TransactionFilters;
  groupBy: VisibleGrouping;
  page: number;
} {
  const defaults = currentMonthFilters(timezone);
  const value = (key: keyof TransactionFilters) => searchParams.get(key);
  // The plural keys are what the page writes; the singular ones keep links
  // minted before filters accepted several values working.
  const list = (plural: keyof TransactionFilters, singular: string) =>
    listFromSearchParams(searchParams, plural, singular);
  const classification = value("classification");
  const amountPattern = /^(0|[1-9]\d*)(\.\d+)?$/;
  const amountMin = value("amount_min");
  const amountMax = value("amount_max");
  const group = searchParams.get("group");
  const requestedPage = Number(searchParams.get("page"));
  return {
    filters: {
      ...defaults,
      origin: value("origin") === "manual" || value("origin") === "synced"
        ? value("origin") as "manual" | "synced" : null,
      card_balance: value("card_balance") === "true" ? true : null,
      source_provider: value("source_provider") === "file" || value("source_provider") === "pluggy"
        ? value("source_provider") as "file" | "pluggy" : null,
      credit_card: value("credit_card") === "true" ? true : null,
      date_from: period.from,
      date_to: period.to,
      description: value("description"),
      account_ids: list("account_ids", "account_id").filter(isUuid),
      category_ids: list("category_ids", "category_id").filter(isUuid),
      classification: classifications.includes(
        classification as (typeof classifications)[number],
      )
        ? (classification as TransactionFilters["classification"])
        : null,
      amount_min: amountMin && amountPattern.test(amountMin) ? amountMin : null,
      amount_max: amountMax && amountPattern.test(amountMax) ? amountMax : null,
      uncategorized: value("uncategorized") === "true" ? true : null,
    },
    groupBy: visibleGroupings.includes(group as VisibleGrouping)
      ? (group as VisibleGrouping)
      : "week",
    page: Number.isInteger(requestedPage) && requestedPage > 0 ? requestedPage : 1,
  };
}

function ResultsSkeleton() {
  return (
    <div className="transaction-results-skeleton" role="status" aria-label="Carregando transações">
      <Skeleton active paragraph={{ rows: 5 }} />
      <span className="visually-hidden">Carregando transações…</span>
    </div>
  );
}

export function TransactionsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const timezone = useTimezone();
  const { period, setPeriod, presets } = usePeriod(() => periodFromUrl(searchParams));
  const [state] = useState(() => initialState(searchParams, period, timezone));
  const [initialFilters] = useState(() => currentMonthFilters(timezone));
  const [filters, setFilters] = useState<TransactionFilters>(state.filters);
  const [groupBy, setGroupBy] = useState<VisibleGrouping>(state.groupBy);
  const [page, setPage] = useState(state.page);
  const [facets, setFacets] =
    useState<TransactionQueryResult["available_filters"]>();
  const query = useTransactions(filters, groupBy, page);
  const categories = useCategories();
  const accounts = useAccounts();
  const compact = useCompactScreen();
  const feedback = useFeedback();
  const rowLayout = useRowLayout();
  const focusReturn = useRowFocusReturn();
  const data = query.data;
  // A snapshot, not a lookup in `data.items`: a write that moves the line out
  // of the current filter must not close the panel the person is working in.
  const { selected, select, patch } = useSelectedTransaction(data?.items);
  const writes = useTransactionPanelWrites({
    categories: categories.categories,
    onDeleted: () => select(null),
    onConfirmed: patch,
    panelOpen: selected !== null,
  });
  const { manualTransaction } = writes;
  const [manualFormOpen, setManualFormOpen] = useState(false);
  const [transferFormOpen, setTransferFormOpen] = useState(false);
  const [manualSaveError, setManualSaveError] = useState<string | null>(null);

  const openManualCreate = () => {
    setManualSaveError(null);
    setManualFormOpen(true);
  };
  const closeManualForm = () => setManualFormOpen(false);
  const createManualTransaction = async (write: ManualTransactionWrite) => {
    setManualSaveError(null);
    try {
      await manualTransaction.create(write);
      setManualFormOpen(false);
      feedback.success("Transação criada");
    } catch (error) {
      setManualSaveError(manualTransactionErrorMessage(error, "save"));
    }
  };
  const selectTransaction = (transactionId: string | null) => {
    writes.clearDeleteError();
    if (transactionId !== null) focusReturn.remember(transactionId);
    select(transactionId);
  };
  // Esc / × / ← on the panel: back to the row that opened it.
  const dismissPanel = () => {
    selectTransaction(null);
    focusReturn.restore();
  };

  useEffect(() => {
    if (data) setFacets(data.available_filters);
  }, [data]);

  // A stale `?page=N` past the last page would otherwise show an empty list.
  const totalPages = data?.page.total_pages;
  const hasItems = (data?.page.total_items ?? 0) > 0;
  useEffect(() => {
    if (hasItems && totalPages !== undefined && page > totalPages) setPage(totalPages);
  }, [hasItems, totalPages, page]);

  useEffect(() => {
    const params = filtersToSearchParams(filters);
    if (filters.date_from === null && filters.date_to === null) params.set("period", "all");
    if (groupBy !== "week") params.set("group", groupBy);
    if (page > 1) params.set("page", String(page));
    if (params.toString() !== searchParams.toString()) {
      setSearchParams(params, { replace: true });
    }
  }, [filters, groupBy, page, searchParams, setSearchParams]);

  const apply = (next: TransactionFilters) => {
    setFilters(next);
    setPage(1);
    select(null);
  };
  // The period is page context, not a list filter: it changes the list and
  // the totals together and is remembered for the next page that reads it.
  const applyPeriod = (from: string | null, to: string | null) => {
    setPeriod(from, to);
    apply({ ...filters, date_from: from, date_to: to });
  };
  const resultsRef = useRef<HTMLDivElement>(null);
  const [focusResults, setFocusResults] = useState(false);
  const changePage = (step: 1 | -1) => {
    setPage((current) => current + step);
    select(null);
    // The button that was pressed goes away with the old page (or is disabled
    // by the new one): once the new page is in, the keyboard moves to its top.
    setFocusResults(true);
  };
  const pageNumber = data?.page.number;
  useEffect(() => {
    if (!focusResults || query.isFetching || pageNumber !== page) return;
    resultsRef.current?.focus();
    setFocusResults(false);
  }, [focusResults, query.isFetching, pageNumber, page]);
  const clear = () => apply({ ...initialFilters, date_from: filters.date_from, date_to: filters.date_to });
  const hasExtraFilters = Object.entries(filters).some(
    ([key, value]) => !key.startsWith("date_") && value !== null && !(Array.isArray(value) && value.length === 0),
  );
  const isInitialMonth =
    filters.date_from === initialFilters.date_from &&
    filters.date_to === initialFilters.date_to &&
    !hasExtraFilters;
  const rangeStart = data ? (data.page.number - 1) * data.page.size + 1 : 0;
  const rangeEnd = data
    ? Math.min(data.page.number * data.page.size, data.page.total_items)
    : 0;

  const createAction = <><PageAction label="Nova transação" shortLabel="Nova" onClick={openManualCreate} /><Button onClick={() => setTransferFormOpen(true)}>{compact ? "Transferir" : "Nova transferência"}</Button></>;

  return (
    <Page
      title="Transações"
      description="Acompanhe suas entradas, saídas e o resultado do período"
      className="transactions-page"
      compactMobileHeader
      context={
        <PeriodNavigator
          id="transactions-period"
          value={[filters.date_from, filters.date_to]}
          presets={presets}
          onChange={applyPeriod}
          reset={{ preset: "this-month", label: "Este mês" }}
          bare
        />
      }
      actions={createAction}
    >
      <TransactionFilterBar
        applied={filters}
        emptyValues={initialFilters}
        facets={data?.available_filters ?? facets}
        facetsLoading={query.isPending}
        facetsError={query.isError && !data && !facets ? "Não foi possível carregar as opções." : null}
        onRetryFacets={() => query.refetch()}
        lookupError={
          categories.error || accounts.error
            ? {
                message: "Não foi possível carregar todas as contas e categorias.",
                retry: () => {
                  if (categories.error) void categories.refetch();
                  if (accounts.error) void accounts.refetch();
                },
              }
            : null
        }
        onApply={apply}
        onClear={clear}
        end={
          <GroupBySelect
            id="transaction-grouping"
            value={groupBy}
            options={[
              { value: "none", label: "Sem agrupamento" },
              { value: "day", label: "Dia" },
              { value: "week", label: "Semana" },
              { value: "month", label: "Mês" },
            ]}
            onChange={(value: VisibleGrouping) => {
              setGroupBy(value);
              setPage(1);
              select(null);
            }}
          />
        }
      />

      <DataCard
        className="transactions-card"
        summary={
          data ? (
            <TransactionSummaryBar
              reporting={data.reporting}
              totalItems={data.page.total_items}
              totals={data.page.total_items === 0 ? [] : data.totals}
              busy={query.isFetching}
            />
          ) : query.isPending ? (
            <section className="data-card-summary" aria-label="Carregando resumo financeiro" aria-busy="true">
              <Skeleton active paragraph={{ rows: 1 }} title={false} />
            </section>
          ) : undefined
        }
      >
        {data && <ReportValuation valuation={data}>{null}</ReportValuation>}
        {query.isPending && <ResultsSkeleton />}
        {query.isError && !data && (
          <Alert
            type="error"
            showIcon
            message="Não foi possível carregar as transações"
            description={<Button onClick={() => query.refetch()}>Tentar novamente</Button>}
          />
        )}
        {query.isError && data && (
          <Alert
            type="warning"
            showIcon
            message="Dados possivelmente desatualizados"
            description={<Button onClick={() => query.refetch()}>Tentar novamente</Button>}
          />
        )}
        {writes.alerts(selected !== null)}

        {data && (
          <div
            ref={resultsRef}
            tabIndex={-1}
            role="region"
            aria-label="Resultados"
            className={`transaction-results ${query.isFetching ? "is-updating" : ""}`}
            aria-busy={query.isFetching}
          >
            {query.isFetching && !query.isPending && (
              <span className="visually-hidden" role="status">
                Atualizando resultados…
              </span>
            )}
            {data.page.total_items === 0 ? (
              // "Nothing yet" and "no results for this search" are different
              // situations and say different things.
              data.stored_total === 0 ? (
                <EmptyState
                  title="Nenhuma transação ainda"
                  hint="Importe um extrato, conecte um banco ou adicione uma transação manual."
                  action={<Button onClick={openManualCreate}>Adicionar transação</Button>}
                />
              ) : hasExtraFilters ? (
                <EmptyState
                  title="Nada encontrado para estes filtros"
                  hint="Tente outros termos ou remova algum filtro."
                  action={<Button onClick={clear}>Limpar filtros</Button>}
                />
              ) : (
                <EmptyState
                  title={isInitialMonth ? "Nenhuma transação neste mês" : "Nenhuma transação neste período"}
                  hint="Escolha outro período para ver mais."
                />
              )
            ) : (
              <>
                <div className="transaction-groups">
                  {data.groups.map((group) => (
                    <TransactionGroup
                      key={group.key}
                      group={group}
                      items={data.items.filter((item) => item.group_key === group.key)}
                      selectedId={selected?.id ?? null}
                      showAccount={filters.account_ids.length !== 1}
                      layout={rowLayout}
                      onSelect={selectTransaction}
                      onInclusion={(transactionId, target) =>
                        writes.inclusion.setInclusion({ transactionId, state: target })
                      }
                      pendingTransactionId={writes.inclusion.pendingTarget?.transactionId}
                    />
                  ))}
                </div>
                <footer className="transaction-pagination">
                  <span>
                    Exibindo {rangeStart.toLocaleString("pt-BR")}–
                    {rangeEnd.toLocaleString("pt-BR")} de{" "}
                    {data.page.total_items.toLocaleString("pt-BR")} transações
                  </span>
                  <nav aria-label="Paginação das transações">
                    <Button
                      disabled={data.page.number <= 1}
                      onClick={() => changePage(-1)}
                    >
                      Anterior
                    </Button>
                    <span>
                      Página {data.page.number} de {data.page.total_pages}
                    </span>
                    <Button
                      disabled={data.page.number >= data.page.total_pages}
                      onClick={() => changePage(1)}
                    >
                      Próxima
                    </Button>
                  </nav>
                </footer>
              </>
            )}
          </div>
        )}
      </DataCard>
      <TransactionPanel
        item={selected}
        categories={categories.categories}
        onClose={dismissPanel}
        {...writes.panelProps(selected)}
      />
      {transferFormOpen && <ManualTransferForm onClose={() => setTransferFormOpen(false)} onCreated={() => { setTransferFormOpen(false); feedback.success("Transferência criada"); }} />}
      <ManualTransactionForm
        open={manualFormOpen}
        accounts={accounts.accounts}
        categories={categories.categories}
        submitting={manualTransaction.isCreating}
        submitError={manualSaveError}
        onSubmit={createManualTransaction}
        onCancel={closeManualForm}
      />
    </Page>
  );
}
