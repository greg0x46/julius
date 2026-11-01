import { useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { Category, RecurringCommitmentWrite, RuleCondition } from "../../api/contracts";
import type { RecurrenceSuggestion, SuggestionApproval, SuggestionReview } from "../../api/contracts/recurrenceSuggestions";
import { acceptRecurrenceSuggestions, discoverRecurrences, dismissRecurrenceSuggestion, reviewRecurrenceSuggestion } from "../../api/recurringCommitments";
import { invalidateAfterAutomationRuleChange, invalidateAfterRecurringCommitmentChange } from "../../api/queryKeys";
import { useTranslation } from "../../i18n/context";
import type { MessageKey } from "../../i18n/messages";
import { calendar } from "../../presentation/calendar";
import { errorMessage } from "../../presentation/errors";
import { formatBRL, formatMoney } from "../../presentation/money";
import { recurringCommitmentCadenceName, recurringCommitmentKindName } from "../../presentation/recurringCommitmentLabels";
import { ruleConditionFieldName, ruleConditionOperatorName, summarizeConditions } from "../../presentation/ruleConditionLabels";
import { DataCard, EmptyState } from "../layout";
import { Alert, Button, Checkbox, Drawer, Flex } from "../ui";
import { useCompactScreen } from "../shared/useCompactScreen";
import { ConditionListEditor, type ConditionFieldConfig } from "../shared/ConditionListEditor";
import { RecurringCommitmentFields } from "./RecurringCommitmentForm";

const reasonKeys: Record<string, MessageKey> = {
  consistent_calendar: "suggestionCalendarReason", one_observation_per_period: "suggestionPeriodReason", fixed_amount: "suggestionFixedReason", variable_amount: "suggestionVariableReason", missing_months: "suggestionMissingReason", category_required: "suggestionCategoryReason", broad_description: "suggestionBroadReason", unrestricted_account: "suggestionAccountReason", cross_month_unsupported: "suggestionUnsupported", multiple_matches_per_occurrence: "suggestionMultipleReason", existing_overlap: "suggestionOverlapReason", existing_rule_or_scenario_overlap: "suggestionOverlapReason", possible_false_positives: "suggestionFalsePositiveReason", possible_false_positive: "suggestionFalsePositiveReason", conditions_failed: "suggestionConditionsFailed", manual_link_precedence: "suggestionManualReason", manually_taken: "suggestionManualReason", manual_detach: "suggestionManualReason", account_restriction: "suggestionIneligibleReason", ineligible_cash_movement: "suggestionIneligibleReason", no_occurrence_in_month: "suggestionIneligibleReason", missing_date_or_money: "suggestionIneligibleReason", conditions_and_month_match: "suggestionMatched",
};

type Reviewed = { approval: SuggestionApproval; preview: SuggestionReview };

export function RecurrenceDiscovery({ categories }: { categories: Category[] }) {
  const { t, language } = useTranslation();
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
    try { await dismissRecurrenceSuggestion(key); setHidden((current) => [...current, key]); reviewed(key, null); await query.refetch(); }
    catch (error) { setErrors((current) => ({ ...current, [key]: errorMessage(error, t("recurrenceSaveFailed")) })); }
    finally { setBusy(false); }
  };
  return <DataCard>
    <Flex vertical gap="middle">
      <h2>{t("discoverRecurrences")}</h2>
      {message && <p role="status">{message}</p>}
      {query.isPending && <p role="status">{t("suggestionsLoading")}</p>}
      {query.isError && <Alert type="error" message={t("suggestionsFailed")} action={<Button onClick={() => void query.refetch()}>{t("retry")}</Button>} />}
      {!query.isPending && !query.isError && candidates.length === 0 && <EmptyState title={t("suggestionsEmpty")} hint={t("suggestionsEmptyHint")} />}
      {candidates.length > 0 && <>
        <p>{t("suggestionApprovalHint")}</p>
        <Flex gap="small" wrap>
          <Button disabled={busy || !candidates.some((c) => reviews[c.key])} onClick={() => setSelected(candidates.filter((c) => reviews[c.key]).map((c) => c.key))}>{t("selectReviewed")}</Button>
          <Button type="primary" disabled={selected.length === 0} loading={busy} onClick={() => void accept(selected)}>{t("createSelectedSuggestions", { count: selected.length })}</Button>
        </Flex>
        {candidates.map((candidate) => <section key={candidate.key} className="recurrence-suggestion">
          <h3>{candidate.draft.name}</h3>
          <p>{recurringCommitmentKindName(candidate.draft.kind, language)} · {formatBRL(candidate.draft.amount)} · {recurringCommitmentCadenceName(candidate.draft.cadence, language)}{candidate.draft.month_of_year ? ` · ${candidate.draft.month_of_year}` : ""}</p>
          <p>{t("suggestionSchedule", { day: candidate.draft.day_of_month, date: candidate.draft.start_date })}</p>
          <p>{t(candidate.confidence === "high" ? "suggestionHighConfidence" : "suggestionMediumConfidence")}. {t("suggestionConfidenceHint")}</p>
          <p>{t("suggestionAmountRange", { min: formatBRL(candidate.minimum_amount), max: formatBRL(candidate.maximum_amount) })}</p>
          <ul>{candidate.reasons.map((reason) => <li key={reason}>{t(reasonKeys[reason] ?? "suggestionIneligibleReason")}</li>)}</ul>
          <details><summary>{t("suggestionEvidence")}</summary><ul>{candidate.evidence.map((row) => <li key={row.id}>{row.date} · {row.description} · {formatBRL(row.amount)}</li>)}</ul></details>
          {!reviews[candidate.key] && <p>{t("suggestionSelectionHint")}</p>}
          {errors[candidate.key] && <Alert type="error" message={errors[candidate.key]} />}
          <Flex gap="small" wrap align="center">
            <Checkbox aria-label={t("selectSuggestion", { name: candidate.draft.name })} disabled={busy || !reviews[candidate.key]} checked={selected.includes(candidate.key)} onChange={(e) => setSelected((current) => e.target.checked ? [...current, candidate.key] : current.filter((id) => id !== candidate.key))}>{t("select")}</Checkbox>
            <Button disabled={busy} onClick={() => setEditing(candidate)}>{t("reviewSuggestion")}</Button>
            <Button disabled={busy} onClick={() => void dismiss(candidate.key)}>{t("dismissSuggestion")}</Button>
            {reviews[candidate.key] && <Button disabled={busy} onClick={() => void accept([candidate.key])}>{t("createSuggestion")}</Button>}
          </Flex>
        </section>)}
      </>}
    </Flex>
    {editing && <SuggestionReviewDrawer key={editing.key} candidate={editing} categories={categories} saved={reviews[editing.key]} error={errors[editing.key]} busy={busy} onReviewed={(r) => reviewed(editing.key, r)} onAccept={() => void accept([editing.key])} onClose={() => setEditing(null)} />}
  </DataCard>;
}

function SuggestionReviewDrawer({ candidate, categories, saved, error, busy, onReviewed, onAccept, onClose }: {
  candidate: RecurrenceSuggestion; categories: Category[]; saved?: Reviewed; error?: string; busy: boolean;
  onReviewed: (r: Reviewed | null) => void; onAccept: () => void; onClose: () => void;
}) {
  const { t, language } = useTranslation();
  const compact = useCompactScreen();
  const [preview, setPreview] = useState(saved?.preview ?? null);
  const [conditions, setConditions] = useState<RuleCondition[] | undefined>(saved?.approval.conditions);
  const [write, setWrite] = useState(saved?.approval.draft ?? candidate.draft);
  const [acknowledge, setAcknowledge] = useState(saved?.approval.acknowledge ?? false);
  const [dirty, setDirty] = useState(!saved);
  const [loading, setLoading] = useState(false);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const version = useRef(0);
  const draft = saved?.approval.draft ?? candidate.draft;
  const invalidate = () => { version.current++; setDirty(true); setAcknowledge(false); onReviewed(null); };
  const eligible = !!saved && !!preview && !dirty && !preview.warnings.includes("cross_month_unsupported") && (!preview.needs_review || acknowledge);
  const publish = (next: SuggestionReview, nextWrite: RecurringCommitmentWrite, ack: boolean) => {
    if (!next.warnings.includes("cross_month_unsupported") && (!next.needs_review || ack)) onReviewed({ preview: next, approval: { key: candidate.key, draft: nextWrite, conditions: next.conditions, token: next.token, acknowledge: ack } });
  };
  const updatePreview = async (nextWrite: RecurringCommitmentWrite) => {
    const currentVersion = version.current;
    setLoading(true); setPreviewError(null); onReviewed(null);
    try {
      const next = await reviewRecurrenceSuggestion({ key: candidate.key, draft: nextWrite, conditions });
      if (version.current !== currentVersion) return;
      setPreview(next); setConditions(next.conditions); setWrite(nextWrite); setDirty(false); setAcknowledge(false);
      publish(next, nextWrite, false);
    } catch (e) { setPreviewError(errorMessage(e, t("recurrenceSaveFailed"))); }
    finally { setLoading(false); }
  };
  const textConfigs: ConditionFieldConfig[] = (["description", "account", "card"] as const).map((field) => ({ field, label: ruleConditionFieldName(field, language), operators: (["contains", "equals"] as const).map((operator) => ({ value: operator, label: ruleConditionOperatorName(operator, language) })) }));
  const configs: ConditionFieldConfig[] = [...textConfigs,
    { field: "amount", label: ruleConditionFieldName("amount", language), operators: [], pairOperator: "within_percent", pairFirstLabel: t("expectedAmount"), pairSecondLabel: t("tolerance"), pairSecondSuffix: "%", pairMin: 0 },
    { field: "day_of_month", label: ruleConditionFieldName("day_of_month", language), operators: [], pairOperator: "day_range", pairFirstLabel: t("fromDay"), pairSecondLabel: t("untilDay"), pairMin: 1, pairMax: 31 },
  ];
  return <Drawer open title={`${t("reviewSuggestion")}: ${candidate.draft.name}`} width={compact ? "100%" : 640} onClose={busy ? undefined : onClose} rootClassName="form-drawer" footer={<Flex gap="small" wrap>
    <Button disabled={busy || loading} htmlType="submit" form="suggestion-review-fields" loading={loading}>{t("updateSuggestionPreview")}</Button>
    <Button disabled={!eligible || busy || loading} onClick={onClose}>{t("suggestionDoneReview")}</Button>
    <Button type="primary" disabled={!eligible || loading} loading={busy} onClick={onAccept}>{t("createSuggestion")}</Button>
  </Flex>}>
    <Flex vertical gap="middle">
      <p>{t("suggestionCategoryHint")}</p>
      <div>
        <RecurringCommitmentFields formId="suggestion-review-fields" onDraftChange={invalidate} commitment={null} initialDraft={{ name: draft.name, kind: draft.kind, amount: draft.amount, categoryId: draft.category_id || null, accountId: draft.account_id ?? "", cadence: draft.cadence, dayOfMonth: draft.day_of_month, monthOfYear: draft.month_of_year, startDate: calendar(draft.start_date), endDate: draft.end_date ? calendar(draft.end_date) : null, isActive: draft.is_active }} categories={categories} submitting={loading || busy} submitError={previewError} onSubmit={(w) => void updatePreview(w)} />
      </div>
      <h3>{t("suggestionRuleConditions")}</h3>
      {conditions && <ConditionListEditor conditions={conditions} onChange={(c) => { invalidate(); setConditions(c); }} fieldConfigs={configs} />}
      {dirty && <p role="status">{t("suggestionDirty")}</p>}
      {error && <Alert type="error" message={error} />}
      {preview && <>
        <h3>{t("suggestionPreview")}</h3>
        <p>{summarizeConditions(preview.conditions, "and", language)}</p>
        {preview.warnings.map((warning) => <Alert key={warning} type="warning" message={t(reasonKeys[warning] ?? "suggestionIneligibleReason")} />)}
        {preview.needs_review && <Checkbox disabled={dirty || loading || busy} checked={acknowledge} onChange={(e) => { setAcknowledge(e.target.checked); if (e.target.checked) publish(preview, write, true); else onReviewed(null); }}>{t("suggestionAcknowledge")}</Checkbox>}
        <details open><summary>{t("suggestionEvidence")}</summary>
          <ul>{preview.history.map((row) => <li key={row.id}>
            <p>{row.date} · {row.description} · {row.currency_code ? formatMoney(row.amount, row.currency_code) : "—"}</p>
            <p>{t(row.status === "matched" ? "suggestionMatched" : row.status === "ambiguous" ? "suggestionAmbiguous" : "suggestionUnmatched")}{row.evidence ? ` · ${t("suggestionEvidence")}` : ""}</p>
            <p>{row.reasons.map((reason) => t(reasonKeys[reason] ?? "suggestionIneligibleReason")).join(" · ")}</p>
            {row.failed_conditions.length > 0 && <p>{summarizeConditions(row.failed_conditions, "and", language)}</p>}
            {row.rule_ids.length + row.scenario_ids.length > 0 && <p>{t("suggestionOverlapReason")}: {[...row.rule_ids, ...row.scenario_ids].join(", ")}</p>}
          </li>)}</ul>
        </details>
      </>}
      <p>{t("suggestionApprovalHint")}</p>
    </Flex>
  </Drawer>;
}
