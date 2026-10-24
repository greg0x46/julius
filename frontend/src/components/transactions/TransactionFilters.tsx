import { Alert, Button, Checkbox } from "../ui";
import { useMemo, type ReactNode } from "react";

import type {
  TransactionFilters as Filters,
  TransactionQueryResult,
} from "../../api/contracts";
import {
  ConfigurableFilters,
  type FilterConfig,
  type FilterOption,
} from "../filters/ConfigurableFilters";
import { FilterCombobox } from "../filters/FilterCombobox";
import { FilterField, FilterSection } from "../filters/FilterPanel";
import { MoneyRangeFilter } from "../filters/MoneyRangeFilter";
import { QuickFilters } from "../filters/QuickFilters";
import { SegmentedControl } from "../filters/SegmentedControl";
import { categoryFilterOptions, renderCategoryIcon } from "../../presentation/categoryLabels";
import { formatBRL } from "../../presentation/money";
import { classificationLabel } from "../../presentation/transactionStatus";
import { useCompactScreen } from "../shared/useCompactScreen";

type Movement = "all" | "inflow" | "outflow";
type ImportSource = "all" | "file" | "pluggy" | "manual";

/** The chip: one name, or how many were picked ("3 contas"). */
const chipSummary =
  (noun: string, fallback: string) =>
  (_value: unknown, _values: Filters, selected: FilterOption[]) =>
    selected.length === 0
      ? fallback
      : selected.length > 1
        ? `${selected.length} ${noun}`
        : selected[0]!.label;

const renderCategoryOption = (option: FilterOption) => (
  <span className="filter-option-with-icon">
    <span style={{ color: option.color }} aria-hidden="true">
      {renderCategoryIcon(option.icon)}
    </span>
    <span>{option.label}</span>
  </span>
);

/**
 * The collection controls of the transactions list. The period is not one
 * of them: it is the page's context and lives in the Page header, so the
 * values here only carry `date_from`/`date_to` through untouched.
 *
 * The toolbar (search, button, chips) and the panel's draft/apply cycle are
 * ConfigurableFilters'; what this component owns is which filters exist,
 * how they are grouped in the panel, and the transactions-specific rules
 * applied on the way out (an "all" movement is no filter, an unchecked box
 * is null).
 */
export function TransactionFilters({
  applied,
  emptyValues,
  facets,
  facetsLoading = false,
  facetsError = null,
  onRetryFacets,
  lookupError = null,
  onApply,
  onClear,
  end,
}: {
  applied: Filters;
  emptyValues?: Filters;
  facets: TransactionQueryResult["available_filters"] | undefined;
  /** The first query has not answered yet, so the option lists are unknown. */
  facetsLoading?: boolean;
  facetsError?: string | null;
  onRetryFacets?: () => void;
  /** Accounts or categories failed to load: the panel says so instead of listing fewer options in silence. */
  lookupError?: { message: string; retry: () => void } | null;
  onApply: (filters: Filters) => void;
  onClear: () => void;
  end?: ReactNode;
}) {
  const accountOptions = useMemo<FilterOption[]>(
    () =>
      facets?.accounts.map((account) => ({
        value: account.id,
        label: [account.name, account.institution].filter(Boolean).join(" · ") || account.id,
      })) ?? [],
    [facets],
  );
  const categoryOptions = useMemo(() => categoryFilterOptions(facets?.categories), [facets]);
  // The phone shares one row between search, filters and grouping: a short placeholder fits.
  const compact = useCompactScreen();

  const config = useMemo<FilterConfig<Filters>[]>(
    () => [
      {
        key: "description",
        label: "Descrição",
        type: "text",
        placement: "main",
        placeholder: compact ? "Buscar…" : "Buscar transações…",
        debounceMs: 300,
        formatActive: (value) => `Busca: ${String(value)}`,
      },
      {
        key: "classification",
        label: "Movimentação",
        type: "segmented",
        placement: "advanced",
        // Shown by the quick toggles under the toolbar instead.
        hideChip: true,
        options: [
          { value: "all", label: "Todas" },
          { value: "inflow", label: "Entradas" },
          { value: "outflow", label: "Saídas" },
        ],
        formatActive: (value) =>
          classificationLabel[value as "inflow" | "outflow" | "unclassified"],
      },
      {
        key: "credit_card",
        label: "Cartão de crédito",
        type: "boolean",
        placement: "advanced",
        formatActive: () => "Cartão de crédito",
      },
      {
        key: "card_balance",
        // Only what makes up the open bill of each card (see
        // transactions.creditCardTransactionTotalAt) — not future ones.
        label: "Fatura atual",
        type: "boolean",
        placement: "advanced",
        formatActive: () => "Fatura atual",
      },
      {
        key: "origin",
        label: "Origem",
        type: "segmented",
        placement: "advanced",
        options: [{ value: "manual", label: "Manual" }],
        formatActive: () => "Origem: manual",
      },
      {
        key: "source_provider",
        label: "Origem",
        type: "segmented",
        placement: "advanced",
        options: [
          { value: "all", label: "Todas" },
          { value: "file", label: "Arquivo" },
          { value: "pluggy", label: "Banco" },
        ],
        formatActive: (value) => value === "file" ? "Origem: arquivo" : "Origem: banco",
      },
      {
        key: "account_ids",
        label: "Conta",
        type: "multiselect",
        placement: "advanced",
        placeholder: "Todas as contas",
        options: accountOptions,
        formatActive: chipSummary("contas", "Conta"),
      },
      {
        key: "category_ids",
        label: "Categoria",
        type: "multiselect",
        placement: "advanced",
        placeholder: "Todas as categorias",
        options: categoryOptions,
        optionRender: renderCategoryOption,
        formatActive: chipSummary("categorias", "Categoria"),
      },
      {
        key: "uncategorized",
        label: "Sem categoria",
        type: "boolean",
        placement: "advanced",
        hideChip: true,
        formatActive: () => "Sem categoria",
      },
      {
        key: "amount_min",
        secondaryKey: "amount_max",
        label: "Valor",
        type: "value-range",
        placement: "advanced",
        formatActive: (_value, values) =>
          values.amount_min && values.amount_max
            ? `Valor: ${formatBRL(values.amount_min)} a ${formatBRL(values.amount_max)}`
            : values.amount_min
              ? `Valor: a partir de ${formatBRL(values.amount_min)}`
              : `Valor: até ${formatBRL(values.amount_max ?? "0")}`,
      },
    ],
    [accountOptions, categoryOptions, compact],
  );

  const listProps = {
    loading: facetsLoading,
    error: facetsError,
    onRetry: onRetryFacets,
  };

  return (
    <ConfigurableFilters
      label="Filtros de transações"
      end={end}
      shortcuts={
        <QuickFilters
          label="Atalhos de filtro"
          items={[
            {
              key: "inflow",
              label: "Entradas",
              pressed: applied.classification === "inflow",
              onToggle: () =>
                onApply({ ...applied, classification: applied.classification === "inflow" ? null : "inflow" }),
            },
            {
              key: "outflow",
              label: "Saídas",
              pressed: applied.classification === "outflow",
              onToggle: () =>
                onApply({ ...applied, classification: applied.classification === "outflow" ? null : "outflow" }),
            },
            {
              key: "uncategorized",
              label: "Sem categoria",
              pressed: applied.uncategorized === true,
              onToggle: () => onApply({ ...applied, uncategorized: applied.uncategorized === true ? null : true }),
            },
          ]}
        />
      }
      values={applied}
      emptyValues={emptyValues ?? applied}
      config={config}
      onApply={(next) =>
        onApply({
          ...next,
          description: next.description?.trim() || null,
          source_provider: (next.source_provider as string | null) === "all" ? null : next.source_provider,
          classification:
            (next.classification as string | null) === "all"
              ? null
              : next.classification,
          uncategorized: next.uncategorized || null,
          credit_card: next.credit_card || null,
          card_balance: next.card_balance || null,
        })
      }
      onClear={onClear}
      renderAdvanced={({ draft, set, error }) => (
        <>
          {lookupError && (
            <Alert
              type="warning"
              showIcon
              message={lookupError.message}
              action={
                <Button size="small" onClick={lookupError.retry}>
                  Tentar novamente
                </Button>
              }
            />
          )}
          <FilterSection title="Movimentação">
            <SegmentedControl<Movement>
              id="filter-classification"
              label="Movimentação"
              value={draft.classification === "inflow" || draft.classification === "outflow" ? draft.classification : "all"}
              options={[
                { value: "all", label: "Todas" },
                { value: "inflow", label: "Entradas" },
                { value: "outflow", label: "Saídas" },
              ]}
              onChange={(value) => set("classification", value === "all" ? null : value)}
            />
            {/* Card refinements of the movement: one quiet row, not two items. */}
            <div className="filter-checks filter-checks-inline">
              <Checkbox
                className="filter-checkbox"
                checked={draft.credit_card === true}
                onChange={(event) => set("credit_card", event.target.checked)}
              >
                Cartão de crédito
              </Checkbox>
              <Checkbox
                className="filter-checkbox"
                checked={draft.card_balance === true}
                onChange={(event) => set("card_balance", event.target.checked)}
              >
                Fatura atual
              </Checkbox>
            </div>
          </FilterSection>

          <FilterSection title="Organização">
            <SegmentedControl<ImportSource>
              id="filter-source_provider"
              label="Origem"
              value={draft.origin === "manual" ? "manual" : draft.source_provider === "file" || draft.source_provider === "pluggy" ? draft.source_provider : "all"}
              options={[
                { value: "all", label: "Todas" },
                { value: "file", label: "Arquivo" },
                { value: "pluggy", label: "Banco" },
                { value: "manual", label: "Manual" },
              ]}
              onChange={(value) => {
                set("origin", value === "manual" ? "manual" : null);
                set("source_provider", value === "file" || value === "pluggy" ? value : null);
              }}
            />
            <FilterField id="filter-account_ids" label="Conta">
              <FilterCombobox
                multiple
                id="filter-account_ids"
                label="Contas"
                placeholder="Todas as contas"
                searchPlaceholder="Buscar conta…"
                emptyText="Nenhuma conta no período"
                countLabel={(count) => (count === 1 ? "1 conta selecionada" : `${count} contas selecionadas`)}
                options={accountOptions}
                value={draft.account_ids}
                onChange={(value) => set("account_ids", value)}
                {...listProps}
              />
            </FilterField>
            <div className="filter-field-group">
              <FilterField id="filter-category_ids" label="Categoria">
                <FilterCombobox
                  multiple
                  id="filter-category_ids"
                  label="Categorias"
                  placeholder="Todas as categorias"
                  searchPlaceholder="Buscar categoria…"
                  emptyText="Nenhuma categoria no período"
                  countLabel={(count) =>
                    count === 1 ? "1 categoria selecionada" : `${count} categorias selecionadas`
                  }
                  options={categoryOptions}
                  optionRender={renderCategoryOption}
                  value={draft.category_ids}
                  onChange={(value) => set("category_ids", value)}
                  {...listProps}
                />
              </FilterField>
              <Checkbox
                className="filter-checkbox"
                checked={draft.uncategorized === true}
                onChange={(event) => set("uncategorized", event.target.checked)}
              >
                Sem categoria
              </Checkbox>
            </div>
          </FilterSection>

          <FilterSection title="Valor">
            <MoneyRangeFilter
              id="filter-amount"
              min={draft.amount_min}
              max={draft.amount_max}
              onChange={(min, max) => {
                set("amount_min", min);
                set("amount_max", max);
              }}
              error={error}
            />
          </FilterSection>
        </>
      )}
    />
  );
}
