import { useCallback, useEffect, useState, type ReactNode } from "react";
import { I18nContext, languageStorageKey, storedLanguage } from "./context";
import type { Language } from "./messages";

export function I18nProvider({ children }: { children: ReactNode }) {
  const [language, setLanguageState] = useState(storedLanguage);
  const setLanguage = useCallback((next: Language) => {
    setLanguageState(next);
    try {
      localStorage.setItem(languageStorageKey, next);
    } catch {
      // Keep the selection usable when browser storage is blocked.
    }
  }, []);

  useEffect(() => {
    document.documentElement.lang = language;
  }, [language]);

  useEffect(() => {
    const onStorage = (event: StorageEvent) => {
      if (event.key === languageStorageKey || event.key === null) {
        setLanguageState(storedLanguage());
      }
    };
    window.addEventListener("storage", onStorage);
    return () => window.removeEventListener("storage", onStorage);
  }, []);

  return (
    <I18nContext.Provider value={{ language, setLanguage }}>
      {children}
    </I18nContext.Provider>
  );
}
