import { useTranslation } from "../../i18n/context";
import { compareDecimals, isPositiveDecimal } from "../../presentation/decimal";
import { Input } from "../ui";
import { useEffect, useState } from "react";

import type { Payable, PayableKind } from "../../api/contracts";
import { getPayableVocabulary } from "../../presentation/payableLabels";
import { FormDrawer } from "../forms/FormDrawer";
import { FormField } from "../forms/FormField";
import { MoneyInput } from "../forms/MoneyInput";

type Draft = {
  name: string;
  totalAmount: string | null;
  initialRemainingAmount: string | null;
};

const blankDraft: Draft = { name: "", totalAmount: null, initialRemainingAmount: null };

function draftFrom(payable: Payable | null): Draft {
  return payable
    ? { name: payable.name, totalAmount: payable.total_amount, initialRemainingAmount: null }
    : blankDraft;
}

export function PayableForm({
  kind,
  open,
  payable,
  submitting,
  submitError,
  onSubmit,
  onCancel,
}: {
  kind: PayableKind;
  open: boolean;
  payable: Payable | null;
  submitting: boolean;
  submitError: string | null;
  onSubmit: (write: { name: string; total_amount: string; initial_remaining_amount: string | null }) => void;
  onCancel: () => void;
}) {
  const { t, language } = useTranslation();
  const vocab = getPayableVocabulary(kind, language);
  const [draftState, setDraftState] = useState<Draft>(() => draftFrom(payable));
  const [error, setError] = useState<string | null>(null);
  const isEditing = payable !== null;

  useEffect(() => {
    if (open) {
      setDraftState(draftFrom(payable));
      setError(null);
    }
  }, [open, payable]);

  const submit = () => {
    if (draftState.name.trim() === "") {
      setError(vocab.nameRequiredError);
      return;
    }
    if (draftState.totalAmount === null || !isPositiveDecimal(draftState.totalAmount)) {
      setError(t("payableTotalPositive"));
      return;
    }
    if (!isEditing && draftState.initialRemainingAmount !== null) {
      if (compareDecimals(draftState.initialRemainingAmount, "0") < 0) {
        setError(t("initialRemainingNegative"));
        return;
      }
      if (compareDecimals(draftState.initialRemainingAmount, draftState.totalAmount) > 0) {
        setError(t("initialRemainingExceedsTotal"));
        return;
      }
    }
    setError(null);
    onSubmit({
      name: draftState.name.trim(),
      total_amount: draftState.totalAmount,
      initial_remaining_amount: isEditing ? null : draftState.initialRemainingAmount,
    });
  };

  return (
    <FormDrawer
      title={isEditing ? vocab.editTitle : vocab.newTitle}
      open={open}
      onClose={onCancel}
      onSubmit={submit}
      submitting={submitting}
      error={error ?? submitError}
    >
      <FormField label={t("name")} htmlFor="payable-name">
        <Input
          id="payable-name"
          value={draftState.name}
          onChange={(event) => setDraftState((current) => ({ ...current, name: event.target.value }))}
          placeholder={vocab.nameFieldPlaceholder}
        />
      </FormField>

      <FormField label={t("totalAmount")} htmlFor="payable-total-amount">
        <MoneyInput
          id="payable-total-amount"
          min="0"
          value={draftState.totalAmount}
          onChange={(value) => setDraftState((current) => ({ ...current, totalAmount: value }))}
        />
      </FormField>

      {!isEditing && (
        <FormField
          label={t("initialRemainingOptional")}
          htmlFor="payable-initial-remaining-amount"
          hint={t("initialRemainingHint")}
        >
          <MoneyInput
            id="payable-initial-remaining-amount"
            min="0"
            value={draftState.initialRemainingAmount}
            onChange={(value) => setDraftState((current) => ({ ...current, initialRemainingAmount: value }))}
            placeholder={t("defaultsToTotal")}
          />
        </FormField>
      )}
    </FormDrawer>
  );
}
