import { PluggySettings } from "../components/PluggySettings";
import { SettingsPageContainer as PageContainer } from "../components/SettingsPageContainer";
import { Collapse, Skeleton } from "../components/ui";
import { useNavigate } from "react-router-dom";

import { SyncNowAction, SyncRunNotice } from "../components/CreateSyncRunAction";
import { DataSourceConnections } from "../components/DataSourceConnections";
import { UnavailableState } from "../components/AsyncState";
import { EmptyState, Section } from "../components/layout";
import { SyncRunHistory } from "../components/SyncRunHistory";
import { useCompactScreen } from "../components/shared/useCompactScreen";
import { useCreateSyncRun } from "../hooks/useCreateSyncRun";
import { useSyncRunList } from "../hooks/useSyncRunList";

/**
 * Open Banking: the connections, the history of their syncs and, tucked
 * behind a disclosure, the Pluggy credentials. "Sincronizar agora" is the
 * page's one action, in the title row.
 */
export function SyncRunListPage() {
  const { state, retry } = useSyncRunList();
  const navigate = useNavigate();
  const compact = useCompactScreen();
  const sync = useCreateSyncRun((id) => navigate(`/configuracoes/open-banking/sync-runs/${id}`));

  const knownRuns = state.kind === "ready" ? state.runs : state.kind === "empty" ? [] : undefined;

  return (
    <PageContainer
      title="Open Banking"
      subTitle="Busque os dados mais recentes de todas as conexões ativas"
      compactMobileHeader
      extra={<SyncNowAction sync={sync} />}
    >
      <div className="sync-runs-page">
        <SyncRunNotice sync={sync} />
        <DataSourceConnections sync={sync} runs={knownRuns} />
        {/* The table runs edge to edge in its section; a phone's rows keep the section's gutter. */}
        <Section title="Histórico de sincronizações" flush={state.kind === "ready" && !compact}>
          {state.kind === "loading" && (
            <div role="status" aria-label="Carregando histórico">
              <Skeleton active paragraph={{ rows: 3 }} />
            </div>
          )}
          {state.kind === "empty" && (
            <EmptyState
              title="Nenhuma sincronização ainda"
              hint="Sincronize para trazer os dados mais recentes das conexões ativas."
            />
          )}
          {state.kind === "unavailable" && (
            <UnavailableState onRetry={retry}>
              O histórico está temporariamente indisponível. Isso não significa que esteja vazio.
            </UnavailableState>
          )}
          {state.kind === "ready" && <SyncRunHistory runs={state.runs} />}
        </Section>
        <Collapse
          className="sync-credentials"
          items={[{ key: "credentials", label: "Credenciais da Pluggy", children: <PluggySettings /> }]}
        />
      </div>
    </PageContainer>
  );
}
