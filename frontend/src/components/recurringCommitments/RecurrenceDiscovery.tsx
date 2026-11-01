import { useEffect, useId, useState, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { Category } from "../../api/contracts";
import type { RecurrenceSuggestion } from "../../api/contracts/recurrenceSuggestions";
import { acceptRecurrenceSuggestions, discoverRecurrences, dismissRecurrenceSuggestion } from "../../api/recurringCommitments";
import { invalidateAfterAutomationRuleChange, invalidateAfterRecurringCommitmentChange } from "../../api/queryKeys";
import { useTranslation } from "../../i18n/context";
import { errorMessage } from "../../presentation/errors";
import { sumDecimals } from "../../presentation/decimal";
import { formatBRL } from "../../presentation/money";
import { recurrenceScheduleLabel, recurrenceScheduleSentence, recurringCommitmentKindName } from "../../presentation/recurringCommitmentLabels";
import { BottomActionBar, DataCard, DataCardSummary, EmptyState, ResponsiveList } from "../layout";
import { Money } from "../shared/Money";
import { StatusTag } from "../shared/StatusTag";
import { useCompactScreen } from "../shared/useCompactScreen";
import { Alert, Button, Checkbox, Skeleton, Table } from "../ui";
import type { ColumnsType } from "../ui/Table";
import { RedoOutlined } from "../ui/icons";
import { SuggestionReviewDrawer, type Reviewed } from "./SuggestionReviewDrawer";

/** What the monthly candidates of one kind add up to; annual ones do not happen every month. */
function monthlyTotal(candidates: RecurrenceSuggestion[], kind: "income" | "expense"): string | null {
  const amounts = candidates.filter((c) => c.draft.cadence === "monthly" && c.draft.kind === kind).map((c) => c.draft.amount);
  return amounts.length > 0 ? sumDecimals(amounts) : null;
}

const direction = (candidate: RecurrenceSuggestion) => (candidate.draft.kind === "income" ? "inflow" : "outflow");

/**
 * The candidates found in the history, shaped like the Recorrências list
 * itself: a table from `xl`, stacked rows below. "Revisar" (or the row) opens
 * the review drawer; the selection checkbox beside it is only enabled once the
 * candidate was reviewed, and creating (one, or the selected ones) never
 * happens without that review. The summary says so while nothing is reviewed.
 *
 * `onBottomBarChange` tells the page when the phone's bulk bar is showing, so
 * the page can make room for it (`Page` `hasBottomActionBar`).
 */
export function RecurrenceDiscovery({ categories, onBottomBarChange }: { categories: Category[]; onBottomBarChange?: (visible: boolean) => void }) {
  const { t, language } = useTranslation();
  const compact = useCompactScreen();
  const headingId = useId();
  const client = useQueryClient();
  const query = useQuery({ queryKey: ["recurrence-suggestions"], queryFn: ({ signal }) => discoverRecurrences(signal), staleTime: 0 });
  const [reviews, setReviews] = useState<Record<string, Reviewed>>({});
  const [selected, setSelected] = useState<string[]>([]);
  const [editing, setEditing] = useState<RecurrenceSuggestion | null>(null);
  const [busy, setBusy] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [message, setMessage] = useState<string | null>(null);
  const [hidden, setHidden] = useState<string[]>([]);
  const candidates = (query.data ?? []).filter((c) => !hidden.includes(c.key));
  const reviewedKeys = candidates.filter((c) => reviews[c.key]).map((c) => c.key);
  const showBottomBar = compact && reviewedKeys.length > 0;
  useEffect(() => {
    onBottomBarChange?.(showBottomBar);
    return () => onBottomBarChange?.(false);
  }, [showBottomBar, onBottomBarChange]);
  const reviewed = (key: string, value: Reviewed | null) => {
    if (value) setErrors((current) => { const next = { ...current }; delete next[key]; return next; });
    setReviews((current) => { const next = { ...current }; if (value) next[key] = value; else delete next[key]; return next; });
    if (!value) setSelected((current) => current.filter((id) => id !== key));
  };
  const accept = async (keys: string[]) => {
    if (busy) return;
    setBusy(true);
    try {
      const results = await acceptRecurrenceSuggestions(keys.map((key) => reviews[key].approval));
      const created = results.filter((r) => r.status === 201).map((r) => r.key);
      setHidden((current) => [...current, ...created]);
      setSelected((current) => current.filter((key) => !created.includes(key)));
      setErrors((current) => ({ ...current, ...Object.fromEntries(results.filter((r) => r.status !== 201).map((r) => [r.key, r.error])) }));
      for (const result of results) if (result.status !== 201) reviewed(result.key, null);
      if (created.includes(editing?.key ?? "")) setEditing(null);
      setMessage(t("suggestionCreatedCount", { count: created.length }));
      await Promise.all([invalidateAfterRecurringCommitmentChange(client), invalidateAfterAutomationRuleChange(client, { transactionsChanged: false }), query.refetch()]);
    } catch (error) {
      setErrors((current) => ({ ...current, ...Object.fromEntries(keys.map((key) => [key, errorMessage(error, t("recurrenceSaveFailed"))])) }));
    } finally { setBusy(false); }
  };
  const dismiss = async (key: string) => {
    if (busy) return;
    setBusy(true);
    try { await dismissRecurrenceSuggestion(key); setHidden((current) => [...current, key]); reviewed(key, null); if (editing?.key === key) setEditing(null); await query.refetch(); }
    catch (error) { setErrors((current) => ({ ...current, [key]: errorMessage(error, t("recurrenceSaveFailed")) })); }
    finally { setBusy(false); }
  };
  const refresh = () => { setReviews({}); setSelected([]); setEditing(null); setErrors({}); void query.refetch(); };

  const categoryName = (id: string) => id === "" ? "—" : categories.find((category) => category.id === id)?.name ?? t("removedCategory");
  const evidenceCount = (candidate: RecurrenceSuggestion) => t(candidate.evidence.length === 1 ? "transactionCountOne" : "transactionCountMany", { count: candidate.evidence.length });
  const variable = (candidate: RecurrenceSuggestion) => candidate.amount_kind === "variable";
  const range = (candidate: RecurrenceSuggestion) => t("suggestionAmountRangeShort", { min: formatBRL(candidate.minimum_amount), max: formatBRL(candidate.maximum_amount) });
  const openLabel = (candidate: RecurrenceSuggestion) => t("reviewNamedSuggestion", { name: candidate.draft.name });
  // Only states worth a tag: a lower confidence, and a review already done.
  const tags = (candidate: RecurrenceSuggestion, cue?: ReactNode): ReactNode => {
    const medium = candidate.confidence === "medium";
    const done = !!reviews[candidate.key];
    if (!medium && !done && !cue) return undefined;
    return <span className="recurrence-candidate-tags">
      {medium && <StatusTag tone="warning">{t("suggestionMediumConfidence")}</StatusTag>}
      {done && <StatusTag tone="success">{t("suggestionReviewedTag")}</StatusTag>}
      {cue}
    </span>;
  };
  const select = (candidate: RecurrenceSuggestion) => <span className="recurrence-candidate-select">
    <Checkbox aria-label={t("selectSuggestion", { name: candidate.draft.name })} disabled={busy || !reviews[candidate.key]} checked={selected.includes(candidate.key)} onChange={(e) => setSelected((current) => e.target.checked ? [...current, candidate.key] : current.filter((id) => id !== candidate.key))} />
  </span>;

  const bulkActions = <>
    <Button className="recurrence-bulk-action" disabled={busy || reviewedKeys.length === 0} onClick={() => setSelected(reviewedKeys)}>{t("selectReviewed")}</Button>
    <Button className="recurrence-bulk-action" type="primary" disabled={selected.length === 0} loading={busy} onClick={() => void accept(selected)}>{t("createSelectedSuggestions", { count: selected.length })}</Button>
  </>;
  const outflow = monthlyTotal(candidates, "expense");
  const inflow = monthlyTotal(candidates, "income");
  const summary = candidates.length > 0 && <DataCardSummary label={t("suggestionsSummaryLabel")} busy={query.isFetching} note={reviewedKeys.length === 0 ? t("suggestionSelectionHint") : undefined}>
    <div className="recurrence-discovery-summary">
      <p className="recurrence-discovery-totals">
        <span className="recurrence-discovery-count">{t(candidates.length === 1 ? "suggestionCountOne" : "suggestionCountMany", { count: candidates.length })}</span>
        {outflow !== null && <span className="recurrence-discovery-figure">{t("suggestionMonthlyOutflow")} <Money value={outflow} currencyCode="BRL" tone="flow" direction="outflow" /></span>}
        {inflow !== null && !compact && <span className="recurrence-discovery-figure">{t("suggestionMonthlyInflow")} <Money value={inflow} currencyCode="BRL" tone="flow" direction="inflow" /></span>}
      </p>
      {!compact && <div className="recurrence-discovery-bulk">{bulkActions}</div>}
    </div>
  </DataCardSummary>;

  const failed = candidates.filter((c) => errors[c.key]);
  const columns: ColumnsType<RecurrenceSuggestion> = [
    { title: <span className="visually-hidden">{t("select")}</span>, key: "select", className: "recurrence-candidate-select-cell", render: (_, candidate) => select(candidate) },
    { title: t("name"), key: "name", render: (_, candidate) => <>
      <span className="recurrence-candidate-name">{candidate.draft.name}</span>
      {variable(candidate) && <span className="recurrence-candidate-range">{range(candidate)}</span>}
    </> },
    { title: t("type"), key: "kind", render: (_, candidate) => recurringCommitmentKindName(candidate.draft.kind, language) },
    { title: t("amount"), key: "amount", align: "right", render: (_, candidate) => <Money value={candidate.draft.amount} currencyCode="BRL" tone="flow" direction={direction(candidate)} size="row" /> },
    { title: t("category"), key: "category", render: (_, candidate) => categoryName(candidate.draft.category_id) },
    { title: t("recurrence"), key: "recurrence", render: (_, candidate) => recurrenceScheduleLabel(candidate.draft.cadence, candidate.draft.day_of_month, candidate.draft.month_of_year, language) },
    { title: t("suggestionEvidenceHeader"), key: "evidence", render: (_, candidate) => evidenceCount(candidate) },
    { title: <span className="visually-hidden">{t("status")}</span>, key: "status", render: (_, candidate) => tags(candidate) },
    // The visible way in: the row click is only a shortcut.
    { title: <span className="visually-hidden">{t("reviewSuggestion")}</span>, key: "review", align: "right", render: (_, candidate) => <Button size="small" aria-label={openLabel(candidate)} onClick={() => setEditing(candidate)}>{t("reviewSuggestion")}</Button> },
  ];

  return <section className="recurrence-discovery" aria-labelledby={headingId}>
    <header className="recurrence-discovery-header">
      <h2 id={headingId}>{t("discoverRecurrences")}</h2>
      <Button icon={<RedoOutlined aria-hidden="true" />} aria-label={t("refreshRecurrenceSuggestions")} disabled={busy} loading={query.isFetching} onClick={refresh}>{compact ? t("refreshRecurrenceSuggestionsShort") : t("refreshRecurrenceSuggestions")}</Button>
      <p>{t("suggestionApprovalHint")}</p>
    </header>
    <DataCard flush className="recurrences-card recurrence-discovery-card" summary={summary || undefined}>
      {(message || query.isError || failed.length > 0) && <div className="recurrence-discovery-notices">
        {message && <Alert type="success" showIcon message={message} />}
        {query.isError && <Alert type="error" showIcon message={t("suggestionsFailed")} action={<Button onClick={() => void query.refetch()}>{t("retry")}</Button>} />}
        {failed.map((candidate) => <Alert key={candidate.key} type="error" showIcon message={<><strong>{candidate.draft.name}</strong>: {errors[candidate.key]}</>} />)}
      </div>}
      <ResponsiveList
        label={t("suggestionsListLabel")}
        items={candidates}
        getKey={(candidate) => candidate.key}
        isLoading={query.isPending}
        loading={<div className="recurrences-loading" role="status">
          <p className="recurrence-discovery-loading">{t("suggestionsLoading")}</p>
          <Skeleton active paragraph={{ rows: 3 }} />
        </div>}
        empty={query.isError ? <></> : <EmptyState title={t("suggestionsEmpty")} hint={t("suggestionsEmptyHint")} />}
        stackBelow="xl"
        row={(candidate) => ({
          title: candidate.draft.name,
          meta: [
            recurringCommitmentKindName(candidate.draft.kind, language),
            recurrenceScheduleSentence(candidate.draft.cadence, candidate.draft.day_of_month, candidate.draft.month_of_year, language),
            // The amount's range tells more than the evidence count when the figure is an average.
            variable(candidate) ? range(candidate) : evidenceCount(candidate),
          ].join(" · "),
          trailing: <Money value={candidate.draft.amount} currencyCode="BRL" tone="flow" direction={direction(candidate)} />,
          // The row is one tap target; the cue says so, as it has no button of its own.
          status: tags(candidate, <span className="recurrence-candidate-cue" aria-hidden="true">{t("reviewSuggestion")}</span>),
          onClick: () => setEditing(candidate),
          ariaLabel: openLabel(candidate),
          leading: select(candidate),
        })}
        wide={<Table<RecurrenceSuggestion>
          className="recurrence-candidates-table"
          aria-label={t("suggestionsListLabel")}
          columns={columns}
          dataSource={candidates}
          rowKey="key"
          pagination={false}
          // A click on the row opens the review; the name button is the keyboard way, the checkbox keeps its own meaning.
          onRow={(candidate) => ({
            className: "recurrence-candidate-row",
            onClick: (event) => {
              if (!event.currentTarget.contains(event.target as HTMLElement)) return;
              if ((event.target as HTMLElement).closest("button, a, label, input")) return;
              setEditing(candidate);
            },
          })}
        />}
      />
    </DataCard>
    {showBottomBar && <BottomActionBar>{bulkActions}</BottomActionBar>}
    {editing && <SuggestionReviewDrawer key={editing.key} candidate={editing} categories={categories} saved={reviews[editing.key]} error={errors[editing.key]} busy={busy} onReviewed={(r) => reviewed(editing.key, r)} onAccept={() => void accept([editing.key])} onDismiss={() => void dismiss(editing.key)} onClose={() => setEditing(null)} />}
  </section>;
}
