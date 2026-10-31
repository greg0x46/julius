import { isPositiveDecimal } from "../../presentation/decimal";
import { CloseOutlined } from "../ui/icons";
import { Alert, Button, DatePicker, Drawer, Input, Select, Skeleton, Switch } from "../ui";
import type { Column } from "../ui/Table";
import Table from "../ui/Table";
import type { CalendarDate } from "../../presentation/calendar";
import { todayIn } from "../../presentation/timezone";
import { useTimezone } from "../../hooks/useTimezone";
import { useEffect, useId, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";

import type { Scenario, ScenarioTransaction, ScenarioTransactionWrite } from "../../api/contracts";
import { getScenario, listScenarioPlannedTransactions } from "../../api/scenarios";
import { errorMessage } from "../../presentation/errors";
import { scenarioKindLabel } from "../../presentation/scenarioLabels";
import { MoneyInput } from "../forms/MoneyInput";
import { FormField } from "../forms/FormField";
import { DataCard, EmptyState, ResponsiveList } from "../layout";
import { Money } from "../shared/Money";
import { RecordMenu } from "../shared/RecordMenu";
import { StatusTag } from "../shared/StatusTag";
import { useCompactScreen } from "../shared/useCompactScreen";
import { useConfirm } from "../shared/useConfirm";
import { useFeedback } from "../shared/useFeedback";
import { queryKeys } from "../../api/queryKeys";

const dateFormat = "YYYY-MM-DD";

function formatProjectedAt(value: string): string {
  const [year, month, day] = value.split("-");
  return `${day}/${month}/${year}`;
}

const originLabel = (scenario: Scenario) => (scenario.is_accounting_source ? "Contábil" : "Simulação");

const deleteScenarioDescription = "As projeções deste cenário também serão excluídas.";

/**
 * The form that adds one hypothetical transaction. Its submit button is either
 * inline (the table's expanded row) or, in the phone sheet, the sheet's pinned
 * footer button, which joins the form through `formId` — the same arrangement
 * `FormDrawer` uses. A failed write keeps what was typed and says why right
 * here, next to the fields, not in an alert somewhere above the fold.
 */
function NewTransactionForm({
  onAdd,
  submitting,
  formId,
  error: writeError,
}: {
  /** Resolves true when the transaction was saved. Never rejects. */
  onAdd: (write: ScenarioTransactionWrite) => Promise<boolean>;
  submitting: boolean;
  /** Set when the submit button lives outside the form (the sheet's footer). */
  formId?: string;
  /** Why the last write failed, shown with the fields. */
  error: string | null;
}) {
  const id = useId();
  const timezone = useTimezone();
  const [description, setDescription] = useState("");
  const [kind, setKind] = useState<"income" | "expense">("expense");
  const [amount, setAmount] = useState<string | null>(null);
  const [projectedAt, setProjectedAt] = useState<CalendarDate>(() => todayIn(timezone));
  const [error, setError] = useState<string | null>(null);
  // `submitting` arrives a render late: a second Enter in the same tick would
  // otherwise create the transaction twice.
  const inFlight = useRef(false);

  const submit = async () => {
    if (inFlight.current || submitting) return;
    if (description.trim() === "") {
      setError("Informe uma descrição.");
      return;
    }
    if (amount === null || !isPositiveDecimal(amount)) {
      setError("Informe um valor maior que zero.");
      return;
    }
    setError(null);
    inFlight.current = true;
    try {
      const saved = await onAdd({
        description: description.trim(),
        amount: kind === "expense" ? `-${amount}` : amount,
        projected_at: projectedAt.format(dateFormat),
      });
      if (saved) {
        setDescription("");
        setAmount(null);
        setProjectedAt(todayIn(timezone));
      }
    } finally {
      inFlight.current = false;
    }
  };

  const shownError = error ?? writeError;

  return (
    <form
      id={formId}
      className="scenario-new-transaction"
      aria-label="Nova transação hipotética"
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
      noValidate
    >
      {shownError && (
        <Alert
          type="error"
          showIcon
          message={shownError}
          closable={error !== null}
          onClose={() => setError(null)}
        />
      )}
      <div className="scenario-new-transaction-fields">
        <FormField label="Descrição" htmlFor={`${id}-description`}>
          <Input
            id={`${id}-description`}
            placeholder="Ex.: Passagem aérea"
            value={description}
            onChange={(event) => setDescription(event.target.value)}
          />
        </FormField>
        <FormField label="Tipo" htmlFor={`${id}-kind`}>
          <Select
            id={`${id}-kind`}
            value={kind}
            options={[
              { value: "expense", label: "Despesa" },
              { value: "income", label: "Receita" },
            ]}
            onChange={setKind}
          />
        </FormField>
        <FormField label="Valor" htmlFor={`${id}-amount`}>
          <MoneyInput id={`${id}-amount`} min="0" value={amount} onChange={setAmount} />
        </FormField>
        <FormField label="Data prevista" htmlFor={`${id}-date`}>
          <DatePicker
            id={`${id}-date`}
            format="DD/MM/YYYY"
            value={projectedAt}
            allowClear={false}
            onChange={(value) => value && setProjectedAt(value)}
          />
        </FormField>
      </div>
      {formId === undefined && (
        <Button htmlType="submit" loading={submitting}>
          Adicionar transação
        </Button>
      )}
    </form>
  );
}

/**
 * The body of a scenario: its hypothetical transactions, the realized/pending
 * counts of the projected occurrences, and — for standalone scenarios only —
 * the form to add another transaction. It is the expanded row of the table on
 * a wide screen and the body of the scenario sheet on a phone.
 *
 * Detail and planned-transaction data load here, per opened scenario, rather
 * than eagerly for the whole list: the same choice RecurrenceOccurrenceList
 * makes for recurring commitments, for the same reason (most scenarios are
 * never opened in a given visit).
 */
function ScenarioTransactionsPanel({
  scenario,
  onAddTransaction,
  onDeleteTransaction,
  formId,
  onSubmittingChange,
}: {
  scenario: Scenario;
  onAddTransaction: (scenarioId: string, write: ScenarioTransactionWrite) => Promise<unknown>;
  onDeleteTransaction: (scenarioId: string, transactionId: string) => Promise<void>;
  /** The sheet's footer button submits the add form through this id. */
  formId?: string;
  /** Tells the sheet when the add is in flight, for its footer button. */
  onSubmittingChange?: (submitting: boolean) => void;
}) {
  const compact = useCompactScreen();
  const feedback = useFeedback();
  const confirm = useConfirm();
  const detailQuery = useQuery({
    queryKey: queryKeys.scenarioDetail(scenario.id),
    queryFn: ({ signal }) => getScenario(scenario.id, signal),
  });
  const plannedQuery = useQuery({
    queryKey: queryKeys.scenarioPlannedTransactions(scenario.id),
    queryFn: async ({ signal }) =>
      (await listScenarioPlannedTransactions(scenario.id, undefined, undefined, signal)) ?? [],
  });
  const [addError, setAddError] = useState<string | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);

  useEffect(() => {
    onSubmittingChange?.(adding);
  }, [adding, onSubmittingChange]);

  const add = async (write: ScenarioTransactionWrite): Promise<boolean> => {
    setAddError(null);
    setAdding(true);
    try {
      await onAddTransaction(scenario.id, write);
      feedback.success("Transação hipotética adicionada");
    } catch (error) {
      setAddError(errorMessage(error, "Não foi possível adicionar a transação."));
      return false;
    } finally {
      setAdding(false);
    }
    // The transaction is saved; a failed refresh must not read as a failed save.
    await detailQuery.refetch().catch(() => undefined);
    return true;
  };

  const remove = async (transactionId: string) => {
    setDeleteError(null);
    try {
      await onDeleteTransaction(scenario.id, transactionId);
      feedback.success("Transação hipotética excluída");
    } catch (error) {
      const message = errorMessage(error, "Não foi possível excluir a transação.");
      setDeleteError(message);
      feedback.error(message);
      return;
    }
    await detailQuery.refetch().catch(() => undefined);
  };

  const transactionMenu = (transaction: ScenarioTransaction) => (
    <RecordMenu
      label={`Ações de ${transaction.description}`}
      items={[
        {
          key: "delete",
          label: "Excluir",
          danger: true,
          onClick: () =>
            confirm({
              title: "Excluir transação hipotética",
              onConfirm: () => remove(transaction.id),
            }),
        },
      ]}
    />
  );

  const columns: Column<ScenarioTransaction>[] = [
    { title: "Descrição", dataIndex: "description" },
    {
      title: "Valor",
      dataIndex: "amount",
      align: "right",
      render: (_, transaction) => <Money value={transaction.amount} tone="flow" />,
    },
    {
      title: "Data prevista",
      dataIndex: "projected_at",
      render: (_, transaction) => formatProjectedAt(transaction.projected_at),
    },
    {
      title: <span className="visually-hidden">Ações da transação</span>,
      key: "options",
      width: 56,
      render: (_, transaction) => transactionMenu(transaction),
    },
  ];

  const transactions = detailQuery.data?.transactions ?? [];
  const realized = plannedQuery.data?.filter((event) => event.realized).length ?? 0;
  const pending = plannedQuery.data?.filter((event) => !event.realized).length ?? 0;

  return (
    <div className="scenario-panel">
      {deleteError && (
        <Alert type="error" showIcon message={deleteError} closable onClose={() => setDeleteError(null)} />
      )}
      {detailQuery.isError ? (
        <Alert
          type="error"
          showIcon
          message="Não foi possível carregar as transações deste cenário"
          action={<Button onClick={() => void detailQuery.refetch()}>Tentar novamente</Button>}
        />
      ) : compact ? (
        detailQuery.isLoading ? (
          <Skeleton active paragraph={{ rows: 2 }} />
        ) : transactions.length === 0 ? (
          <p className="scenario-panel-empty">Nenhuma transação hipotética ainda.</p>
        ) : (
          <ul className="list-rows" aria-label={`Transações hipotéticas de ${scenario.name}`}>
            {transactions.map((transaction) => (
              <li key={transaction.id} className="list-row-item scenario-tx-row">
                <span className="list-row-text">
                  <span className="list-row-title">{transaction.description}</span>
                  <span className="list-row-meta">{formatProjectedAt(transaction.projected_at)}</span>
                </span>
                <span className="list-row-trailing">
                  <Money value={transaction.amount} tone="flow" />
                </span>
                {transactionMenu(transaction)}
              </li>
            ))}
          </ul>
        )
      ) : (
        <Table<ScenarioTransaction>
          aria-label={`Transações hipotéticas de ${scenario.name}`}
          columns={columns}
          dataSource={transactions}
          loading={detailQuery.isLoading}
          rowKey="id"
          pagination={false}
          size="small"
          locale={{ emptyText: "Nenhuma transação hipotética ainda." }}
        />
      )}
      <div className="scenario-panel-counts">
        Eventos realizados: {realized}
        {" · "}
        Eventos pendentes: {pending}
      </div>
      {scenario.kind === "standalone" && (
        <NewTransactionForm onAdd={add} submitting={adding} formId={formId} error={addError} />
      )}
    </div>
  );
}

/** The switch that decides whether a scenario's transactions count in the balance projection. */
function ProjectionSwitch({
  scenario,
  onToggleScenario,
  onError,
}: {
  scenario: Scenario;
  /** Resolves with the error message, or null when the change was saved. */
  onToggleScenario: (scenario: Scenario, isActive: boolean) => Promise<string | null>;
  /** Where the failure shows: the page for the table, the sheet itself on a phone. */
  onError: (message: string) => void;
}) {
  const [busy, setBusy] = useState(false);
  const change = async (checked: boolean) => {
    setBusy(true);
    try {
      const message = await onToggleScenario(scenario, checked);
      if (message !== null) onError(message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Switch
      aria-label={`Incluir na projeção: ${scenario.name}`}
      checked={scenario.is_active !== false}
      loading={busy}
      onChange={(checked) => void change(checked)}
    />
  );
}

/**
 * A scenario opened from its row on a phone: the full-screen sheet that
 * stands in for the table's expanded row. It has the same shell as every
 * other form sheet — close at the right, the record's own actions (Excluir)
 * behind a `···` beside it, one pinned full-width button in the footer — so
 * nothing about it is special.
 */
function ScenarioSheet({
  scenario,
  onClose,
  onDeleteScenario,
  onAddTransaction,
  onDeleteTransaction,
  onToggleScenario,
}: {
  scenario: Scenario | null;
  onClose: () => void;
  onDeleteScenario: (scenario: Scenario) => Promise<string | null>;
  onAddTransaction: (scenarioId: string, write: ScenarioTransactionWrite) => Promise<unknown>;
  onDeleteTransaction: (scenarioId: string, transactionId: string) => Promise<void>;
  onToggleScenario?: (scenario: Scenario, isActive: boolean) => Promise<string | null>;
}) {
  const confirm = useConfirm();
  const formId = useId();
  const [error, setError] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);
  const scenarioId = scenario?.id;

  // Each scenario opens clean: no error left over from the previous one.
  useEffect(() => {
    setError(null);
  }, [scenarioId]);

  const standalone = scenario?.kind === "standalone";

  return (
    <Drawer
      open={scenario !== null}
      onClose={onClose}
      title={scenario?.name}
      width="100%"
      rootClassName="form-drawer"
      destroyOnHidden
      closable={false}
      extra={
        scenario && (
          <span className="scenario-sheet-actions">
            <RecordMenu
              label={`Ações de ${scenario.name}`}
              items={[
                {
                  key: "delete",
                  label: "Excluir cenário",
                  danger: true,
                  onClick: () =>
                    confirm({
                      title: "Excluir cenário",
                      description: deleteScenarioDescription,
                      onConfirm: async () => {
                        setError(null);
                        const message = await onDeleteScenario(scenario);
                        if (message === null) onClose();
                        else setError(message);
                      },
                    }),
                },
              ]}
            />
            <Button
              type="text"
              className="form-drawer-close"
              icon={<CloseOutlined />}
              aria-label="Fechar"
              onClick={onClose}
            />
          </span>
        )
      }
      footer={
        <div className="form-drawer-footer">
          {standalone ? (
            <Button type="primary" htmlType="submit" form={formId} size="large" loading={adding}>
              Adicionar transação
            </Button>
          ) : (
            <Button size="large" onClick={onClose}>
              Fechar
            </Button>
          )}
        </div>
      }
    >
      {scenario && (
        <div className="scenario-sheet">
          {error && <Alert type="error" showIcon message={error} closable onClose={() => setError(null)} />}
          <p className="scenario-sheet-meta">
            {scenarioKindLabel[scenario.kind]} · {originLabel(scenario)}
          </p>
          {onToggleScenario && (
            <div className="scenario-sheet-switch">
              <span>
                <span className="form-field-label">Incluir na projeção</span>
                <span className="form-field-hint">
                  Quando ligado, as transações deste cenário entram na projeção de saldo.
                </span>
              </span>
              <ProjectionSwitch scenario={scenario} onToggleScenario={onToggleScenario} onError={setError} />
            </div>
          )}
          <ScenarioTransactionsPanel
            scenario={scenario}
            onAddTransaction={onAddTransaction}
            onDeleteTransaction={onDeleteTransaction}
            formId={formId}
            onSubmittingChange={setAdding}
          />
        </div>
      )}
    </Drawer>
  );
}

/**
 * Scenarios as a table with an expander per row from `md` up and, on a
 * phone, as stacked rows (name, type · origin, "Fora da projeção" only when
 * excluded) that open the scenario's sheet. Tapping a table row expands it
 * too; Excluir is behind the row's `···`. The empty state tells "nothing
 * yet" apart from "nothing for this search or type".
 */
export function StandaloneScenarioList({
  scenarios,
  isLoading,
  isFiltered = false,
  onDeleteScenario,
  onAddTransaction,
  onDeleteTransaction,
  onToggleScenario,
  onActionError,
}: {
  scenarios: Scenario[];
  isLoading: boolean;
  /** A search or a type filter is narrowing the list. */
  isFiltered?: boolean;
  /** Resolves with the error message, or null when the scenario was deleted. */
  onDeleteScenario: (scenario: Scenario) => Promise<string | null>;
  onAddTransaction: (scenarioId: string, write: ScenarioTransactionWrite) => Promise<unknown>;
  onDeleteTransaction: (scenarioId: string, transactionId: string) => Promise<void>;
  /** Resolves with the error message, or null when the change was saved. */
  onToggleScenario?: (scenario: Scenario, isActive: boolean) => Promise<string | null>;
  /** A failure raised from the table (not from inside the sheet): shown on the page. */
  onActionError: (message: string) => void;
}) {
  const confirm = useConfirm();
  const [openId, setOpenId] = useState<string | null>(null);

  const deleteFromTable = async (scenario: Scenario) => {
    const message = await onDeleteScenario(scenario);
    if (message !== null) onActionError(message);
  };

  const columns: Column<Scenario>[] = [
    { title: "Nome", dataIndex: "name" },
    {
      title: "Tipo",
      dataIndex: "kind",
      render: (_, scenario) => scenarioKindLabel[scenario.kind],
    },
    {
      title: "Origem",
      dataIndex: "is_accounting_source",
      render: (_, scenario) => originLabel(scenario),
    },
    {
      title: "Incluir na projeção",
      dataIndex: "is_active",
      render: (_, scenario) =>
        onToggleScenario ? (
          <span onClick={(event) => event.stopPropagation()}>
            <ProjectionSwitch scenario={scenario} onToggleScenario={onToggleScenario} onError={onActionError} />
          </span>
        ) : null,
    },
    {
      title: <span className="visually-hidden">Ações do cenário</span>,
      key: "options",
      width: 56,
      render: (_, scenario) => (
        <span onClick={(event) => event.stopPropagation()}>
          <RecordMenu
            label={`Ações de ${scenario.name}`}
            items={[
              {
                key: "delete",
                label: "Excluir",
                danger: true,
                onClick: () =>
                  confirm({
                    title: "Excluir cenário",
                    description: deleteScenarioDescription,
                    onConfirm: () => deleteFromTable(scenario),
                  }),
              },
            ]}
          />
        </span>
      ),
    },
  ];

  return (
    <DataCard flush className="settings-list-card">
      <ResponsiveList<Scenario>
        label="Cenários"
        items={scenarios}
        getKey={(scenario) => scenario.id}
        isLoading={isLoading}
        loading={<Skeleton active paragraph={{ rows: 4 }} />}
        empty={
          isFiltered ? (
            <EmptyState title="Nenhum cenário encontrado" hint="Ajuste a busca ou o tipo para ver outros cenários." />
          ) : (
            <EmptyState
              title="Nenhum cenário ainda"
              hint="Crie um cenário, como uma viagem ou uma troca de emprego, com transações hipotéticas."
            />
          )
        }
        row={(scenario) => ({
          title: scenario.name,
          meta: `${scenarioKindLabel[scenario.kind]} · ${originLabel(scenario)}`,
          status: scenario.is_active === false ? <StatusTag tone="neutral">Fora da projeção</StatusTag> : undefined,
          onClick: () => setOpenId(scenario.id),
          ariaLabel: `Abrir cenário ${scenario.name}`,
        })}
        wide={
          <Table<Scenario>
            aria-label="Cenários"
            columns={columns}
            dataSource={scenarios}
            rowKey="id"
                pagination={false}
            expandable={{
              expandRowByClick: true,
              // The expand column has no header text; say what it does.
              columnTitle: <span className="visually-hidden">Transações hipotéticas</span>,
              expandedRowRender: (scenario) => (
                <ScenarioTransactionsPanel
                  scenario={scenario}
                  onAddTransaction={onAddTransaction}
                  onDeleteTransaction={onDeleteTransaction}
                />
              ),
            }}
          />
        }
      />
      <ScenarioSheet
        scenario={scenarios.find((scenario) => scenario.id === openId) ?? null}
        onClose={() => setOpenId(null)}
        onDeleteScenario={onDeleteScenario}
        onAddTransaction={onAddTransaction}
        onDeleteTransaction={onDeleteTransaction}
        onToggleScenario={onToggleScenario}
      />
    </DataCard>
  );
}
