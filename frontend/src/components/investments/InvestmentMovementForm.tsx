import { useEffect, useRef, useState } from "react";

import type { InvestmentMovementKind, InvestmentMovementWrite, InvestmentTransaction } from "../../api/contracts";
import { useTimezone } from "../../hooks/useTimezone";
import { useTranslation } from "../../i18n/context";
import { calendar, type CalendarDate } from "../../presentation/calendar";
import { isPositiveDecimal, isZeroDecimal, multiplyDecimals, subtractDecimals } from "../../presentation/decimal";
import { todayIn } from "../../presentation/timezone";
import { FormDrawer } from "../forms/FormDrawer";
import { FormField } from "../forms/FormField";
import { MoneyInput } from "../forms/MoneyInput";
import { Button, DatePicker, Input, InputNumber, Select } from "../ui";
import { fromMoneyInput, toMoneyInput } from "./moneyDraft";

type Draft = {
  kind: InvestmentMovementKind;
  tradeDate: CalendarDate;
  quantity: string | null;
  unitPrice: string | null;
  amount: string | null;
  notes: string;
};

/**
 * A total that is exactly quantity × unit price was priced by the server, not
 * typed. Loading it back as empty keeps it out of the PUT, so a corrected
 * quantity or price is priced again instead of inheriting the old total.
 */
function isDerivedAmount(movement: InvestmentTransaction): boolean {
  if (movement.amount === null || movement.quantity === null || movement.value === null) return false;
  return isZeroDecimal(subtractDecimals(movement.amount, multiplyDecimals(movement.quantity, movement.value)));
}

function draftFrom(movement: InvestmentTransaction | null, timezone: string): Draft {
  if (movement === null) {
    return { kind: "BUY", tradeDate: todayIn(timezone), quantity: null, unitPrice: null, amount: null, notes: "" };
  }
  const kind = movement.movement_type === "SELL" || movement.movement_type === "INTEREST" ? movement.movement_type : "BUY";
  const day = movement.trade_date ?? movement.occurred_at;
  return {
    kind,
    tradeDate: day === null ? todayIn(timezone) : calendar(day.slice(0, 10)),
    quantity: movement.quantity,
    unitPrice: movement.value,
    amount: isDerivedAmount(movement) ? null : movement.amount,
    notes: movement.notes ?? "",
  };
}

/**
 * A movement the institution never reported for one of its holdings. The
 * total is what left or entered the account, costs included: when it is left
 * empty the server prices it as quantity × unit price.
 */
export function InvestmentMovementForm({
  open,
  investmentId,
  movement,
  submitting,
  submitError,
  onSubmit,
  onDelete,
  onCancel,
}: {
  open: boolean;
  investmentId: string;
  /** The manual movement being corrected; null records a new one. */
  movement: InvestmentTransaction | null;
  submitting: boolean;
  submitError: string | null;
  onSubmit: (write: InvestmentMovementWrite) => void;
  onDelete: (movement: InvestmentTransaction) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const timezone = useTimezone();
  const [draft, setDraft] = useState<Draft>(() => draftFrom(movement, timezone));
  const [error, setError] = useState<string | null>(null);
  // Until the total is typed it follows quantity × price, so changing either clears it.
  const amountTouched = useRef(movement !== null && !isDerivedAmount(movement));

  // The drawer stays mounted: start over each time it opens.
  useEffect(() => {
    if (!open) return;
    setDraft(draftFrom(movement, timezone));
    setError(null);
    amountTouched.current = movement !== null && !isDerivedAmount(movement);
    // `timezone` is read once per opening, not tracked.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, movement]);

  const isTrade = draft.kind !== "INTEREST";
  const kindOptions = [
    { value: "BUY", label: t("manualMovementKindBuy") },
    { value: "SELL", label: t("manualMovementKindSell") },
    { value: "INTEREST", label: t("manualMovementKindInterest") },
  ];

  const setDecimal = (field: "quantity" | "unitPrice", value: string | number | null) => {
    const next = value === null ? null : String(value);
    setDraft((current) => ({ ...current, [field]: next, ...(amountTouched.current ? {} : { amount: null }) }));
  };

  const submit = () => {
    if (todayIn(timezone).isBefore(draft.tradeDate, "day")) {
      setError(t("manualMovementFutureDate"));
      return;
    }
    if (isTrade && (draft.quantity === null || !isPositiveDecimal(draft.quantity))) {
      setError(t("manualMovementQuantityRequired"));
      return;
    }
    const hasTotal = draft.amount !== null && isPositiveDecimal(draft.amount);
    const hasPrice = draft.unitPrice !== null && isPositiveDecimal(draft.unitPrice);
    if (!hasTotal && !(isTrade && hasPrice)) {
      setError(t("positiveAmountRequired"));
      return;
    }
    setError(null);
    onSubmit({
      investment_id: investmentId,
      kind: draft.kind,
      trade_date: draft.tradeDate.format("YYYY-MM-DD"),
      quantity: isTrade ? draft.quantity : null,
      unit_price: isTrade ? draft.unitPrice : null,
      amount: hasTotal ? draft.amount : null,
      notes: draft.notes.trim() || null,
    });
  };

  return (
    <FormDrawer
      title={movement === null ? t("manualMovementNew") : t("manualMovementEdit")}
      open={open}
      onClose={onCancel}
      onSubmit={submit}
      submitting={submitting}
      error={error ?? submitError}
    >
      <div className="form-drawer-form">
        {movement === null && <p className="form-field-hint">{t("manualMovementHint")}</p>}
        <FormField label={t("type")} htmlFor="investment-movement-kind">
          <Select
            id="investment-movement-kind"
            value={draft.kind}
            options={kindOptions}
            onChange={(value: InvestmentMovementKind) => setDraft((current) => ({ ...current, kind: value }))}
          />
        </FormField>
        <FormField label={t("date")} htmlFor="investment-movement-date">
          <DatePicker
            id="investment-movement-date"
            style={{ width: "100%" }}
            value={draft.tradeDate}
            allowClear={false}
            onChange={(value) => value && setDraft((current) => ({ ...current, tradeDate: value }))}
          />
        </FormField>
        {isTrade && (
          <div className="investment-form-pair">
            <FormField label={t("quantityLabel")} htmlFor="investment-movement-quantity">
              <InputNumber
                id="investment-movement-quantity"
                style={{ width: "100%" }}
                min="0.00000001"
                stringMode
                inputMode="decimal"
                decimalSeparator=","
                value={draft.quantity}
                onChange={(value) => setDecimal("quantity", value)}
              />
            </FormField>
            <FormField label={t("unitPriceLabel")} htmlFor="investment-movement-unit-price">
              <InputNumber
                id="investment-movement-unit-price"
                style={{ width: "100%" }}
                min="0.00000001"
                stringMode
                inputMode="decimal"
                prefix="R$"
                decimalSeparator=","
                value={draft.unitPrice}
                onChange={(value) => setDecimal("unitPrice", value)}
              />
            </FormField>
          </div>
        )}
        <FormField
          label={t("manualMovementAmount")}
          htmlFor="investment-movement-amount"
          hint={isTrade ? t("manualMovementAmountHint") : undefined}
        >
          <MoneyInput
            id="investment-movement-amount"
            min="0"
            value={toMoneyInput(draft.amount)}
            onChange={(value) => {
              amountTouched.current = true;
              setDraft((current) => ({ ...current, amount: fromMoneyInput(value) }));
            }}
          />
        </FormField>
        <FormField label={t("notesOptional")} htmlFor="investment-movement-notes">
          <Input.TextArea
            id="investment-movement-notes"
            rows={3}
            value={draft.notes}
            onChange={(event) => setDraft((current) => ({ ...current, notes: event.target.value }))}
          />
        </FormField>
        {movement !== null && (
          <Button danger block disabled={submitting} onClick={() => onDelete(movement)}>
            {t("delete")}
          </Button>
        )}
      </div>
    </FormDrawer>
  );
}
