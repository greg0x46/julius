import { useTranslation } from "../i18n/context";
import { useParams } from "react-router-dom";

import { isUuid } from "../api/contracts";
import { DetailPage, InvalidDetailPage } from "../components/layout";
import { SyncFailureList } from "../components/SyncFailureList";
import { SyncRunOverview } from "../components/SyncRunOverview";
import { useSyncRun } from "../hooks/useSyncRun";

const backTo = "/configuracoes/open-banking";

function ValidRunDetail({ id }: { id: string }) {
  const { t } = useTranslation();
  const { state, retry } = useSyncRun(id);

  return (
    <DetailPage
      title={t("sync")}
      backTo={backTo}
      backLabel={t("backToSyncs")}
      width="narrow"
      state={state}
      retry={retry}
      loadingLabel={t("loadingSync")}
      notFoundMessage={t("syncNotFound")}
      notFoundDescription={t("syncNotFoundDescription")}
      unavailableMessage={t("syncUnavailable")}
    >
      {(snapshot) => (
        <>
          <SyncRunOverview run={snapshot} />
          <SyncFailureList failures={snapshot.failures} />
        </>
      )}
    </DetailPage>
  );
}

export function SyncRunDetailPage() {
  const { t } = useTranslation();
  const { id = "" } = useParams();
  return isUuid(id) ? (
    <ValidRunDetail id={id} />
  ) : (
    <InvalidDetailPage
      title={t("sync")}
      backTo={backTo}
      backLabel={t("backToSyncs")}
      width="narrow"
      invalidTitle={t("invalidSyncAddress")}
    />
  );
}
