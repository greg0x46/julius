import { translate, type Language } from "../../i18n/messages";
import { formatDateOnly } from "../../presentation/dates";

/** A calendar day from the API through the shared formatter, or "Sem registro" when there is none. */
export function formatDate(value: string | null, language: Language = "pt-BR"): string {
  return value === null ? translate(language, "investmentNoRecord") : formatDateOnly(value, language);
}
