import { isPositiveDecimal } from "../../presentation/decimal";
import { DatePicker, Input } from "antd";
import dayjs, { type Dayjs } from "dayjs";
import { useEffect, useState } from "react";

import type { InvestmentPortfolio, InvestmentPortfolioWrite } from "../../api/contracts";
import { FormDrawer } from "../forms/FormDrawer";
import { FormField } from "../forms/FormField";
import { MoneyInput } from "../forms/MoneyInput";
import { fromMoneyInput, toMoneyInput } from "./moneyDraft";

type Draft = {
  name: string;
  targetAmount: string | null;
  targetDate: Dayjs | null;
  notes: string;
};

function draftFrom(portfolio: InvestmentPortfolio | null): Draft {
  if (!portfolio) return { name: "", targetAmount: null, targetDate: null, notes: "" };
  return {
    name: portfolio.name,
    targetAmount: portfolio.target_amount,
    targetDate: portfolio.target_date ? dayjs(portfolio.target_date) : null,
    notes: portfolio.notes ?? "",
  };
}

export function InvestmentPortfolioForm({
  open,
  portfolio,
  submitting,
  submitError,
  onSubmit,
  onCancel,
}: {
  open: boolean;
  portfolio: InvestmentPortfolio | null;
  submitting: boolean;
  submitError: string | null;
  onSubmit: (write: InvestmentPortfolioWrite) => void;
  onCancel: () => void;
}) {
  const [draft, setDraft] = useState<Draft>(() => draftFrom(portfolio));
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setDraft(draftFrom(portfolio));
      setError(null);
    }
  }, [open, portfolio]);

  const submit = () => {
    if (draft.name.trim() === "") {
      setError("Informe o nome do objetivo.");
      return;
    }
    if (draft.targetAmount !== null && !isPositiveDecimal(draft.targetAmount)) {
      setError("A meta precisa ser maior que zero.");
      return;
    }
    setError(null);
    onSubmit({
      name: draft.name.trim(),
      target_amount: draft.targetAmount,
      target_date: draft.targetDate?.format("YYYY-MM-DD") ?? null,
      notes: draft.notes.trim() === "" ? null : draft.notes.trim(),
    });
  };

  return (
    <FormDrawer
      title={portfolio ? "Editar objetivo" : "Novo objetivo"}
      open={open}
      onClose={onCancel}
      onSubmit={submit}
      submitting={submitting}
      error={error ?? submitError}
    >
      <FormField label="Nome do objetivo" htmlFor="investment-portfolio-name">
        <Input
          id="investment-portfolio-name"
          value={draft.name}
          onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))}
          placeholder="Ex.: Reserva de emergência"
          autoFocus
        />
      </FormField>
      <FormField label="Meta (opcional)" htmlFor="investment-portfolio-target">
        <MoneyInput
          id="investment-portfolio-target"
          min="0"
          value={toMoneyInput(draft.targetAmount)}
          onChange={(value) => setDraft((current) => ({ ...current, targetAmount: fromMoneyInput(value) }))}
        />
      </FormField>
      <FormField label="Data-alvo (opcional)" htmlFor="investment-portfolio-date">
        <DatePicker
          id="investment-portfolio-date"
          style={{ width: "100%" }}
          format="DD/MM/YYYY"
          value={draft.targetDate}
          onChange={(value) => setDraft((current) => ({ ...current, targetDate: value }))}
        />
      </FormField>
      <FormField label="Observações (opcional)" htmlFor="investment-portfolio-notes">
        <Input.TextArea
          id="investment-portfolio-notes"
          value={draft.notes}
          rows={4}
          onChange={(event) => setDraft((current) => ({ ...current, notes: event.target.value }))}
          placeholder="Para que serve este objetivo?"
        />
      </FormField>
    </FormDrawer>
  );
}
