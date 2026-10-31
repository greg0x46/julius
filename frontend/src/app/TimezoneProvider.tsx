import { useEffect, useState, type ReactNode } from "react";

import { usePreferences } from "../hooks/usePreferences";
import { TimezoneContext } from "../hooks/useTimezone";
import { resolveTimezone } from "../presentation/timezone";
import { AppLoadingScreen } from "./AppLoadingScreen";

/**
 * Supplies the configured timezone to the app. The first paint waits for the
 * preferences: a page that opened on the browser's zone would already have
 * built its "today", its period and its form defaults on the wrong day. A
 * failed request does not block: the browser zone stands in.
 */
export function TimezoneProvider({ children }: { children: ReactNode }) {
  const preferences = usePreferences();
  // Only the first load blocks; a later retry after a failure must not unmount the app.
  const [settled, setSettled] = useState(false);
  useEffect(() => {
    if (!preferences.isLoading) setSettled(true);
  }, [preferences.isLoading]);
  if (!settled && preferences.isLoading) return <AppLoadingScreen />;
  return (
    <TimezoneContext.Provider value={resolveTimezone(preferences.data?.timezone)}>
      {children}
    </TimezoneContext.Provider>
  );
}
