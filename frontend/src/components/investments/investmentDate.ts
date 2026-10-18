import { formatDateOnly } from "../../presentation/dates";

/** A calendar day from the API through the shared formatter, or "Sem registro" when there is none. */
export function formatDate(value: string | null): string {
  return value === null ? "Sem registro" : formatDateOnly(value);
}
