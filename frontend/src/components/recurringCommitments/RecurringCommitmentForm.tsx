import { longMonthName } from "../../i18n/dateFormats";
import { useTranslation } from "../../i18n/context";
import type { MessageKey } from "../../i18n/messages";
import { isPositiveDecimal, sumDecimals } from "../../presentation/decimal";
import { Alert, DatePicker, Flex, Input, InputNumber, Select, Switch } from "../ui";
import { calendar, type CalendarDate } from "../../presentation/calendar";
import { useEffect, useState, type Dispatch, type ReactNode, type SetStateAction } from "react";

import type {
  Category,
  RecurringCommitment,
  RecurringCommitmentCadence,
  RecurringCommitmentKind,
  RecurringCommitmentWrite,
} from "../../api/contracts";
import {
  recurringCommitmentCadenceName,
  recurringCommitmentKindName,
} from "../../presentation/recurringCommitmentLabels";
import { AccountSelect } from "../forms/AccountSelect";
import { FormDrawer } from "../forms/FormDrawer";
import { FormField } from "../forms/FormField";
import { MoneyInput } from "../forms/MoneyInput";

const dateFormat = "YYYY-MM-DD";


export type RecurringCommitmentDraft = {
  name: string;
  kind: RecurringCommitmentKind;
  amount: string | null;
  categoryId: string | null;
  accountId: string;
  cadence: RecurringCommitmentCadence;
  dayOfMonth: number | null;
  monthOfYear: number | null;
  startDate: CalendarDate;
  endDate: CalendarDate | null;
  isActive: boolean;
};

function blankDraft(): RecurringCommitmentDraft {
  return {
    name: "",
    kind: "expense",
    amount: null,
    categoryId: null,
    accountId: "",
    cadence: "monthly",
    dayOfMonth: null,
    monthOfYear: null,
    startDate: calendar(),
    endDate: null,
    isActive: true,
  };
}

function draftFrom(
  commitment: RecurringCommitment | null,
  initialDraft?: Partial<RecurringCommitmentDraft> | null,
): RecurringCommitmentDraft {
  if (!commitment) return { ...blankDraft(), ...initialDraft };
  return {
    name: commitment.name,
    kind: commitment.kind,
    amount: commitment.amount,
    categoryId: commitment.category_id,
    accountId: commitment.account_id ?? "",
    cadence: commitment.cadence,
    dayOfMonth: commitment.day_of_month,
    monthOfYear: commitment.month_of_year,
    startDate: calendar(commitment.start_date),
    endDate: commitment.end_date ? calendar(commitment.end_date) : null,
    isActive: commitment.is_active,
  };
}

/** Validates the draft; an empty "Dia do mês" falls back to the start date's day. */
function buildWrite(
  draft: RecurringCommitmentDraft,
  t: (key: MessageKey) => string,
): { error: string } | { write: RecurringCommitmentWrite } {
  if (draft.name.trim() === "") return { error: t("recurrenceNameRequired") };
  if (draft.amount === null || !isPositiveDecimal(draft.amount)) return { error: t("positiveAmountRequired") };
  if (draft.categoryId === null) return { error: t("categoryRequired") };
  if (draft.cadence === "annual" && draft.monthOfYear === null) {
    return { error: t("monthOfYearRequired") };
  }
  if (draft.endDate && draft.endDate.isBefore(draft.startDate, "day")) {
    return { error: t("endBeforeStart") };
  }
  return {
    write: {
      name: draft.name.trim(),
      kind: draft.kind,
      amount: sumDecimals([draft.amount]),
      category_id: draft.categoryId,
      account_id: draft.accountId.trim() === "" ? null : draft.accountId.trim(),
      cadence: draft.cadence,
      day_of_month: draft.dayOfMonth ?? draft.startDate.date(),
      month_of_year: draft.cadence === "annual" ? draft.monthOfYear : null,
      start_date: draft.startDate.format(dateFormat),
      end_date: draft.endDate ? draft.endDate.format(dateFormat) : null,
      is_active: draft.isActive,
    },
  };
}

/**
 * The inputs of a recurrence, without a container or a `<form>`: the drawer
 * and the transaction panel each wrap them in their own form. The draft lives
 * in the caller.
 */
function RecurringCommitmentFieldset({
  draft,
  setDraft,
  categories,
}: {
  draft: RecurringCommitmentDraft;
  setDraft: Dispatch<SetStateAction<RecurringCommitmentDraft>>;
  categories: Category[];
}) {
  const { t, language } = useTranslation();
  const kindOptions = (["income", "expense"] as RecurringCommitmentKind[]).map((value) => ({
    value,
    label: recurringCommitmentKindName(value, language),
  }));
  const cadenceOptions = (["monthly", "annual"] as RecurringCommitmentCadence[]).map((value) => ({
    value,
    label: recurringCommitmentCadenceName(value, language),
  }));
  const monthOptions = Array.from({ length: 12 }, (_, index) => ({
    value: index + 1,
    label: longMonthName(language, index),
  }));
  const categoryOptions = categories
    .filter((category) => category.is_active && category.kind !== "transfer")
    .map((category) => ({ value: category.id, label: category.name }));

  return (
    <>
      <FormField label={t("name")} htmlFor="recurring-commitment-name">
        <Input
          id="recurring-commitment-name"
          value={draft.name}
          onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))}
          placeholder={t("recurrenceNameExample")}
        />
      </FormField>

      <FormField label={t("type")} htmlFor="recurring-commitment-kind">
        <Select
          id="recurring-commitment-kind"
          value={draft.kind}
          options={kindOptions}
          onChange={(value: RecurringCommitmentKind) => setDraft((current) => ({ ...current, kind: value }))}
        />
      </FormField>

      <FormField label={t("amount")} htmlFor="recurring-commitment-amount">
        <MoneyInput
          id="recurring-commitment-amount"
          min="0"
          value={draft.amount}
          onChange={(value) => setDraft((current) => ({ ...current, amount: value }))}
        />
      </FormField>

      <FormField label={t("category")} htmlFor="recurring-commitment-category">
        <Select
          id="recurring-commitment-category"
          value={draft.categoryId ?? undefined}
          options={categoryOptions}
          showSearch
          optionFilterProp="label"
          placeholder={t("selectACategory")}
          onChange={(value: string) => setDraft((current) => ({ ...current, categoryId: value }))}
        />
      </FormField>

      <FormField label={t("accountOptional")} htmlFor="recurring-commitment-account">
        <AccountSelect
          id="recurring-commitment-account"
          optional
          value={draft.accountId}
          onChange={(accountId) => setDraft((current) => ({ ...current, accountId: accountId ?? "" }))}
        />
      </FormField>

      <FormField label={t("cadence")} htmlFor="recurring-commitment-cadence">
        <Select
          id="recurring-commitment-cadence"
          value={draft.cadence}
          options={cadenceOptions}
          onChange={(value: RecurringCommitmentCadence) => setDraft((current) => ({ ...current, cadence: value }))}
        />
      </FormField>

      <FormField
        label={t("dayOfMonth")}
        htmlFor="recurring-commitment-day"
        hint={t("dayOfMonthHintBlank")}
      >
        <InputNumber
          id="recurring-commitment-day"
          style={{ width: "100%" }}
          min={1}
          max={31}
          precision={0}
          inputMode="numeric"
          value={draft.dayOfMonth}
          placeholder={String(draft.startDate.date())}
          onChange={(value) => setDraft((current) => ({ ...current, dayOfMonth: value }))}
        />
      </FormField>

      {draft.cadence === "annual" && (
        <FormField label={t("monthOfYear")} htmlFor="recurring-commitment-month">
          <Select
            id="recurring-commitment-month"
            value={draft.monthOfYear ?? undefined}
            options={monthOptions}
            placeholder={t("selectAMonth")}
            onChange={(value: number) => setDraft((current) => ({ ...current, monthOfYear: value }))}
          />
        </FormField>
      )}

      <FormField label={t("startDateLabel")} htmlFor="recurring-commitment-start-date">
        <DatePicker
          id="recurring-commitment-start-date"
          style={{ width: "100%" }}
          value={draft.startDate}
          allowClear={false}
          onChange={(value) => value && setDraft((current) => ({ ...current, startDate: value }))}
        />
      </FormField>

      <FormField label={t("endDateOptional")} htmlFor="recurring-commitment-end-date">
        <DatePicker
          id="recurring-commitment-end-date"
          style={{ width: "100%" }}
          value={draft.endDate}
          onChange={(value) => setDraft((current) => ({ ...current, endDate: value }))}
        />
      </FormField>

      <FormField label={t("activeFeminineHeader")} htmlFor="recurring-commitment-active">
        <Switch
          id="recurring-commitment-active"
          aria-label={t("recurrenceActive")}
          checked={draft.isActive}
          onChange={(checked) => setDraft((current) => ({ ...current, isActive: checked }))}
          style={{ width: "fit-content" }}
        />
      </FormField>
    </>
  );
}

type FieldsProps = {
  /** The `<form>` id, so a submit button can live outside the fields (e.g. a sticky footer). */
  formId: string;
  commitment: RecurringCommitment | null;
  initialDraft?: Partial<RecurringCommitmentDraft> | null;
  categories: Category[];
  submitError: string | null;
  onSubmit: (write: RecurringCommitmentWrite) => void;
};

/**
 * The fields of a recurrence as a `<form>` of their own, for hosts that bring
 * a submit control of their own (the transaction panel's sticky footer). The
 * draft is seeded once on mount (remount with a `key` to start over). While
 * `submitting`, Enter in a field does nothing — the host's button is already
 * disabled by its own spinner, but the keyboard would still send a second write.
 */
export function RecurringCommitmentFields({
  formId,
  commitment,
  initialDraft,
  categories,
  submitError,
  submitting = false,
  onSubmit,
}: FieldsProps & { submitting?: boolean }) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<RecurringCommitmentDraft>(() => draftFrom(commitment, initialDraft));
  const [error, setError] = useState<string | null>(null);

  return (
    <form
      id={formId}
      onSubmit={(event) => {
        event.preventDefault();
        if (submitting) return;
        const built = buildWrite(draft, t);
        if ("error" in built) {
          setError(built.error);
          return;
        }
        setError(null);
        onSubmit(built.write);
      }}
    >
      <Flex vertical gap="middle">
        {(error ?? submitError) && <Alert type="error" showIcon message={error ?? submitError} />}
        <RecurringCommitmentFieldset draft={draft} setDraft={setDraft} categories={categories} />
      </Flex>
    </form>
  );
}

/**
 * The Recorrências page's editor: the shared form drawer (full-screen sheet
 * on a phone) around the same fields. `extra` is extra content under the
 * fields while editing — on a phone it carries what the table's expanded row
 * carries on a wide screen (the occurrences) and the delete action.
 */
export function RecurringCommitmentForm({
  open,
  commitment,
  initialDraft,
  categories,
  submitting,
  submitError,
  onSubmit,
  onCancel,
  extra,
}: Omit<FieldsProps, "formId"> & {
  open: boolean;
  submitting: boolean;
  onCancel: () => void;
  extra?: ReactNode;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<RecurringCommitmentDraft>(() => draftFrom(commitment, initialDraft));
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setDraft(draftFrom(commitment, initialDraft));
      setError(null);
    }
    // The draft is seeded when the drawer opens; later prop changes must not reset what is being typed.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, commitment?.id]);

  const submit = () => {
    const built = buildWrite(draft, t);
    if ("error" in built) {
      setError(built.error);
      return;
    }
    setError(null);
    onSubmit(built.write);
  };

  return (
    <FormDrawer
      title={commitment ? t("editRecurrence") : t("newRecurrence")}
      open={open}
      onClose={onCancel}
      onSubmit={submit}
      submitting={submitting}
      error={error ?? submitError}
    >
      <RecurringCommitmentFieldset draft={draft} setDraft={setDraft} categories={categories} />
      {extra}
    </FormDrawer>
  );
}
