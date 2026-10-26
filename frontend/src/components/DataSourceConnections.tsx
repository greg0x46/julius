import { Alert, Button, Collapse, Input, Skeleton, Switch, Typography } from "./ui";
import { useEffect, useState } from "react";

import type { DataSource, SyncRun } from "../api/contracts";
import { ApiError } from "../api/problems";
import { UnavailableState } from "./AsyncState";
import { FormDrawer } from "./forms/FormDrawer";
import { FormField } from "./forms/FormField";
import { EmptyState, Section } from "./layout";
import { RecordMenu } from "./shared/RecordMenu";
import { useFeedback } from "./shared/useFeedback";
import type { useCreateSyncRun } from "../hooks/useCreateSyncRun";
import { useDataSources } from "../hooks/useDataSources";
import { formatOptionalDateTime } from "../presentation/dates";

function errorMessage(error: unknown, fallback: string): string {
  if (error instanceof ApiError) {
    return error.problem?.detail ?? error.problem?.title ?? fallback;
  }
  return fallback;
}

/** The most recent run of one connection, or null when there is none (yet). */
function latestRunOf(source: DataSource, runs: SyncRun[]): SyncRun | null {
  let latest: SyncRun | null = null;
  for (const run of runs) {
    if (run.source_id !== source.id) continue;
    if (latest === null || new Date(run.started_at) > new Date(latest.started_at)) latest = run;
  }
  return latest;
}

/**
 * The meta line of a connection: the institution when the user gave it their
 * own name, "Pausada" only when it is (the default is active), and when it
 * last synced. `runs` is undefined while the history is not known, in which
 * case nothing is claimed about the last sync.
 */
function connectionMeta(source: DataSource, runs: SyncRun[] | undefined): string {
  const parts: string[] = [];
  if (source.label && source.display_name && source.display_name !== source.label) {
    parts.push(source.display_name);
  }
  if (!source.is_active) parts.push("Pausada");
  if (runs !== undefined) {
    const latest = latestRunOf(source, runs);
    parts.push(latest ? `Última sincronização ${formatOptionalDateTime(latest.started_at)}` : "Nunca sincronizada");
  }
  return parts.join(" · ");
}

/**
 * The registry of banks Julius syncs. Each connection is one Pluggy item,
 * created in the Pluggy dashboard and pasted here — adding a second bank is a
 * row, not a reinstall.
 *
 * A connection is a flat row: tapping its name and meta line opens the edit
 * sheet, the on/off switch and a `···` (Editar, Sincronizar agora) sit at the
 * right edge. Syncing one bank is rare next to syncing them all, which is the
 * page's own title-row action, so it is a menu item rather than a button on
 * every row. The raw Pluggy Item ID is a technical detail, so it lives in the
 * edit sheet, behind "Detalhes técnicos", and in the "Adicionar conexão"
 * sheet where it is needed.
 */
export function DataSourceConnections({
  sync,
  runs,
}: {
  sync: ReturnType<typeof useCreateSyncRun>;
  /** The sync history, to show each connection's last sync; undefined while unknown. */
  runs?: SyncRun[];
}) {
  const { state, retry, create, update } = useDataSources();
  const feedback = useFeedback();
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<DataSource | null>(null);

  const openAdding = () => {
    create.reset();
    setAdding(true);
  };
  const closeAdding = () => {
    setAdding(false);
    create.reset();
  };
  const openEditing = (source: DataSource) => {
    update.reset();
    setEditing(source);
  };
  const closeEditing = () => {
    setEditing(null);
    update.reset();
  };

  const addButton = (
    <Button type="link" onClick={openAdding}>
      Adicionar conexão
    </Button>
  );

  return (
    <Section title="Conexões" trailing={state.kind === "ready" ? addButton : undefined}>
      {state.kind === "loading" && (
        <div role="status" aria-label="Carregando conexões">
          <Skeleton active paragraph={{ rows: 2 }} />
        </div>
      )}
      {state.kind === "unavailable" && (
        <UnavailableState onRetry={retry}>
          As conexões estão temporariamente indisponíveis. Isso não significa que não existam.
        </UnavailableState>
      )}
      {state.kind === "empty" && (
        <EmptyState
          title="Nenhuma conexão ainda"
          hint="Adicione o Item ID de um Item criado no painel do Pluggy para começar a sincronizar."
          action={<Button onClick={openAdding}>Adicionar conexão</Button>}
        />
      )}
      {state.kind === "ready" && (
        <ul className="connection-list" aria-label="Conexões">
          {state.sources.map((source) => (
            <li key={source.id} className="connection-row">
              <button
                type="button"
                className="connection-row-main"
                aria-label={`Editar ${source.name}`}
                onClick={() => openEditing(source)}
              >
                <span className="connection-row-name">{source.name}</span>
                <span className="connection-row-meta">{connectionMeta(source, runs)}</span>
              </button>
              <div className="connection-row-controls">
                <Switch
                  aria-label={`Conexão ativa: ${source.name}`}
                  checked={source.is_active}
                  loading={update.isPending && update.variables?.id === source.id}
                  onChange={(checked) =>
                    update.mutate(
                      { id: source.id, changes: { is_active: checked } },
                      {
                        onSuccess: () => feedback.success(checked ? "Conexão ativada" : "Conexão pausada"),
                        onError: (error) =>
                          feedback.error(errorMessage(error, "Não foi possível atualizar a conexão.")),
                      },
                    )
                  }
                />
                <RecordMenu
                  label={`Ações de ${source.name}`}
                  items={[
                    { key: "edit", label: "Editar", onClick: () => openEditing(source) },
                    {
                      key: "sync",
                      label: "Sincronizar agora",
                      disabled: !source.is_active || sync.state.kind === "submitting",
                      onClick: () => void sync.submit(source.id),
                    },
                  ]}
                />
              </div>
            </li>
          ))}
        </ul>
      )}

      {update.isError && editing === null && (
        <Alert
          type="error"
          showIcon
          message={errorMessage(update.error, "Não foi possível atualizar a conexão.")}
        />
      )}

      <NewConnectionSheet
        open={adding}
        submitting={create.isPending}
        error={create.isError ? errorMessage(create.error, "Não foi possível adicionar a conexão.") : null}
        onClose={closeAdding}
        onSubmit={(input) =>
          create.mutate(input, {
            onSuccess: () => {
              setAdding(false);
              feedback.success("Conexão adicionada");
            },
          })
        }
      />
      <EditConnectionSheet
        key={editing?.id ?? "none"}
        source={editing}
        submitting={update.isPending}
        error={update.isError ? errorMessage(update.error, "Não foi possível atualizar a conexão.") : null}
        onClose={closeEditing}
        onSubmit={(label) => {
          if (editing === null) return;
          // Saving without touching the nickname must not rewrite it: a
          // connection with no label would otherwise freeze its institution
          // name into a permanent label.
          if (label === (editing.label ?? "")) {
            closeEditing();
            return;
          }
          update.mutate(
            { id: editing.id, changes: { label } },
            {
              onSuccess: () => {
                setEditing(null);
                feedback.success("Conexão salva");
              },
            },
          );
        }}
      />
    </Section>
  );
}

function NewConnectionSheet({
  open,
  submitting,
  error,
  onClose,
  onSubmit,
}: {
  open: boolean;
  submitting: boolean;
  error: string | null;
  onClose: () => void;
  onSubmit: (input: { external_item_id: string; label: string | null }) => void;
}) {
  const [itemId, setItemId] = useState("");
  const [label, setLabel] = useState("");
  const [validation, setValidation] = useState<string | null>(null);

  // Every opening starts blank: a half-typed Item ID left over from a closed
  // or finished sheet would let a second tap add the same connection twice.
  useEffect(() => {
    if (open) {
      setItemId("");
      setLabel("");
      setValidation(null);
    }
  }, [open]);

  const submit = () => {
    if (itemId.trim() === "") {
      setValidation("Informe o Item ID do Pluggy.");
      return;
    }
    setValidation(null);
    onSubmit({ external_item_id: itemId.trim(), label: label.trim() === "" ? null : label.trim() });
  };

  return (
    <FormDrawer
      title="Nova conexão"
      open={open}
      onClose={onClose}
      onSubmit={submit}
      submitting={submitting}
      submitLabel="Adicionar"
      error={validation ?? error}
    >
      <FormField
        label="Item ID (Pluggy)"
        htmlFor="connection-item-id"
        hint="Crie o Item no painel do Pluggy e cole o Item ID aqui para que ele seja sincronizado junto com os demais."
      >
        <Input id="connection-item-id" value={itemId} onChange={(event) => setItemId(event.target.value)} />
      </FormField>
      <FormField label="Apelido (opcional)" htmlFor="connection-label">
        <Input
          id="connection-label"
          value={label}
          placeholder="Ex.: Conta pessoal"
          onChange={(event) => setLabel(event.target.value)}
        />
      </FormField>
    </FormDrawer>
  );
}

function EditConnectionSheet({
  source,
  submitting,
  error,
  onClose,
  onSubmit,
}: {
  source: DataSource | null;
  submitting: boolean;
  error: string | null;
  onClose: () => void;
  onSubmit: (label: string) => void;
}) {
  // The parent keys this sheet by connection, so each one opens from its own nickname.
  const [label, setLabel] = useState(source?.label ?? "");

  return (
    <FormDrawer
      title="Editar conexão"
      open={source !== null}
      onClose={onClose}
      onSubmit={() => onSubmit(label.trim())}
      submitting={submitting}
      error={error}
    >
      <FormField
        label="Apelido"
        htmlFor="connection-edit-label"
        hint="O nome que aparece na lista. Deixe em branco para usar o nome da instituição."
      >
        <Input
          id="connection-edit-label"
          value={label}
          placeholder={source?.display_name ?? "Ex.: Conta pessoal"}
          onChange={(event) => setLabel(event.target.value)}
        />
      </FormField>
      <Collapse
        ghost
        items={[
          {
            key: "technical",
            label: "Detalhes técnicos",
            children: (
              <div className="connection-technical">
                <span className="form-field-label">Item ID (Pluggy)</span>
                <Typography.Text copyable code>
                  {source?.external_item_id}
                </Typography.Text>
              </div>
            ),
          },
        ]}
      />
    </FormDrawer>
  );
}
