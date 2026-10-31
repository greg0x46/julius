import { requiredRecordWithOptionalKeys } from "./shared";

/** What the server uses until the owner picks a zone. */
export const defaultTimezone = "America/Sao_Paulo";

export type TransactionsPeriodBasis = "occurred_at" | "paid_at";

export interface Preferences {
  transactions_period_basis: TransactionsPeriodBasis;
  reference_currency: string;
  /** IANA zone name. */
  timezone: string;
}

// A name shape only: whether this browser knows the zone is the hook's concern.
const timezonePattern = /^[A-Za-z][A-Za-z0-9_+-]*(\/[A-Za-z0-9_+-]+)*$/;

export function parsePreferences(value: unknown): Preferences {
  const preferences = requiredRecordWithOptionalKeys(
    value,
    ["transactions_period_basis"],
    ["reference_currency", "timezone"],
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
  const timezone = preferences.timezone ?? defaultTimezone;
  if (typeof timezone !== "string" || timezone.length > 64 || !timezonePattern.test(timezone)) {
    throw new TypeError("Preferências inválidas.");
  }
  return {
    transactions_period_basis: preferences.transactions_period_basis,
    reference_currency: code,
    timezone,
  };
}
