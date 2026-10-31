import { translate, type Language } from "../i18n/messages";
export function movementTypeLabel(type: string | null, language: Language = "pt-BR"): string {
  if (!type) return translate(language, "unknownAccountType");
  if (type === "CREDIT") return translate(language, "credit");
  if (type === "DEBIT") return translate(language, "debit");
  return type;
}
