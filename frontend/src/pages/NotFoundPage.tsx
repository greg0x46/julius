import { useTranslation } from "../i18n/context";
import { Button } from "../components/ui";
import { useNavigate } from "react-router-dom";

import { EmptyState, Page } from "../components/layout";


export function NotFoundPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  return (
    <Page title={t("notFoundTitle")} width="narrow">
      <EmptyState
        title={t("notFoundDescription")}
        action={<Button onClick={() => navigate("/")}>{t("backHome")}</Button>}
      />
    </Page>
  );
}
