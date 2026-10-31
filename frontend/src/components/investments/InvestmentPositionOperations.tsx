import { useTranslation } from "../../i18n/context";
import { Alert } from "../ui";
import { useState } from "react";

import type { InvestmentOperation, InvestmentOperationWrite } from "../../api/contracts";
import { useInvestmentWorkspace } from "../../hooks/useInvestmentWorkspace";
import { errorMessage } from "../../presentation/errors";
import { Section } from "../layout";
import { useFeedback } from "../shared/useFeedback";
import { InvestmentOperationForm } from "./InvestmentOperationForm";
import { InvestmentOperationsTable } from "./InvestmentOperationsTable";

/**
 * The movimentações of the position backed by this synced investment, where
 * the position is shown in detail: correcting or deleting one happens here as
 * well as from the account's own list.
 */
export function InvestmentPositionOperations({ investmentId }: { investmentId: string }) {
  const { t } = useTranslation();
  const workspace = useInvestmentWorkspace();
  const feedback = useFeedback();
  const [editing, setEditing] = useState<InvestmentOperation | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const position = workspace.positions.find((candidate) => candidate.linked_investment_id === investmentId);
  if (workspace.isLoading || !position) return null;

  const operations = workspace.operations.filter((operation) => operation.position_id === position.id);
  if (operations.length === 0) return null;

  const busy = workspace.isSaving || workspace.isDeleting;

  const submit = async (write: InvestmentOperationWrite) => {
    if (!editing) return;
    setSaveError(null);
    try {
      await workspace.updateOperation({ operationId: editing.id, write });
      setEditing(null);
      feedback.success(t("saved"));
    } catch (error) {
      setSaveError(errorMessage(error, t("operationSaveError")));
    }
  };
  const remove = async (operation: InvestmentOperation) => {
    setActionError(null);
    try {
      await workspace.deleteOperation(operation.id);
      feedback.success(t("deleted"));
    } catch (error) {
      const message = errorMessage(error, t("operationDeleteError"));
      setActionError(message);
      feedback.error(message);
    }
  };

  return (
    <Section title={t("recordedMovements", { count: operations.length })}>
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
      <InvestmentOperationsTable
        operations={operations}
        positions={workspace.positions}
        onEdit={(operation) => {
          setSaveError(null);
          setEditing(operation);
        }}
        onDelete={(operation) => void remove(operation)}
        busy={busy}
      />
      <InvestmentOperationForm
        open={editing !== null}
        operation={editing}
        initial={editing === null ? null : { account_id: editing.account_id }}
        accounts={workspace.accounts}
        positions={workspace.positions}
        submitting={workspace.isSaving}
        submitError={saveError}
        onSubmit={(write) => void submit(write)}
        onSubmitCompound={() => undefined}
        onCancel={() => setEditing(null)}
      />
    </Section>
  );
}
