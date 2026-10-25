import { Alert, Button, Popconfirm, Skeleton, Typography } from "../ui";
import { useState } from "react";

import type { ReconciliationOption, TransactionItem } from "../../api/contracts";
import { useTransactionReconciliation } from "../../hooks/useTransactionReconciliation";
import { errorMessage } from "../../presentation/errors";
import { formatDay } from "../../presentation/dates";
import { formatBRL } from "../../presentation/money";
import { reconciliationOriginLabel } from "../../presentation/reconciliationLabels";
import { recurringCommitmentKindLabel } from "../../presentation/recurringCommitmentLabels";
import { TransactionAmount } from "./TransactionAmount";
import { TransactionPicker } from "../shared/TransactionPicker";
import { PanelDisclosure, PanelFooter, PanelSection } from "../shared/PanelStack";

// The option's value has to carry both halves of an occurrence's address —
// the commitment and the calendar day — because occurrences have no id.
function optionValue(option: ReconciliationOption): string {
  return `${option.commitment_id}|${option.occurrence_date}`;
}


/**
 * Reconciliation seen from a transaction: what recurring occurrence it
 * settles, and which nearby occurrences it could settle instead.
 *
 * Only occurrences flowing the same direction as the transaction are offered
 * — an outflow can never settle an income commitment — so an empty list
 * usually means there is simply no matching commitment near this date.
 *
 * The writes are the same occurrence-scoped endpoints the Recorrências page
 * calls; this screen must not grow a second set of rules.
 */
export function TransactionReconcileScreen({
  item,
  onDone,
}: {
  item: TransactionItem;
  /** Called after a successful write so the stack can pop back. */
  onDone: () => void;
}) {
  const reconciliation = useTransactionReconciliation(item.id);
  const [selected, setSelected] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const run = async (action: () => Promise<unknown>, fallback: string) => {
    setError(null);
    try {
      await action();
      onDone();
    } catch (caught) {
      setError(errorMessage(caught, fallback));
    }
  };

  const reconcile = () => {
    if (selected === null) {
      setError("Selecione uma ocorrência para conciliar.");
      return;
    }
    const [commitmentId, occurrenceDate] = selected.split("|") as [string, string];
    void run(
      () => reconciliation.reconcile({ commitmentId, occurrenceDate }),
      "Não foi possível conciliar a transação.",
    );
  };

  const current = reconciliation.current;

  return (
    <>
      <header className="transaction-screen-summary">
        <TransactionAmount item={item} />
        <span>{item.description ?? "Descrição não informada"}</span>
      </header>

      {error && (
        <Alert type="error" showIcon closable onClose={() => setError(null)} message={error} />
      )}

      {reconciliation.isLoading ? (
        <div role="status" aria-label="Carregando conciliação">
          <Skeleton active paragraph={{ rows: 2 }} title={{ width: "40%" }} />
        </div>
      ) : reconciliation.isError ? (
        // A failed lookup is not "no recurrence nearby": say so, and offer the retry.
        <Alert
          type="error"
          showIcon
          message="Não foi possível consultar as recorrências"
          description={<Button onClick={reconciliation.refetch}>Tentar novamente</Button>}
        />
      ) : current !== null ? (
        <PanelSection title="Conciliada com">
          <div className="transaction-reconciled">
            <strong>{current.commitment_name}</strong>
            <span>
              {formatDay(current.occurrence_date)} · {formatBRL(current.expected_amount)}
            </span>
            <Typography.Text type="secondary">
              Conciliação {reconciliationOriginLabel[current.origin]}
            </Typography.Text>
          </div>
          <PanelFooter>
            <Popconfirm
              title="Desconciliar"
              description="A ocorrência volta a ser projetada na Home. A transação permanece inalterada."
              onConfirm={() =>
                run(
                  () =>
                    reconciliation.detach({
                      commitmentId: current.commitment_id,
                      occurrenceDate: current.occurrence_date,
                    }),
                  "Não foi possível desconciliar.",
                )
              }
              okText="Desconciliar"
              cancelText="Cancelar"
            >
              <Button danger block loading={reconciliation.isWriting}>
                Desconciliar
              </Button>
            </Popconfirm>
          </PanelFooter>
        </PanelSection>
      ) : (
        <>
          <PanelSection title="Ocorrência">
            <TransactionPicker
              id="transaction-reconcile-occurrence"
              label="Conciliar com uma recorrência"
              value={selected}
              placeholder="Selecione uma ocorrência"
              disabled={reconciliation.options.length === 0}
              emptyText="Nenhuma ocorrência compatível por perto"
              transactions={reconciliation.options.map((option) => ({
                id: optionValue(option),
                description: option.commitment_name,
                amount: option.expected_amount,
                direction: option.kind === "expense" ? "outflow" : "inflow",
                date: option.occurrence_date,
                category: `${recurringCommitmentKindLabel[option.kind]} recorrente`,
              }))}
              onSelect={setSelected}
            />
            {reconciliation.options.length === 0 && (
              <Typography.Text type="secondary" className="transaction-field-hint">
                Nenhuma ocorrência de recorrência compatível perto desta data.
              </Typography.Text>
            )}
            <PanelDisclosure summary="A ocorrência escolhida deixa de ser projetada.">
              Só aparecem ocorrências na mesma direção desta transação e com data próxima. Ao conciliar, o
              valor previsto dessa ocorrência sai da projeção e passa a ser representado por esta transação.
            </PanelDisclosure>
          </PanelSection>
          <PanelFooter>
            <Button
              type="primary"
              block
              loading={reconciliation.isWriting}
              disabled={reconciliation.options.length === 0}
              onClick={reconcile}
            >
              Conciliar
            </Button>
          </PanelFooter>
        </>
      )}
    </>
  );
}
