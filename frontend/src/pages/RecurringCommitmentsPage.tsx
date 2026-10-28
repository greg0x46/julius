import { Alert, Button } from "../components/ui";
import { useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";

import type { RecurringCommitment, RecurringCommitmentWrite } from "../api/contracts";
import { DataCard, EmptyState, Page, PageAction } from "../components/layout";
import { RecurrenceEditExtras } from "../components/recurringCommitments/RecurrenceEditExtras";
import type { RecurringCommitmentDraft } from "../components/recurringCommitments/RecurringCommitmentForm";
import { RecurringCommitmentForm } from "../components/recurringCommitments/RecurringCommitmentForm";
import { RecurringCommitmentList } from "../components/recurringCommitments/RecurringCommitmentList";
import { useCompactScreen } from "../components/shared/useCompactScreen";
import { useFeedback } from "../components/shared/useFeedback";
import { useCategories } from "../hooks/useCategories";
import { useRecurringCommitments } from "../hooks/useRecurringCommitments";
import { errorMessage } from "../presentation/errors";

export function RecurringCommitmentsPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const compact = useCompactScreen();
  const feedback = useFeedback();
  const commitments = useRecurringCommitments();
  const categories = useCategories();
  const [formOpen, setFormOpen] = useState(
    Boolean((location.state as { prefill?: Partial<RecurringCommitmentDraft> } | null)?.prefill),
  );
  const [editingCommitment, setEditingCommitment] = useState<RecurringCommitment | null>(null);
  const [initialDraft, setInitialDraft] = useState<Partial<RecurringCommitmentDraft> | null>(
    (location.state as { prefill?: Partial<RecurringCommitmentDraft> } | null)?.prefill ?? null,
  );
  const [togglingCommitmentId, setTogglingCommitmentId] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const openCreate = () => {
    setEditingCommitment(null);
    setSaveError(null);
    setFormOpen(true);
  };

  const openEdit = (commitment: RecurringCommitment) => {
    setEditingCommitment(commitment);
    setSaveError(null);
    setFormOpen(true);
  };

  const closeForm = () => {
    setFormOpen(false);
    setInitialDraft(null);
    if (location.state) navigate(".", { replace: true, state: null });
  };

  const submit = async (write: RecurringCommitmentWrite) => {
    setSaveError(null);
    try {
      if (editingCommitment) {
        await commitments.updateCommitment({ commitmentId: editingCommitment.id, write });
      } else {
        await commitments.createCommitment(write);
      }
      setFormOpen(false);
      setInitialDraft(null);
      if (location.state) navigate(".", { replace: true, state: null });
      feedback.success(editingCommitment ? "Salvo" : "Recorrência criada");
    } catch (error) {
      setSaveError(errorMessage(error, "Não foi possível salvar a recorrência."));
    }
  };

  // The page-top Alert is out of view once the list has scrolled, and the
  // row's menu/confirm that started the write is already closed: a toast says
  // it where the user is, the Alert stays for whoever scrolls up.
  const reportActionError = (message: string) => {
    setActionError(message);
    feedback.error(message);
  };

  const toggle = async (commitment: RecurringCommitment, isActive: boolean) => {
    setActionError(null);
    setTogglingCommitmentId(commitment.id);
    try {
      await commitments.toggleCommitment({ commitmentId: commitment.id, isActive });
    } catch (error) {
      reportActionError(errorMessage(error, "Não foi possível salvar a recorrência."));
    } finally {
      setTogglingCommitmentId(null);
    }
  };

  const remove = async (commitment: RecurringCommitment) => {
    setActionError(null);
    try {
      await commitments.deleteCommitment(commitment.id);
      setFormOpen(false);
      feedback.success("Excluído");
    } catch (error) {
      const message = errorMessage(error, "Não foi possível excluir a recorrência.");
      // Deleting from the phone's edit drawer: the error must show there, not behind it.
      if (formOpen) setSaveError(message);
      else reportActionError(message);
    }
  };

  // No button here: the title row's "Nova recorrência" is the one way to start,
  // and a second one right under it would only repeat it.
  const empty = (
    <EmptyState
      title="Nenhuma recorrência ainda"
      hint="Cadastre salário, aluguel e assinaturas para acompanhar o que se repete a cada mês."
    />
  );
  // A failed load with nothing to show is not "nothing yet": it only says it failed.
  const loadFailed = commitments.error !== null && commitments.commitments.length === 0;

  return (
    <Page
      title="Recorrências"
      description="Acompanhe salário, aluguel e assinaturas que se repetem"
      width="narrow"
      compactMobileHeader
      actions={<PageAction label="Nova recorrência" shortLabel="Nova" onClick={openCreate} />}
    >
      {actionError && (
        <Alert
          type="error"
          showIcon
          closable
          onClose={() => setActionError(null)}
          message={actionError}
          style={{ marginBottom: 16 }}
        />
      )}
      {commitments.error && (
        <Alert
          type="error"
          showIcon
          message="Não foi possível carregar as recorrências"
          action={<Button onClick={() => commitments.refetch()}>Tentar novamente</Button>}
          style={{ marginBottom: 16 }}
        />
      )}
      {!loadFailed && (
        <DataCard flush className="recurrences-card">
          <RecurringCommitmentList
            commitments={commitments.commitments}
            categories={categories.categories}
            isLoading={commitments.isLoading}
            togglingCommitmentId={togglingCommitmentId}
            empty={empty}
            onEdit={openEdit}
            onToggle={toggle}
            onDelete={remove}
          />
        </DataCard>
      )}
      <RecurringCommitmentForm
        open={formOpen}
        commitment={editingCommitment}
        initialDraft={editingCommitment ? null : initialDraft}
        categories={categories.categories}
        submitting={commitments.isSaving}
        submitError={saveError}
        onSubmit={submit}
        onCancel={closeForm}
        extra={
          compact && editingCommitment ? (
            <RecurrenceEditExtras commitment={editingCommitment} onDelete={remove} />
          ) : undefined
        }
      />
    </Page>
  );
}
