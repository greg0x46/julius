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
  return (
    <PageAction
      icon={<SyncOutlined aria-hidden />}
      label="Sincronizar agora"
      shortLabel="Sincronizar"
      loading={sync.state.kind === "submitting"}
      onClick={() => void sync.submit()}
    />
  );
}

/** What the last sync request did — started, already running, or not confirmed. Renders nothing while idle. */
export function SyncRunNotice({ sync }: { sync: Sync }) {
  const { state, reset } = sync;
  const feedback = useFeedback();

  // The notice sits at the top of the page, but the action may have been a
  // row's menu further down: a toast says what happened wherever the user is.
  useEffect(() => {
    if (state.kind === "started") {
      feedback.success(
        state.runs.length < state.requested ? "Sincronização iniciada em parte das conexões" : "Sincronização iniciada",
      );
    } else if (state.kind === "conflict") {
      feedback.info(
        state.scope === "one" ? "Essa conexão já está sincronizando" : "Já há uma sincronização em andamento",
      );
    } else if (state.kind === "uncertain") {
      feedback.error("Não foi possível confirmar se a sincronização começou");
    }
    // Only a change of state is worth a toast, not a new `state` object of the same kind.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state.kind, feedback]);

  return (
    <div aria-live="polite" aria-atomic="true">
      {state.kind === "submitting" && <p role="status">Solicitando a sincronização…</p>}
      {state.kind === "started" && (
        <Alert
          type={state.runs.length < state.requested ? "warning" : "success"}
          showIcon
          message={
            state.runs.length < state.requested
              ? `Sincronização iniciada em ${state.runs.length} de ${state.requested} conexões.`
              : `Sincronização iniciada em ${state.runs.length} conexões.`
          }
          description={
            <Flex vertical align="start" gap="small">
              {state.runs.length < state.requested && (
                <span>Uma ou mais conexões não iniciaram a sincronização. Tente novamente em instantes.</span>
              )}
              {state.runs.map((run) => (
                <Link key={run.id} to={`/configuracoes/open-banking/sync-runs/${run.id}`}>
                  Acompanhar {run.source_name}
                </Link>
              ))}
              <Button type="link" onClick={reset}>
                Fechar aviso
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
              ? "Essa conexão já está sincronizando."
              : "Todas as conexões já estão sincronizando."
          }
          description={
            <Flex vertical align="start" gap="small">
              {state.activeRunId !== null && (
                <Link to={`/configuracoes/open-banking/sync-runs/${state.activeRunId}`}>
                  Acompanhar sincronização ativa
                </Link>
              )}
              <Button type="link" onClick={reset}>
                Fechar aviso
              </Button>
            </Flex>
          }
        />
      )}
      {state.kind === "uncertain" && (
        <Alert
          type="error"
          showIcon
          message="Não foi possível confirmar se a sincronização começou."
          description={
            <Flex vertical align="start" gap="small">
              <span>Consulte o histórico antes de tentar novamente.</span>
              <Button onClick={reset}>Tentar novamente</Button>
            </Flex>
          }
        />
      )}
    </div>
  );
}
