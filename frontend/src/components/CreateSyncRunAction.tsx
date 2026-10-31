import { useTranslation } from "../i18n/context";
import { Alert, Button, Flex } from "./ui";
import { SyncOutlined } from "./ui/icons";
import { useEffect } from "react";
import { Link } from "react-router-dom";

import type { useCreateSyncRun } from "../hooks/useCreateSyncRun";
import { PageAction } from "./layout";
import { useFeedback } from "./shared/useFeedback";

type Sync = ReturnType<typeof useCreateSyncRun>;

// The sync state is owned by the page and shared with the connection list, so
// one lock and one notice cover both "refresh everything" and "refresh this
// bank" rather than the two disagreeing about what is running.

/**
 * "Sincronizar agora" as a page action in the title row: a command that
 * refreshes every active connection does not need a card of its own around a
 * single button.
 */
export function SyncNowAction({ sync }: { sync: Sync }) {
  const { t } = useTranslation();
  return (
    <PageAction
      icon={<SyncOutlined aria-hidden />}
      label={t("syncNow")}
      shortLabel={t("syncAction")}
      loading={sync.state.kind === "submitting"}
      onClick={() => void sync.submit()}
    />
  );
}

/** What the last sync request did — started, already running, or not confirmed. Renders nothing while idle. */
export function SyncRunNotice({ sync }: { sync: Sync }) {
  const { t } = useTranslation();
  const { state, reset } = sync;
  const feedback = useFeedback();

  // The notice sits at the top of the page, but the action may have been a
  // row's menu further down: a toast says what happened wherever the user is.
  useEffect(() => {
    if (state.kind === "started") {
      feedback.success(
        state.runs.length < state.requested ? t("syncStartedPart") : t("syncStarted"),
      );
    } else if (state.kind === "conflict") {
      feedback.info(
        state.scope === "one" ? t("connectionAlreadySyncing") : t("syncAlreadyRunning"),
      );
    } else if (state.kind === "uncertain") {
      feedback.error(t("syncUncertain"));
    }
    // Only a change of state is worth a toast, not a new `state` object of the same kind.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state.kind, feedback]);

  return (
    <div aria-live="polite" aria-atomic="true">
      {state.kind === "submitting" && <p role="status">{t("syncRequesting")}</p>}
      {state.kind === "started" && (
        <Alert
          type={state.runs.length < state.requested ? "warning" : "success"}
          showIcon
          message={
            state.runs.length < state.requested
              ? t("syncStartedPartialCount").replace("{started}", String(state.runs.length)).replace("{requested}", String(state.requested))
              : t("syncStartedCount").replace("{started}", String(state.runs.length))
          }
          description={
            <Flex vertical align="start" gap="small">
              {state.runs.length < state.requested && (
                <span>{t("syncSomeNotStarted")}</span>
              )}
              {state.runs.map((run) => (
                <Link key={run.id} to={`/configuracoes/open-banking/sync-runs/${run.id}`}>
                  {t("followConnection", { name: run.source_name })}
                </Link>
              ))}
              <Button type="link" onClick={reset}>
                {t("closeNotice")}
              </Button>
            </Flex>
          }
        />
      )}
      {state.kind === "conflict" && (
        <Alert
          type="warning"
          showIcon
          message={
            state.scope === "one"
              ? t("connectionAlreadySyncingFull")
              : t("allConnectionsSyncing")
          }
          description={
            <Flex vertical align="start" gap="small">
              {state.activeRunId !== null && (
                <Link to={`/configuracoes/open-banking/sync-runs/${state.activeRunId}`}>
                  {t("followActiveSync")}
                </Link>
              )}
              <Button type="link" onClick={reset}>
                {t("closeNotice")}
              </Button>
            </Flex>
          }
        />
      )}
      {state.kind === "uncertain" && (
        <Alert
          type="error"
          showIcon
          message={t("syncUncertainFull")}
          description={
            <Flex vertical align="start" gap="small">
              <span>{t("checkHistoryBeforeRetry")}</span>
              <Button onClick={reset}>{t("retry")}</Button>
            </Flex>
          }
        />
      )}
    </div>
  );
}
