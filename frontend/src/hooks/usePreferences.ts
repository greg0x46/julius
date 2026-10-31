import { useQuery } from "@tanstack/react-query";

import { getPreferences } from "../api/preferences";
import { queryKeys } from "../api/queryKeys";

/** The owner's preferences. One shared query: the settings page and the timezone provider read the same data. */
export function usePreferences() {
  return useQuery({
    queryKey: queryKeys.preferences,
    queryFn: ({ signal }) => getPreferences(signal),
  });
}
