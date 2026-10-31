export const messages = {
  "pt-BR": {
    language: "Idioma",
    brandSubtitle: "De graça !?",
    home: "Início",
    transactions: "Transações",
    accounts: "Contas e cartões",
    payables: "Pendências",
    recurrences: "Recorrências",
    investments: "Investimentos",
    goals: "Metas",
    netWorth: "Patrimônio",
    settings: "Configurações",
    navigation: "Navegação principal",
    openNavigation: "Abrir navegação",
    darkTheme: "Tema escuro",
    skipToContent: "Pular para o conteúdo",
  },
  en: {
    language: "Language",
    brandSubtitle: "For free!?",
    home: "Home",
    transactions: "Transactions",
    accounts: "Accounts and cards",
    payables: "Payables and receivables",
    recurrences: "Recurring commitments",
    investments: "Investments",
    goals: "Goals",
    netWorth: "Net worth",
    settings: "Settings",
    navigation: "Main navigation",
    openNavigation: "Open navigation",
    darkTheme: "Dark theme",
    skipToContent: "Skip to content",
  },
} as const;

export type Language = keyof typeof messages;
export type MessageKey = keyof typeof messages["pt-BR"];

export function translate(language: Language, key: MessageKey): string {
  return messages[language][key];
}
