import { createContext, useContext } from "react";
import { translate, type Language, type MessageKey } from "./messages";

export const languageStorageKey = "julius.language";
export const defaultLanguage: Language = "pt-BR";

export function storedLanguage(): Language {
  try {
    const value = localStorage.getItem(languageStorageKey);
    return value === "en" || value === "pt-BR" ? value : defaultLanguage;
  } catch {
    return defaultLanguage;
  }
}

export const I18nContext = createContext({
  language: defaultLanguage as Language,
  setLanguage: (() => {}) as (language: Language) => void,
});

export function useTranslation() {
  const { language, setLanguage } = useContext(I18nContext);
  return {
    language,
    setLanguage,
    t: (key: MessageKey) => translate(language, key),
  };
}
