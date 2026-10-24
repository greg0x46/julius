import { Alert, Button, Flex, Skeleton, Space, Table, Typography } from "../ui";
import type { ColumnsType } from "../ui/Table";
import { useState } from "react";

import type { RecurrenceOccurrence, RecurringCommitment, RecurringCommitmentKind } from "../../api/contracts";
import { errorMessage } from "../../presentation/errors";
import { formatDay, formatOptionalLocalDay } from "../../presentation/dates";
import { formatBRL } from "../../presentation/money";
import {
  reconciliationLabel,
  reconciliationStatusTone,
} from "../../presentation/reconciliationLabels";
import {
  useReconciliationCandidates,
  useRecurrenceOccurrences,
} from "../../hooks/useRecurrenceOccurrences";
import { FormDrawer } from "../forms/FormDrawer";
import { Money } from "../shared/Money";
import { RecordMenu, type RecordMenuItem } from "../shared/RecordMenu";
import { StatusTag } from "../shared/StatusTag";
import { TransactionPicker } from "../shared/TransactionPicker";
import { useCompactScreen } from "../shared/useCompactScreen";
import { useConfirm } from "../shared/useConfirm";
import { useFeedback } from "../shared/useFeedback";

/**
 * The picker for "Conciliar com…": a server-side searched TransactionPicker
 * over the transactions eligible for one occurrence. Same shape as the payables link
 * dialog, because it is the same problem — choosing a real transaction for a
 * planned thing.
 */
function ReconcileDialog({
  commitmentId,
  kind,
  occurrence,
  onCancel,
  onConfirm,
}: {
  commitmentId: string;
  kind: RecurringCommitmentKind;
  occurrence: RecurrenceOccurrence | null;
  onCancel: () => void;
  onConfirm: (transactionId: string) => Promise<unknown>;
}) {
  const { search, setSearch, candidates, isSearching } = useReconciliationCandidates(
    commitmentId,
    occurrence?.date ?? null,
  );
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const close = () => {
    setSelectedId(null);
    setError(null);
    onCancel();
  };

  const submit = async () => {
    if (selectedId === null) return;
    setError(null);
    setSubmitting(true);
    try {
      await onConfirm(selectedId);
      setSelectedId(null);
      onCancel();
    } catch (caught) {
      setError(errorMessage(caught, "Não foi possível conciliar a transação."));
    } finally {
      setSubmitting(false);
    }
  };

  // A form shell rather than a centered dialog: a full-screen sheet with a
  // full-size Conciliar on a phone, a right-hand drawer on a wide screen.
  // FormDrawer stops its submit from reaching the recurrence's own edit form,
  // which this sits inside on a phone.
  return (
    <>
      <FormDrawer
        title={occurrence === null ? "Conciliar ocorrência" : `Conciliar ocorrência de ${formatDay(occurrence.date)}`}
        open={occurrence !== null}
        onClose={close}
        onSubmit={submit}
        submitLabel="Conciliar"
        submitting={submitting}
        submitDisabled={selectedId === null}
        error={error}
      >
        {occurrence && (
          <Typography.Text type="secondary">Valor esperado: {formatBRL(occurrence.expected_amount)}</Typography.Text>
        )}
        <TransactionPicker
          id="recurrence-reconcile-transaction"
          label="Buscar transação para conciliar"
          allowClear
          placeholder="Buscar por descrição"
          value={selectedId}
          search={{ value: search, onChange: setSearch }}
          loading={isSearching}
          emptyText="Nenhuma transação elegível por perto"
          transactions={candidates.map((candidate) => ({
            id: candidate.id,
            description: candidate.description ?? "Sem descrição",
            amount: candidate.effective_money.value,
            direction: kind === "expense" ? "outflow" : "inflow",
            date: candidate.occurred_at,
            account: candidate.account_name,
          }))}
          onSelect={setSelectedId}
        />
      </FormDrawer>
    </>
  );
}

/**
 * The occurrences of a recurrence and what settled each one: the expanded
 * row of the table on a wide screen, and — on a phone, where the table cannot
 * fit — stacked rows inside the edit drawer.
 *
 * The three actions map to the three states an occurrence can be in.
 * "Desconciliar" always leaves the occurrence unreconciled, whether an
 * automation rule or a manual pick was behind it — deleting a manual link
 * alone would let the rule re-claim the occurrence on the next read, and the
 * button would look broken. "Voltar ao automático" is the separate, explicit
 * undo that hands it back to the rule. Conciliar is the everyday action and
 * stays visible; the other two sit behind a `···`.
 */
export function RecurrenceOccurrenceList({ commitment }: { commitment: RecurringCommitment }) {
  const compact = useCompactScreen();
  const occurrences = useRecurrenceOccurrences(commitment.id);
  const [picking, setPicking] = useState<RecurrenceOccurrence | null>(null);
  const confirm = useConfirm();
  const feedback = useFeedback();
  const [actionError, setActionError] = useState<string | null>(null);
  const [pendingDate, setPendingDate] = useState<string | null>(null);

  // The inline alert stays by the list; a toast says it too, since the `···`
  // or the confirm that started the write is gone by the time it fails.
  const runAction = async (date: string, action: () => Promise<unknown>, fallback: string, done: string) => {
    setActionError(null);
    setPendingDate(date);
    try {
      await action();
      feedback.success(done);
    } catch (error) {
      const message = errorMessage(error, fallback);
      setActionError(message);
      feedback.error(message);
    } finally {
      setPendingDate(null);
    }
  };

  const direction = commitment.kind === "income" ? "inflow" : "outflow";

  const transactionText = (occurrence: RecurrenceOccurrence): string | null => {
    if (occurrence.transaction === null) return null;
    const { occurred_at, description, effective_money } = occurrence.transaction;
    return `${formatOptionalLocalDay(occurred_at)} · ${description ?? "Sem descrição"} · ${formatBRL(effective_money.value)}`;
  };

  const actions = (occurrence: RecurrenceOccurrence) => {
    const pending = pendingDate === occurrence.date;
    const secondary: RecordMenuItem[] =
      occurrence.status === "reconciled"
        ? [
            {
              key: "detach",
              label: "Desconciliar",
              danger: true,
              onClick: () =>
                confirm({
                  title: "Desconciliar ocorrência",
                  description: "A ocorrência volta a ser projetada no Início. A transação permanece inalterada.",
                  okText: "Desconciliar",
                  onConfirm: () =>
                    runAction(
                      occurrence.date,
                      () => occurrences.detach(occurrence.date),
                      "Não foi possível desconciliar a ocorrência.",
                      "Ocorrência desconciliada",
                    ),
                }),
            },
          ]
        : occurrence.status === "detached"
          ? [
              {
                key: "restore",
                label: "Voltar ao automático",
                onClick: () =>
                  void runAction(
                    occurrence.date,
                    () => occurrences.restoreAutomatic(occurrence.date),
                    "Não foi possível voltar ao automático.",
                    "Ocorrência de volta ao automático",
                  ),
              },
            ]
          : [];
    return (
      <Space size={0}>
        {occurrence.status !== "reconciled" && (
          <Button type="link" loading={pending} onClick={() => setPicking(occurrence)}>
            Conciliar com…
          </Button>
        )}
        {secondary.length > 0 && (
          <RecordMenu
            label={`Mais ações da ocorrência de ${formatDay(occurrence.date)}`}
            loading={pending && occurrence.status === "reconciled"}
            items={secondary}
          />
        )}
      </Space>
    );
  };

  const columns: ColumnsType<RecurrenceOccurrence> = [
    {
      title: "Ocorrência",
      dataIndex: "date",
      render: (_: unknown, occurrence) => formatDay(occurrence.date),
    },
    {
      title: "Valor esperado",
      dataIndex: "expected_amount",
      render: (_: unknown, occurrence) => (
        <Money value={occurrence.expected_amount} tone="flow" direction={direction} />
      ),
    },
    {
      title: "Situação",
      dataIndex: "status",
      render: (_: unknown, occurrence) => (
        <StatusTag tone={reconciliationStatusTone[occurrence.status]}>
          {reconciliationLabel(occurrence.status, occurrence.origin)}
        </StatusTag>
      ),
    },
    {
      title: "Transação",
      dataIndex: "transaction",
      render: (_: unknown, occurrence) => transactionText(occurrence) ?? "—",
    },
    {
      title: "Ação",
      render: (_: unknown, occurrence) => actions(occurrence),
    },
  ];

  const stacked = (
    <ul className="occurrence-rows" aria-label={`Ocorrências de ${commitment.name}`}>
      {occurrences.occurrences.map((occurrence) => (
        <li key={occurrence.date} className="occurrence-row">
          <div className="occurrence-row-line">
            <span className="occurrence-row-title">{formatDay(occurrence.date)}</span>
            <Money value={occurrence.expected_amount} tone="flow" direction={direction} size="row" />
          </div>
          <div className="occurrence-row-meta">
            <StatusTag tone={reconciliationStatusTone[occurrence.status]}>
              {reconciliationLabel(occurrence.status, occurrence.origin)}
            </StatusTag>
            {transactionText(occurrence) && <span>{transactionText(occurrence)}</span>}
          </div>
          <div className="occurrence-row-actions">{actions(occurrence)}</div>
        </li>
      ))}
    </ul>
  );

  return (
    <Flex vertical gap="small">
      {actionError && (
        <Alert
          type="error"
          showIcon
          closable
          onClose={() => setActionError(null)}
          message={actionError}
        />
      )}
      {occurrences.error !== null && (
        <Alert
          type="warning"
          showIcon
          message="Não foi possível carregar as ocorrências."
          action={<Button onClick={occurrences.retry}>Tentar novamente</Button>}
        />
      )}
      {occurrences.isLoading ? (
        // Rows the shape of the ones that follow, on either layout, so the
        // expanded row or the drawer does not jump when they arrive.
        <div role="status" aria-label="Carregando ocorrências">
          <Skeleton active title={false} paragraph={{ rows: 3 }} />
        </div>
      ) : compact ? (
        occurrences.occurrences.length === 0 ? (
          <Typography.Text type="secondary">Nenhuma ocorrência neste período.</Typography.Text>
        ) : (
          stacked
        )
      ) : (

        <Table<RecurrenceOccurrence>
          aria-label={`Ocorrências de ${commitment.name}`}
          columns={columns}
          dataSource={occurrences.occurrences}
          rowKey="date"
          pagination={false}
          size="small"
          locale={{
            emptyText: "Nenhuma ocorrência neste período.",
          }}
        />
      )}
      <ReconcileDialog
        commitmentId={commitment.id}
        kind={commitment.kind}
        occurrence={picking}
        onCancel={() => setPicking(null)}
        onConfirm={async (transactionId) => {
          await occurrences.reconcile(picking!.date, transactionId);
          feedback.success("Ocorrência conciliada");
        }}
      />
    </Flex>
  );
}
