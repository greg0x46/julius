import { useEffect, useId, useRef, useState } from "react";
import type { Category, RecurringCommitmentWrite, RuleCondition } from "../../api/contracts";
import type { RecurrenceSuggestion, SuggestionApproval, SuggestionReview } from "../../api/contracts/recurrenceSuggestions";
import { reviewRecurrenceSuggestion } from "../../api/recurringCommitments";
import { useTranslation } from "../../i18n/context";
import type { MessageKey } from "../../i18n/messages";
import { calendar } from "../../presentation/calendar";
import { formatDay } from "../../presentation/dates";
import { errorMessage } from "../../presentation/errors";
import { formatBRL } from "../../presentation/money";
import { ruleConditionFieldName, ruleConditionOperatorName, summarizeConditions } from "../../presentation/ruleConditionLabels";
import { FormField } from "../forms/FormField";
import { ConditionListEditor, type ConditionFieldConfig } from "../shared/ConditionListEditor";
import { Money } from "../shared/Money";
import { StatusTag, type StatusTagTone } from "../shared/StatusTag";
import { useCompactScreen } from "../shared/useCompactScreen";
import { Alert, Button, Checkbox, Drawer } from "../ui";
import { CloseOutlined } from "../ui/icons";
import { RecurringCommitmentFields } from "./RecurringCommitmentForm";

const reasonKeys: Record<string, MessageKey> = {
  consistent_calendar: "suggestionCalendarReason", one_observation_per_period: "suggestionPeriodReason", fixed_amount: "suggestionFixedReason", variable_amount: "suggestionVariableReason", missing_months: "suggestionMissingReason", category_required: "suggestionCategoryReason", broad_description: "suggestionBroadReason", unrestricted_account: "suggestionAccountReason", cross_month_unsupported: "suggestionUnsupported", multiple_matches_per_occurrence: "suggestionMultipleReason", existing_overlap: "suggestionOverlapReason", existing_rule_or_scenario_overlap: "suggestionOverlapReason", possible_false_positives: "suggestionFalsePositiveReason", possible_false_positive: "suggestionFalsePositiveReason", conditions_failed: "suggestionConditionsFailed", manual_link_precedence: "suggestionManualReason", manually_taken: "suggestionManualReason", manual_detach: "suggestionManualReason", account_restriction: "suggestionIneligibleReason", ineligible_cash_movement: "suggestionIneligibleReason", no_occurrence_in_month: "suggestionIneligibleReason", missing_date_or_money: "suggestionIneligibleReason", conditions_and_month_match: "suggestionMatched",
};

const historyStatus = {
  matched: { key: "suggestionMatched", tone: "success" },
  ambiguous: { key: "suggestionAmbiguous", tone: "warning" },
  unmatched: { key: "suggestionUnmatched", tone: "neutral" },
} as const satisfies Record<string, { key: MessageKey; tone: StatusTagTone }>;

export type Reviewed = { approval: SuggestionApproval; preview: SuggestionReview };

/**
 * One candidate's review: the editable draft and reconciliation conditions, a
 * preview against the history, and the creation. Opening it already simulates
 * the rule (when the pattern has a category), and a valid preview is what marks
 * the candidate as reviewed; closing keeps it. It is a hand-built drawer
 * (the fields bring a `<form>` of their own, so `FormDrawer` cannot wrap them)
 * on the same `form-drawer` skin: header, gutter and footer.
 */
export function SuggestionReviewDrawer({ candidate, categories, saved, error, busy, onReviewed, onAccept, onDismiss, onClose }: {
  candidate: RecurrenceSuggestion; categories: Category[]; saved?: Reviewed; error?: string; busy: boolean;
  onReviewed: (r: Reviewed | null) => void; onAccept: () => void; onDismiss: () => void; onClose: () => void;
}) {
  const { t, language } = useTranslation();
  const compact = useCompactScreen();
  const conditionsLabelId = useId();
  const previewTitleId = useId();
  const evidenceTitleId = useId();
  const [preview, setPreview] = useState(saved?.preview ?? null);
  const [conditions, setConditions] = useState<RuleCondition[] | undefined>(saved?.approval.conditions);
  const [write, setWrite] = useState(saved?.approval.draft ?? candidate.draft);
  const [acknowledge, setAcknowledge] = useState(saved?.approval.acknowledge ?? false);
  const [dirty, setDirty] = useState(!saved);
  const autoPreview = !saved && candidate.draft.category_id !== "";
  const [loading, setLoading] = useState(autoPreview);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const version = useRef(0);
  const open = useRef(false);
  const started = useRef(false);
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
      if (version.current !== currentVersion || !open.current) return;
      setPreview(next); setConditions(next.conditions); setWrite(nextWrite); setDirty(false); setAcknowledge(false);
      publish(next, nextWrite, false);
    } catch (e) { setPreviewError(errorMessage(e, t("recurrenceSaveFailed"))); }
    finally { setLoading(false); }
  };
  useEffect(() => {
    open.current = true;
    if (autoPreview && !started.current) { started.current = true; void updatePreview(candidate.draft); }
    return () => { open.current = false; };
    // Once, on open; later previews are the owner's.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  const textConfigs: ConditionFieldConfig[] = (["description", "account", "card"] as const).map((field) => ({ field, label: ruleConditionFieldName(field, language), operators: (["contains", "equals"] as const).map((operator) => ({ value: operator, label: ruleConditionOperatorName(operator, language) })) }));
  const configs: ConditionFieldConfig[] = [...textConfigs,
    { field: "amount", label: ruleConditionFieldName("amount", language), operators: [], pairOperator: "within_percent", pairFirstLabel: t("expectedAmount"), pairSecondLabel: t("tolerance"), pairSecondSuffix: "%", pairMin: 0 },
    { field: "day_of_month", label: ruleConditionFieldName("day_of_month", language), operators: [], pairOperator: "day_range", pairFirstLabel: t("fromDay"), pairSecondLabel: t("untilDay"), pairMin: 1, pairMax: 31 },
  ];
  // What still blocks the creation, in the order the owner meets it. A preview
  // exists only for a valid category, so the category is asked for only before one.
  const step: MessageKey | null = loading ? "suggestionStepLoading"
    : !preview ? (draft.category_id === "" ? "suggestionStepCategory" : "suggestionStepSimulate")
    : dirty ? "suggestionStepPreview"
    : preview.warnings.includes("cross_month_unsupported") ? null
    : preview.needs_review && !acknowledge ? "suggestionStepAcknowledge"
    : eligible ? "suggestionStepReady"
    : "suggestionStepPreview";
  // What the rule finds; the month's unrelated transactions stay out, except evidence the rule misses.
  const found = preview?.history.filter((row) => row.status !== "unmatched" || row.evidence) ?? [];
  const reasonText = (reason: string) => t(reasonKeys[reason] ?? "suggestionIneligibleReason");
  const close = busy ? undefined : onClose;
  return <Drawer open title={`${t("reviewSuggestion")}: ${candidate.draft.name}`} width={compact ? "100%" : 640} onClose={close} rootClassName="form-drawer" closable={!compact}
    extra={compact ? <Button type="text" className="form-drawer-close" icon={<CloseOutlined />} aria-label={t("close")} onClick={close} /> : undefined}
    footer={<div className="form-drawer-footer recurrence-review-footer">
      <Button type={eligible ? "default" : "primary"} size={compact && !eligible ? "large" : "middle"} disabled={busy || loading} htmlType="submit" form="suggestion-review-fields" loading={loading}>{t("updateSuggestionPreview")}</Button>
      <Button type={eligible ? "primary" : "default"} size={compact && eligible ? "large" : "middle"} disabled={!eligible || loading} loading={busy} onClick={onAccept}>{t("createSuggestion")}</Button>
    </div>}>
    <div className="form-drawer-form">
      <div className="recurrence-review-pattern">
        <p><span className="recurrence-review-confidence">{t(candidate.confidence === "high" ? "suggestionHighConfidence" : "suggestionMediumConfidence")}.</span> {t("suggestionConfidenceHint")}</p>
        {candidate.amount_kind === "variable" && <p>{t("suggestionAmountRange", { min: formatBRL(candidate.minimum_amount), max: formatBRL(candidate.maximum_amount) })}</p>}
        <ul>{candidate.reasons.map((reason) => <li key={reason}>{reasonText(reason)}</li>)}</ul>
      </div>
      {step && <p className={`recurrence-review-step${eligible ? " recurrence-review-step-ready" : ""}`} role="status">{t(step)}</p>}
      <RecurringCommitmentFields formId="suggestion-review-fields" onDraftChange={invalidate} commitment={null} initialDraft={{ name: draft.name, kind: draft.kind, amount: draft.amount, categoryId: draft.category_id || null, accountId: draft.account_id ?? "", cadence: draft.cadence, dayOfMonth: draft.day_of_month, monthOfYear: draft.month_of_year, startDate: calendar(draft.start_date), endDate: draft.end_date ? calendar(draft.end_date) : null, isActive: draft.is_active }} categories={categories} submitting={loading || busy} submitError={previewError} onSubmit={(w) => void updatePreview(w)} />
      {conditions && <FormField label={t("suggestionRuleConditions")} labelId={conditionsLabelId}>
        <ConditionListEditor conditions={conditions} onChange={(c) => { invalidate(); setConditions(c); }} fieldConfigs={configs} />
      </FormField>}
      {error && <Alert type="error" showIcon message={error} />}
      {preview ? <section className="recurrence-extra" aria-labelledby={previewTitleId}>
        <h3 id={previewTitleId} className="recurrence-extra-title">{t("suggestionPreview")}</h3>
        <div className="form-drawer-form">
          <p className="recurrence-review-hint">{summarizeConditions(preview.conditions, "and", language)}</p>
          {preview.warnings.map((warning) => <Alert key={warning} type="warning" showIcon message={reasonText(warning)} />)}
          {preview.needs_review && <Checkbox disabled={dirty || loading || busy} checked={acknowledge} onChange={(e) => { setAcknowledge(e.target.checked); if (e.target.checked) publish(preview, write, true); else onReviewed(null); }}>{t("suggestionAcknowledge")}</Checkbox>}
          {found.length === 0 && <p className="recurrence-review-hint">{t("suggestionNoMatches")}</p>}
          <ul className="occurrence-rows" aria-labelledby={previewTitleId}>{found.map((row) => {
            const status = t(historyStatus[row.status].key);
            // The match reason repeats the status tag.
            const reasons = row.reasons.map(reasonText).filter((text) => text !== status);
            return <li key={row.id} className="occurrence-row">
              <div className="occurrence-row-line">
                <span className="occurrence-row-title">{row.description}</span>
                {row.currency_code ? <Money value={row.amount} currencyCode={row.currency_code} tone="flow" size="row" /> : <span>—</span>}
              </div>
              <div className="occurrence-row-meta">
                <span>{formatDay(row.date, language)}</span>
                <StatusTag tone={historyStatus[row.status].tone}>{status}</StatusTag>
                {row.evidence && <span>{t("suggestionEvidence")}</span>}
              </div>
              {reasons.length > 0 && <div className="occurrence-row-meta"><span>{reasons.join(" · ")}</span></div>}
              {row.failed_conditions.length > 0 && <div className="occurrence-row-meta"><span>{summarizeConditions(row.failed_conditions, "and", language)}</span></div>}
              {row.rule_ids.length + row.scenario_ids.length > 0 && <div className="occurrence-row-meta"><span>{t("suggestionOverlapReason")}: {[...row.rule_ids, ...row.scenario_ids].join(", ")}</span></div>}
            </li>;
          })}</ul>
        </div>
      </section> : <section className="recurrence-extra" aria-labelledby={evidenceTitleId}>
        <h3 id={evidenceTitleId} className="recurrence-extra-title">{t("suggestionEvidence")}</h3>
        <ul className="occurrence-rows" aria-labelledby={evidenceTitleId}>{candidate.evidence.map((row) => <li key={row.id} className="occurrence-row">
          <div className="occurrence-row-line">
            <span className="occurrence-row-title">{row.description}</span>
            <Money value={row.amount} currencyCode={candidate.currency_code} tone="flow" size="row" />
          </div>
          <div className="occurrence-row-meta"><span>{formatDay(row.date, language)}</span></div>
        </li>)}</ul>
      </section>}
      <p className="recurrence-review-hint">{t("suggestionApprovalHint")}</p>
      <Button type="link" className="recurrence-review-dismiss" disabled={busy} onClick={onDismiss}>{t("dismissSuggestion")}</Button>
    </div>
  </Drawer>;
}
