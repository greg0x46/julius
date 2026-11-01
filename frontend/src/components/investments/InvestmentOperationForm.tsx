import { useTranslation } from "../../i18n/context";
import { compareDecimals, isPositiveDecimal } from "../../presentation/decimal";
import { Alert, Checkbox, DatePicker, Flex, Input, InputNumber, Select } from "../ui";
import { calendar, type CalendarDate } from "../../presentation/calendar";
import { todayIn } from "../../presentation/timezone";
import { useTimezone } from "../../hooks/useTimezone";
import { useEffect, useMemo, useRef, useState, type MutableRefObject } from "react";

import type {
  InvestmentAccount,
  InvestmentOperation,
  InvestmentOperationKind,
  InvestmentOperationWrite,
  InvestmentPosition,
} from "../../api/contracts";
import { isZeroDecimal, multiplyDecimals, subtractDecimals, sumDecimals } from "../../presentation/decimal";
import { investmentOperationKindName } from "../../presentation/investmentWorkspaceLabels";
import { FormDrawer } from "../forms/FormDrawer";
import { FormField } from "../forms/FormField";
import { MoneyInput } from "../forms/MoneyInput";
import { fromMoneyInput, toMoneyInput } from "./moneyDraft";

type Draft = {
  accountId: string;
  positionId: string | null;
  kind: InvestmentOperationKind;
  occurredOn: CalendarDate;
  amount: string | null;
  quantity: string | null;
  unitPrice: string | null;
  fees: string | null;
  taxes: string | null;
  notes: string;
};

function blankDraft(accounts: InvestmentAccount[], timezone: string, initial?: Partial<InvestmentOperationWrite>): Draft {
  return {
    accountId: initial?.account_id ?? accounts.find((account) => account.active)?.id ?? "",
    positionId: initial?.position_id ?? null,
    kind: initial?.kind ?? "deposit",
    occurredOn: initial?.occurred_on ? calendar(initial.occurred_on) : todayIn(timezone),
    amount: initial?.amount ?? null,
    quantity: initial?.quantity ?? null,
    unitPrice: initial?.unit_price ?? null,
    fees: initial?.fees ?? null,
    taxes: initial?.taxes ?? null,
    notes: initial?.notes ?? "",
  };
}

function draftFrom(
  operation: InvestmentOperation | null,
  accounts: InvestmentAccount[],
  timezone: string,
  initial?: Partial<InvestmentOperationWrite>,
): Draft {
  if (!operation) return blankDraft(accounts, timezone, initial);
  return {
    accountId: operation.account_id,
    positionId: operation.position_id,
    kind: operation.kind,
    occurredOn: calendar(operation.occurred_on),
    amount: isDerivedAmount(operation) ? null : operation.amount,
    quantity: operation.quantity,
    unitPrice: operation.unit_price,
    fees: operation.fees,
    taxes: operation.taxes,
    notes: operation.notes ?? "",
  };
}

/**
 * A trade whose amount is exactly quantity × unit price was computed by the
 * server, not typed. Loading it back as empty keeps it out of the PUT, so a
 * corrected quantity or price is priced again instead of inheriting the old
 * total (20 cotas por R$ 990).
 */
function isDerivedAmount(operation: InvestmentOperation): boolean {
  if (operation.quantity === null || operation.unit_price === null) return false;
  return isZeroDecimal(subtractDecimals(operation.amount, multiplyDecimals(operation.quantity, operation.unit_price)));
}

const kindsNeedingPosition: InvestmentOperationKind[] = ["buy", "sell", "valuation"];
const tradeKinds: InvestmentOperationKind[] = ["buy", "sell"];
// An integrated account's cash is the provider's: no cash-only opening
// balance, and trades only move its manual holdings.
const integratedKinds: InvestmentOperationKind[] = ["deposit", "withdrawal", "income", "fee", "tax", "buy", "sell", "valuation"];
const selectableKinds: InvestmentOperationKind[] = [
  "initial_balance", "deposit", "withdrawal", "buy", "sell", "income", "fee", "tax", "valuation",
];

function greaterThanZero(value: string | null): boolean {
  return value !== null && isPositiveDecimal(value);
}

function isNegative(value: string | null): boolean {
  return value !== null && compareDecimals(value, "0") < 0;
}

type FieldsProps = {
  /** The `<form>` id, so a submit button can live outside the fields (e.g. a sticky footer). */
  formId: string;
  /**
   * Render a plain container instead of a `<form>`: the caller is already
   * inside one (FormDrawer) and submits through `submitRef`.
   */
  bare?: boolean;
  /** Receives the fields' submit, for a caller whose own `<form>` wraps them. */
  submitRef?: MutableRefObject<(() => void) | null>;
  operation: InvestmentOperation | null;
  initial?: Partial<InvestmentOperationWrite> | null;
  accounts: InvestmentAccount[];
  positions: InvestmentPosition[];
  submitError: string | null;
  onSubmit: (write: InvestmentOperationWrite) => void;
  onSubmitCompound?: (writes: InvestmentOperationWrite[]) => void;
  allowedKinds?: InvestmentOperationKind[];
};

/**
 * The fields of a movimentação, without a container. The draft lives here
 * and is seeded once on mount: callers pass `initial` as a fresh literal on
 * every render and `accounts` changes identity on every refetch, and
 * neither may wipe what the person typed (a 409 on save re-renders the
 * parent). Remount (a `key`) to start over.
 *
 * Renders a `<form>` so the submit control can sit anywhere — the drawer's
 * footer, a panel's sticky bar — via `form={formId}`.
 */
export function InvestmentOperationFields({
  formId,
  bare = false,
  submitRef,
  operation,
  initial,
  accounts,
  positions,
  submitError,
  onSubmit,
  onSubmitCompound,
  allowedKinds,
}: FieldsProps) {
  const { t, language } = useTranslation();
  const timezone = useTimezone();
  const kindOptions = selectableKinds.map((value) => ({ value, label: investmentOperationKindName(value, language) }));
  const [draft, setDraft] = useState<Draft>(() => draftFrom(operation, accounts, timezone, initial ?? undefined));
  const [error, setError] = useState<string | null>(null);
  const [funding, setFunding] = useState<"deposit" | "income" | null>(null);
  // Whether the person typed into "Valor bruto" since mount. Until then a
  // trade's amount follows quantity × price, so changing either clears it.
  const [amountTouched, setAmountTouched] = useState(false);
  const integrated = accounts.find((account) => account.id === draft.accountId)?.kind === "integrated";
  const isEditing = operation !== null;
  const isTrade = tradeKinds.includes(draft.kind) || (draft.kind === "initial_balance" && draft.positionId !== null);
  const needsPosition = kindsNeedingPosition.includes(draft.kind) || (draft.kind === "initial_balance" && draft.positionId !== null);

  // Accounts may still be loading when the fields mount; fill the account in
  // once they arrive instead of leaving the select empty.
  useEffect(() => {
    if (accounts.length === 0) return;
    setDraft((current) => {
      if (current.accountId !== "") return current;
      const accountId = initial?.account_id ?? accounts.find((account) => account.active)?.id ?? "";
      return accountId === "" ? current : { ...current, accountId };
    });
    // `initial` is read, not tracked: a new literal must not re-run this.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [accounts]);

  const writableAccounts = useMemo(
    () => accounts.filter((account) => account.active),
    [accounts],
  );
  const accountOptions = (isEditing ? accounts : writableAccounts).map((account) => ({
    value: account.id,
    label: account.name,
  }));
  const positionOptions = positions
    .filter((position) => position.account_id === draft.accountId && position.source === "manual")
    .map((position) => ({
      value: position.id,
      label: [position.ticker, position.name].filter(Boolean).join(" · "),
    }));

  const setMoney = (field: "amount" | "fees" | "taxes", value: string | null) =>
    setDecimal(field, fromMoneyInput(value));

  const setDecimal = (
    field: "amount" | "quantity" | "unitPrice" | "fees" | "taxes",
    value: string | number | null,
  ) => {
    const next = value === null ? null : String(value);
    if (field === "amount") setAmountTouched(true);
    const clearsAmount = (field === "quantity" || field === "unitPrice") && !amountTouched;
    setDraft((current) => ({ ...current, [field]: next, ...(clearsAmount ? { amount: null } : {}) }));
  };

  const submit = () => {
    if (draft.accountId === "") {
      setError(t("selectInvestmentAccountRequired"));
      return;
    }
    if (needsPosition && draft.positionId === null) {
      setError(t("selectPositionForOperation"));
      return;
    }
    if (isTrade && (!greaterThanZero(draft.quantity) || !greaterThanZero(draft.unitPrice))) {
      setError(t("quantityPricePositive"));
      return;
    }
    if (!isTrade && draft.kind !== "valuation" && !greaterThanZero(draft.amount)) {
      setError(t("positiveAmountRequired"));
      return;
    }
    if (draft.kind === "valuation" && (draft.amount === null || compareDecimals(draft.amount, "0") < 0)) {
      setError(t("valuationTotalRequired"));
      return;
    }
    if (isNegative(draft.fees) || isNegative(draft.taxes)) {
      setError(t("feesTaxesNegative"));
      return;
    }
    setError(null);
    const write: InvestmentOperationWrite = {
      account_id: draft.accountId,
      position_id: needsPosition ? draft.positionId : null,
      kind: draft.kind,
      occurred_on: draft.occurredOn.format("YYYY-MM-DD"),
      amount: draft.amount ?? undefined,
      quantity: isTrade ? draft.quantity : null,
      unit_price: isTrade ? draft.unitPrice : null,
      fees: isTrade ? draft.fees : null,
      taxes: isTrade ? draft.taxes : null,
      notes: draft.notes.trim() || null,
    };
    if (funding && isTrade && draft.kind === "buy" && onSubmitCompound && !isEditing) {
      const amount = sumDecimals([draft.amount ?? multiplyDecimals(draft.quantity!, draft.unitPrice!), draft.fees ?? "0", draft.taxes ?? "0"]);
      onSubmitCompound([{ account_id: draft.accountId, position_id: null, kind: funding, occurred_on: write.occurred_on, amount }, write]);
    } else {
      onSubmit(write);
    }
  };

  // The drawer's own <form> (FormDrawer) submits through this; a standalone
  // `<form>` (the transaction panel) submits through its own onSubmit.
  useEffect(() => {
    if (submitRef) submitRef.current = submit;
  });

  // The alert sits at the top of a form that scrolls on a phone: bring it into view.
  const alertRef = useRef<HTMLDivElement>(null);
  const shownError = error ?? submitError;
  useEffect(() => {
    if (shownError) alertRef.current?.scrollIntoView?.({ block: "nearest" });
  }, [shownError]);

  const body = (
    <>
      {shownError && (
        <div ref={alertRef}>
          <Alert type="error" showIcon message={shownError} />
        </div>
      )}
      {draft.kind === "valuation" && (
        <p className="form-field-hint">{t("valuationHint")}</p>
      )}
      <FormField label={t("investmentAccountField")} htmlFor="investment-operation-account">
        <Select
          id="investment-operation-account"
          value={draft.accountId || undefined}
          options={accountOptions}
          disabled={isEditing}
          placeholder={t("selectInvestmentAccountPlaceholder")}
          onChange={(value: string) =>
            setDraft((current) => ({ ...current, accountId: value, positionId: null, kind: initial?.kind ?? "deposit" }))
          }
        />
      </FormField>
      <FormField label={t("type")} htmlFor="investment-operation-kind">
        <Select
          id="investment-operation-kind"
          value={draft.kind}
          options={kindOptions.filter(({ value }) => (!allowedKinds || allowedKinds.includes(value)) && (!integrated || integratedKinds.includes(value)))}
          disabled={isEditing}
          onChange={(value: InvestmentOperationKind) =>
            setDraft((current) => ({ ...current, kind: value }))
          }
        />
      </FormField>
      {(needsPosition || draft.kind === "initial_balance") && (
        <FormField
          label={needsPosition ? t("position") : t("positionOptional")}
          htmlFor="investment-operation-position"
        >
          <Select
            id="investment-operation-position"
            value={draft.positionId ?? undefined}
            options={positionOptions}
            allowClear={!needsPosition}
            placeholder={needsPosition ? t("selectAPosition") : t("cashOnlyMovement")}
            showSearch
            optionFilterProp="label"
            onChange={(value: string | undefined) =>
              setDraft((current) => ({ ...current, positionId: value ?? null }))
            }
          />
        </FormField>
      )}
      <FormField label={t("date")} htmlFor="investment-operation-date">
        <DatePicker
          id="investment-operation-date"
          style={{ width: "100%" }}
          value={draft.occurredOn}
          allowClear={false}
          onChange={(value) => value && setDraft((current) => ({ ...current, occurredOn: value }))}
        />
      </FormField>
      {isTrade ? (
        <>
          <div className="investment-form-pair">
            <FormField label={t("quantityLabel")} htmlFor="investment-operation-quantity">
              <InputNumber
                id="investment-operation-quantity"
                style={{ width: "100%" }}
                min="0.00000001"
                stringMode
                inputMode="decimal"
                decimalSeparator=","
                value={draft.quantity}
                onChange={(value) => setDecimal("quantity", value)}
              />
            </FormField>
            <FormField label={t("unitPriceLabel")} htmlFor="investment-operation-unit-price">
              <InputNumber
                id="investment-operation-unit-price"
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
          <FormField
            label={t("grossAmountOptional")}
            htmlFor="investment-operation-amount"
            hint={t("grossAmountHint")}
          >
            <MoneyInput
              id="investment-operation-amount"
              min="0"
              value={toMoneyInput(draft.amount)}
              onChange={(value) => setMoney("amount", value)}
            />
          </FormField>
          <div className="investment-form-pair">
            <FormField label={t("feesOptional")} htmlFor="investment-operation-fees">
              <MoneyInput
                id="investment-operation-fees"
                min="0"
                value={toMoneyInput(draft.fees)}
                onChange={(value) => setMoney("fees", value)}
              />
            </FormField>
            <FormField label={t("taxesOptional")} htmlFor="investment-operation-taxes">
              <MoneyInput
                id="investment-operation-taxes"
                min="0"
                value={toMoneyInput(draft.taxes)}
                onChange={(value) => setMoney("taxes", value)}
              />
            </FormField>
          </div>
        </>
      ) : (
        <FormField
          label={draft.kind === "valuation" ? t("currentTotalValue") : t("amount")}
          htmlFor="investment-operation-amount"
        >
          <MoneyInput
            id="investment-operation-amount"
            min="0"
            value={toMoneyInput(draft.amount)}
            onChange={(value) => setMoney("amount", value)}
          />
        </FormField>
      )}
      {isTrade && draft.kind === "buy" && !isEditing && onSubmitCompound && (
        <Flex vertical gap="small">
          <Checkbox checked={funding !== null} onChange={(event) => setFunding(event.target.checked ? "deposit" : null)}>
            {t("recordCashWithPurchase")}
          </Checkbox>
          {funding && <Select aria-label={t("cashSource")} value={funding} onChange={setFunding}
            options={[{ value: "deposit", label: t("contributionAndPurchase") }, { value: "income", label: t("reinvestedYield") }]} />}
        </Flex>
      )}
      {integrated && (
        <p className="form-field-hint">{t("integratedAccountHint")}</p>
      )}
      <FormField label={t("notesOptional")} htmlFor="investment-operation-notes">
        <Input.TextArea
          id="investment-operation-notes"
          rows={3}
          value={draft.notes}
          onChange={(event) => setDraft((current) => ({ ...current, notes: event.target.value }))}
        />
      </FormField>
      {isEditing && <p className="form-field-hint">{t("recalculationHint")}</p>}
    </>
  );

  return bare ? (
    <div className="form-drawer-form">{body}</div>
  ) : (
    <form
      id={formId}
      className="form-drawer-form"
      onSubmit={(event) => {
        event.preventDefault();
        submit();
      }}
    >
      {body}
    </form>
  );
}

const drawerFormId = "investment-operation-form";

/** The fields inside a side drawer — the Investimentos page's own editor. */
export function InvestmentOperationForm({
  open,
  submitting,
  onCancel,
  ...fields
}: Omit<FieldsProps, "formId" | "bare" | "submitRef"> & {
  open: boolean;
  submitting: boolean;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const isEditing = fields.operation !== null;
  const submitRef = useRef<(() => void) | null>(null);
  return (
    <FormDrawer
      title={isEditing ? t("editManualOperation") : t("recordOperation")}
      open={open}
      onClose={onCancel}
      onSubmit={() => submitRef.current?.()}
      submitting={submitting}
    >
      <InvestmentOperationFields
        key={fields.operation?.id ?? "new"}
        formId={drawerFormId}
        bare
        submitRef={submitRef}
        {...fields}
      />
    </FormDrawer>
  );
}
