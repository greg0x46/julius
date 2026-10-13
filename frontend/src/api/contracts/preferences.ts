import { requiredRecordWithOptionalKeys } from "./shared";

export type TransactionsPeriodBasis = "occurred_at" | "paid_at";

export interface Preferences {
  transactions_period_basis: TransactionsPeriodBasis;
  reference_currency: string;
}

export function parsePreferences(value: unknown): Preferences {
  const preferences = requiredRecordWithOptionalKeys(
    value,
    ["transactions_period_basis"],
    ["reference_currency"],
    "Preferências inválidas.",
  );
  if (
    preferences.transactions_period_basis !== "occurred_at" &&
    preferences.transactions_period_basis !== "paid_at"
  ) {
    throw new TypeError("Preferências inválidas.");
  }
  const code = preferences.reference_currency ?? "BRL";
  if (typeof code !== "string" || !/^[A-Z]{3}$/.test(code)) throw new TypeError("Preferências inválidas.");
  return {
    transactions_period_basis: preferences.transactions_period_basis,
    reference_currency: code,
  };
}
