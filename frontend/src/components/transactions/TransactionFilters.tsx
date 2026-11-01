import { useTranslation } from "../../i18n/context";
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
  const { t, language } = useTranslation();
  const accountOptions = useMemo<FilterOption[]>(
    () =>
      facets?.accounts.map((account) => ({
        value: account.id,
        label: [account.name, account.institution].filter(Boolean).join(" · ") || account.id,
      })) ?? [],
    [facets],
  );
  const categoryOptions = useMemo(() => categoryFilterOptions(facets?.categories, language), [facets, language]);
  // The phone shares one row between search, filters and grouping: a short placeholder fits.
  const compact = useCompactScreen();

  const config = useMemo<FilterConfig<Filters>[]>(
    () => [
      {
        key: "description",
        label: t("description"),
        type: "text",
        placement: "main",
        placeholder: compact ? t("search") : t("searchTransactions"),
        debounceMs: 300,
        formatActive: (value) => t("searchTerm", { value: String(value) }),
      },
      {
        key: "classification",
        label: t("movement"),
        type: "segmented",
        placement: "advanced",
        // Shown by the quick toggles under the toolbar instead.
        hideChip: true,
        options: [
          { value: "all", label: t("allFeminine") },
          { value: "inflow", label: t("inflows") },
          { value: "outflow", label: t("outflows") },
        ],
        formatActive: (value) =>
          t(value === "inflow" ? "incomeSingular" : value === "outflow" ? "expenseSingular" : "unclassifiedFeminine"),
      },
      {
        key: "credit_card",
        label: t("creditCard"),
        type: "boolean",
        placement: "advanced",
        formatActive: () => t("creditCard"),
      },
      {
        key: "card_balance",
        // Only what makes up the open bill of each card (see
        // transactions.creditCardTransactionTotalAt) — not future ones.
        label: t("currentBillCapitalized"),
        type: "boolean",
        placement: "advanced",
        formatActive: () => t("currentBillCapitalized"),
      },
      {
        key: "origin",
        label: t("origin"),
        type: "segmented",
        placement: "advanced",
        options: [{ value: "manual", label: "Manual" }],
        formatActive: () => t("originManual"),
      },
      {
        key: "source_provider",
        label: t("origin"),
        type: "segmented",
        placement: "advanced",
        options: [
          { value: "all", label: t("allFeminine") },
          { value: "file", label: t("file") },
          { value: "pluggy", label: t("bank") },
        ],
        formatActive: (value) => value === "file" ? t("originFile") : t("originBank"),
      },
      {
        key: "account_ids",
        label: t("account"),
        type: "multiselect",
        placement: "advanced",
        placeholder: t("allAccounts"),
        options: accountOptions,
        formatActive: chipSummary(t("accountCount", { count: "" }).trim(), t("account")),
      },
      {
        key: "category_ids",
        label: t("category"),
        type: "multiselect",
        placement: "advanced",
        placeholder: t("allCategories"),
        options: categoryOptions,
        optionRender: renderCategoryOption,
        formatActive: chipSummary(t("categoryCount", { count: "" }).trim(), t("category")),
      },
      {
        key: "uncategorized",
        label: t("uncategorized"),
        type: "boolean",
        placement: "advanced",
        hideChip: true,
        formatActive: () => t("uncategorized"),
      },
      {
        key: "amount_min",
        secondaryKey: "amount_max",
        label: t("amount"),
        type: "value-range",
        placement: "advanced",
        formatActive: (_value, values) =>
          values.amount_min && values.amount_max
            ? t("amountBetween", { min: formatBRL(values.amount_min), max: formatBRL(values.amount_max) })
            : values.amount_min
              ? t("amountFrom", { min: formatBRL(values.amount_min) })
              : t("amountUpTo", { max: formatBRL(values.amount_max ?? "0") }),
      },
    ],
    [accountOptions, categoryOptions, compact, t],
  );

  const listProps = {
    loading: facetsLoading,
    error: facetsError,
    onRetry: onRetryFacets,
  };

  return (
    <ConfigurableFilters
      label={t("transactionFilters")}
      end={end}
      shortcuts={
        <QuickFilters
          label={t("filterShortcuts")}
          items={[
            {
              key: "inflow",
              label: t("inflows"),
              pressed: applied.classification === "inflow",
              onToggle: () =>
                onApply({ ...applied, classification: applied.classification === "inflow" ? null : "inflow" }),
            },
            {
              key: "outflow",
              label: t("outflows"),
              pressed: applied.classification === "outflow",
              onToggle: () =>
                onApply({ ...applied, classification: applied.classification === "outflow" ? null : "outflow" }),
            },
            {
              key: "uncategorized",
              label: t("uncategorized"),
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
                  {t("retry")}
                </Button>
              }
            />
          )}
          <FilterSection title={t("movement")}>
            <SegmentedControl<Movement>
              id="filter-classification"
              label={t("movement")}
              value={draft.classification === "inflow" || draft.classification === "outflow" ? draft.classification : "all"}
              options={[
                { value: "all", label: t("allFeminine") },
                { value: "inflow", label: t("inflows") },
                { value: "outflow", label: t("outflows") },
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
                {t("creditCard")}
              </Checkbox>
              <Checkbox
                className="filter-checkbox"
                checked={draft.card_balance === true}
                onChange={(event) => set("card_balance", event.target.checked)}
              >
                {t("currentBillCapitalized")}
              </Checkbox>
            </div>
          </FilterSection>

          <FilterSection title={t("organization")}>
            <SegmentedControl<ImportSource>
              id="filter-source_provider"
              label={t("origin")}
              value={draft.origin === "manual" ? "manual" : draft.source_provider === "file" || draft.source_provider === "pluggy" ? draft.source_provider : "all"}
              options={[
                { value: "all", label: t("allFeminine") },
                { value: "file", label: t("file") },
                { value: "pluggy", label: t("bank") },
                { value: "manual", label: "Manual" },
              ]}
              onChange={(value) => {
                set("origin", value === "manual" ? "manual" : null);
                set("source_provider", value === "file" || value === "pluggy" ? value : null);
              }}
            />
            <FilterField id="filter-account_ids" label={t("account")}>
              <FilterCombobox
                multiple
                id="filter-account_ids"
                label={t("accountList")}
                placeholder={t("allAccounts")}
                searchPlaceholder={t("searchAccount")}
                emptyText={t("noAccountsInPeriod")}
                countLabel={(count) => (t(count === 1 ? "oneAccountSelected" : "manyAccountsSelected", { count }))}
                options={accountOptions}
                value={draft.account_ids}
                onChange={(value) => set("account_ids", value)}
                {...listProps}
              />
            </FilterField>
            <div className="filter-field-group">
              <FilterField id="filter-category_ids" label={t("category")}>
                <FilterCombobox
                  multiple
                  id="filter-category_ids"
                  label={t("categories")}
                  placeholder={t("allCategories")}
                  searchPlaceholder={t("searchCategory")}
                  emptyText={t("noCategoriesInPeriod")}
                  countLabel={(count) =>
                    t(count === 1 ? "oneCategorySelected" : "manyCategoriesSelected", { count })
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
                {t("uncategorized")}
              </Checkbox>
            </div>
          </FilterSection>

          <FilterSection title={t("amount")}>
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
