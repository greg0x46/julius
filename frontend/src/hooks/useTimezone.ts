import { createContext, useContext } from "react";

import { browserTimezone } from "../presentation/timezone";

/** Set by TimezoneProvider to the zone in the owner's preferences. */
export const TimezoneContext = createContext<string | null>(null);

/**
 * The owner's configured timezone. The browser's own zone stands in only while
 * the preferences are loading or failed to load (and outside the provider).
 */
export function useTimezone(): string {
  return useContext(TimezoneContext) ?? browserTimezone();
}
