import { createContext, useCallback, useContext } from "react";
import { translate, type Language, type MessageKey } from "./messages";

export const languageStorageKey = "julius.language";
export const defaultLanguage: Language = "pt-BR";

// Last language chosen in this page, for when browser storage is blocked.
let sessionLanguage: Language | null = null;

export function rememberLanguage(language: Language): void {
  sessionLanguage = language;
}

export function storedLanguage(): Language {
  try {
    const value = localStorage.getItem(languageStorageKey);
    return value === "en" || value === "pt-BR" ? value : defaultLanguage;
  } catch {
    return sessionLanguage ?? defaultLanguage;
  }
}

export const I18nContext = createContext({
  language: defaultLanguage as Language,
  setLanguage: (() => {}) as (language: Language) => void,
});

export function useTranslation() {
  const { language, setLanguage } = useContext(I18nContext);
  const t = useCallback((key: MessageKey, values?: Record<string, string | number>) => translate(language, key, values), [language]);
  return {
    language,
    setLanguage,
    t,
  };
}
