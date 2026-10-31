import { useTranslation } from "../i18n/context";
import { useQuery } from "@tanstack/react-query";

import { getSession } from "../api/auth";
import { queryKeys } from "../api/queryKeys";

/** Shown only when the server runs as the public demo (same query as AuthGate). */
export function DemoNotice() {
  const { t } = useTranslation();
  const { data } = useQuery({ queryKey: queryKeys.authSession, queryFn: ({ signal }) => getSession(signal) });
  if (!data?.demo) return null;
  return (
    <div className="demo-notice" role="note">
      <strong>{t("demoTitle")}</strong> {t("demoDescription")}
    </div>
  );
}
