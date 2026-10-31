import { common } from "./catalogs/common";
import { settings } from "./catalogs/settings";
import { periods } from "./catalogs/periods";
import { accounts } from "./catalogs/accounts";
import { home } from "./catalogs/home";
import { openBanking } from "./catalogs/openBanking";
import { goals } from "./catalogs/goals";
import { transactions } from "./catalogs/transactions";
import { categories } from "./catalogs/categories";
import { payables } from "./catalogs/payables";
import { investments } from "./catalogs/investments";
import { scenarios } from "./catalogs/scenarios";
import { automations } from "./catalogs/automations";
import { recurrences } from "./catalogs/recurrences";
import { netWorth } from "./catalogs/netWorth";
import { statementImport } from "./catalogs/statementImport";
import { api } from "./catalogs/api";

/** One catalog per area in `catalogs/`, each with both languages side by side. */
export const messages = {
  "pt-BR": {
    ...common["pt-BR"],
    ...settings["pt-BR"],
    ...periods["pt-BR"],
    ...accounts["pt-BR"],
    ...home["pt-BR"],
    ...openBanking["pt-BR"],
    ...goals["pt-BR"],
    ...transactions["pt-BR"],
    ...categories["pt-BR"],
    ...payables["pt-BR"],
    ...investments["pt-BR"],
    ...scenarios["pt-BR"],
    ...automations["pt-BR"],
    ...recurrences["pt-BR"],
    ...netWorth["pt-BR"],
    ...statementImport["pt-BR"],
    ...api["pt-BR"],
  },
  en: {
    ...common["en"],
    ...settings["en"],
    ...periods["en"],
    ...accounts["en"],
    ...home["en"],
    ...openBanking["en"],
    ...goals["en"],
    ...transactions["en"],
    ...categories["en"],
    ...payables["en"],
    ...investments["en"],
    ...scenarios["en"],
    ...automations["en"],
    ...recurrences["en"],
    ...netWorth["en"],
    ...statementImport["en"],
    ...api["en"],
  },
} as const;

export type Language = keyof typeof messages;
export type MessageKey = keyof typeof messages["pt-BR"];

export function translate(language: Language, key: MessageKey, values: Record<string, string | number> = {}): string {
  return messages[language][key].replace(/\{([a-zA-Z]+)\}/g, (placeholder, name: string) =>
    Object.hasOwn(values, name) ? String(values[name]) : placeholder,
  );
}
