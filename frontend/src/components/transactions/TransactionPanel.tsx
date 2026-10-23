import { useQueryClient } from "@tanstack/react-query";
import { Alert, Button } from "../ui";
import { useEffect, useState } from "react";

import type {
  Category,
  ManualTransactionWrite,
  RecurringCommitmentWrite,
  TransactionInclusionState,
  TransactionItem,
} from "../../api/contracts";
import { useRecurringCommitments } from "../../hooks/useRecurringCommitments";
import { recurringCommitmentPrefill } from "../../presentation/transactionDetail";
import { InvestmentLinkNewOperationScreen, InvestmentLinkScreen } from "../investments/InvestmentLinkScreen";
import { RecurringCommitmentFields } from "../recurringCommitments/RecurringCommitmentForm";
import { PanelFooter, PanelSection, PanelStack } from "../shared/PanelStack";
import { RecordMenu } from "../shared/RecordMenu";
import { useConfirm } from "../shared/useConfirm";
import { useScreenStack } from "../shared/useScreenStack";
import { ManualTransactionFields } from "./ManualTransactionForm";
import {
  manualDraftFrom,
  manualDraftIssue,
  nextIssue,
  type ManualIssue,
  manualDraftToWrite,
  type ManualDraft,
} from "./manualTransactionDraft";
import { TransactionDetailsScreen, TransactionTechnicalScreen } from "./TransactionInfoScreens";
import { TransactionTransferScreen } from "./TransactionTransferScreen";
import { TransactionOverviewScreen } from "./TransactionOverviewScreen";
import { TransactionReconcileScreen } from "./TransactionReconcileScreen";
import type { WriteIssue } from "./useTransactionPanelWrites";
import { invalidateTransactionReconciliation } from "../../api/queryKeys";

/**
 * Every place the panel can show. Each is a whole screen that replaces the
 * previous one inside the same container — never a layer on top of it.
 */
export type TransactionScreen =
  | { kind: "overview" }
  | { kind: "details" }
  | { kind: "technical" }
  | { kind: "recurrence" }
  | { kind: "reconcile" }
  | { kind: "investment" }
  | { kind: "transfer" }
  | { kind: "investment-new"; amount: string | null }
  | { kind: "edit-manual" };

const screenTitle: Record<TransactionScreen["kind"], string> = {
  overview: "Transação",
  details: "Detalhes da transação",
  technical: "Informações técnicas",
  recurrence: "Criar recorrência",
  reconcile: "Conciliar com recorrência",
  investment: "Vincular a investimento",
  transfer: "Transferência entre contas",
  "investment-new": "Nova movimentação",
  "edit-manual": "Editar transação",
};

const root: TransactionScreen = { kind: "overview" };

export type TransactionPanelProps = {
  item: TransactionItem | null;
  categories?: Category[];
  onClose: () => void;
  onInclusion?: (id: string, state: TransactionInclusionState) => void;
  inclusionPending?: boolean;
  /** A failed "considerar nos totais" write on this line, shown next to the switch. */
  inclusionError?: WriteIssue | null;
  onCategory?: (id: string, categoryId: string) => void;
  categoryPending?: boolean;
  /** A failed category write on this line, shown under the category field. */
  categoryError?: WriteIssue | null;
  /** Rejects with an Error whose message is ready to show. */
  onSaveManual?: (id: string, write: ManualTransactionWrite) => Promise<void>;
  saveManualPending?: boolean;
  onDeleteManual?: (id: string) => void;
  deleteManualPending?: boolean;
  deleteManualError?: string | null;
};

/**
 * The transaction's own workspace: a full-screen page on a phone, a side
 * panel on a desktop, and the same stack of screens inside either. The
 * stack is keyed by the transaction, so picking another line in the list
 * always lands on its overview.
 */
export function TransactionPanel(props: TransactionPanelProps) {
  // The last selected line stays rendered while `item` is null so the
  // container can animate closed instead of vanishing.
  const [shown, setShown] = useState<TransactionItem | null>(props.item);
  if (props.item !== null && props.item !== shown) setShown(props.item);
  if (shown === null) return null;
  return <TransactionStack key={shown.id} {...props} item={shown} open={props.item !== null} />;
}

function TransactionStack({
  item,
  open,
  categories = [],
  onClose,
  onInclusion,
  inclusionPending = false,
  inclusionError = null,
  onCategory,
  categoryPending = false,
  categoryError = null,
  onSaveManual,
  saveManualPending = false,
  onDeleteManual,
  deleteManualPending = false,
  deleteManualError = null,
}: TransactionPanelProps & { item: TransactionItem; open: boolean }) {
  const stack = useScreenStack<TransactionScreen>(root);
  const confirm = useConfirm();
  const current = stack.current;
  // The stack outlives the container's content (which the Drawer destroys
  // on close), so reopening the same line must start from the overview.
  const { reset } = stack;
  useEffect(() => {
    if (open) reset();
  }, [open, reset]);
  const isManual = item.origin === "manual";

  const menu =
    isManual && current.kind === "overview" ? (
      <RecordMenu
        label="Mais ações"
        items={[
          {
            key: "edit",
            label: "Editar transação",
            disabled: !onSaveManual,
            onClick: () => stack.push({ kind: "edit-manual" }),
          },
          {
            key: "delete",
            label: "Excluir transação",
            danger: true,
            disabled: !onDeleteManual || deleteManualPending,
            onClick: () =>
              confirm({
                title: "Excluir transação",
                description: "Esta ação não pode ser desfeita.",
                onConfirm: () => onDeleteManual?.(item.id),
              }),
          },
        ]}
      />
    ) : null;

  return (
    <PanelStack
      open={open}
      onClose={onClose}
      title={screenTitle[current.kind]}
      onBack={stack.depth > 1 ? stack.pop : undefined}
      extra={menu}
    >
      {current.kind === "overview" && (
        <TransactionOverviewScreen
          item={item}
          categories={categories}
          onCategory={onCategory}
          categoryPending={categoryPending}
          onInclusion={onInclusion}
          inclusionPending={inclusionPending}
          inclusionError={inclusionError}
          categoryError={categoryError}
          deleteError={deleteManualError}
          navigate={stack.push}
        />
      )}
      {current.kind === "details" && <TransactionDetailsScreen item={item} />}
      {current.kind === "technical" && <TransactionTechnicalScreen item={item} />}
      {current.kind === "recurrence" && (
        <CreateRecurrenceScreen item={item} categories={categories} onDone={stack.reset} />
      )}
      {current.kind === "reconcile" && <TransactionReconcileScreen item={item} onDone={stack.reset} />}
      {current.kind === "transfer" && <TransactionTransferScreen item={item} />}
      {current.kind === "investment" && (
        <InvestmentLinkScreen
          transaction={item}
          onNewOperation={(amount) => stack.push({ kind: "investment-new", amount })}
          onDone={stack.reset}
        />
      )}
      {current.kind === "investment-new" && (
        <InvestmentLinkNewOperationScreen transaction={item} amount={current.amount} onDone={stack.reset} />
      )}
      {current.kind === "edit-manual" && onSaveManual && (
        <EditManualScreen
          item={item}
          categories={categories}
          submitting={saveManualPending}
          onSave={onSaveManual}
          onDone={stack.reset}
        />
      )}
    </PanelStack>
  );
}

const recurrenceFormId = "transaction-recurrence-form";

/** "Criar recorrência" from a transaction: the commitment form seeded with the line's figures. */
function CreateRecurrenceScreen({
  item,
  categories,
  onDone,
}: {
  item: TransactionItem;
  categories: Category[];
  onDone: () => void;
}) {
  const commitments = useRecurringCommitments();
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | null>(null);

  const submit = async (write: RecurringCommitmentWrite) => {
    setError(null);
    try {
      await commitments.createCommitment(write);
      // The new commitment may have an occurrence near this line, so the
      // "Conciliar" row must see it.
      await invalidateTransactionReconciliation(queryClient, item.id);
      onDone();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Não foi possível salvar a recorrência.");
    }
  };

  return (
    <>
      <PanelSection>
        <RecurringCommitmentFields
          formId={recurrenceFormId}
          commitment={null}
          initialDraft={recurringCommitmentPrefill(item)}
          categories={categories}
          submitError={error}
          submitting={commitments.isSaving}
          onSubmit={(write) => void submit(write)}
        />
      </PanelSection>
      <PanelFooter>
        <Button type="primary" block htmlType="submit" form={recurrenceFormId} loading={commitments.isSaving}>
          Criar recorrência
        </Button>
      </PanelFooter>
    </>
  );
}

const manualFormId = "transaction-edit-manual-form";

/**
 * Editing a manual transaction in place of the overview, not in a second
 * drawer. The same fields as the new-transaction drawer, in the panel's own
 * form with the shell's sticky Salvar.
 */
function EditManualScreen({
  item,
  categories,
  submitting,
  onSave,
  onDone,
}: {
  item: TransactionItem;
  categories: Category[];
  submitting: boolean;
  onSave: (id: string, write: ManualTransactionWrite) => Promise<void>;
  onDone: () => void;
}) {
  const [draft, setDraft] = useState<ManualDraft>(() => manualDraftFrom(item, item.account.id));
  const [error, setError] = useState<string | null>(null);
  const [issue, setIssue] = useState<ManualIssue | null>(null);
  const submit = async () => {
    const problem = manualDraftIssue(draft);
    setIssue((previous) => nextIssue(previous, problem));
    setError(null);
    if (problem !== null) return;
    try {
      await onSave(item.id, manualDraftToWrite(draft));
      onDone();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Não foi possível salvar a transação.");
    }
  };
  return (
    <>
      <PanelSection>
        <form
          id={manualFormId}
          className="form-drawer-form"
          noValidate
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          {error && <Alert type="error" showIcon message={error} />}
          <ManualTransactionFields
            draft={draft}
            onChange={setDraft}
            categories={categories}
            isEditing
            issue={issue}
          />
        </form>
      </PanelSection>
      <PanelFooter>
        <Button type="primary" block htmlType="submit" form={manualFormId} loading={submitting}>
          Salvar
        </Button>
      </PanelFooter>
    </>
  );
}
