import { useState } from "react";

import type { InvestmentMovementWrite, InvestmentTransaction } from "../../api/contracts";
import { useInvestmentMovementWrites } from "../../hooks/useInvestmentMovementWrites";
import { useTranslation } from "../../i18n/context";
import { errorMessage } from "../../presentation/errors";
import { useConfirm } from "../shared/useConfirm";
import { useFeedback } from "../shared/useFeedback";
import { InvestmentMovementForm } from "./InvestmentMovementForm";
import { InvestmentTimeline } from "./InvestmentTimeline";

/** A provider holding's movements and the editor for the ones the owner records. */
export function InvestmentMovementsSection({
  investmentId,
  transactions,
  isLoading,
  error,
  onRetry,
}: {
  investmentId: string;
  transactions: InvestmentTransaction[];
  isLoading: boolean;
  error: unknown;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  const writes = useInvestmentMovementWrites();
  const confirm = useConfirm();
  const feedback = useFeedback();
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<InvestmentTransaction | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);

  const openEditor = (movement: InvestmentTransaction | null) => {
    setSaveError(null);
    setEditing(movement);
    setOpen(true);
  };
  const submit = async (write: InvestmentMovementWrite) => {
    setSaveError(null);
    try {
      if (editing === null) await writes.createMovement(write);
      else await writes.updateMovement({ movementId: editing.id, write });
      setOpen(false);
      feedback.success(t("saved"));
    } catch (failure) {
      setSaveError(errorMessage(failure, t("manualMovementSaveError")));
    }
  };
  const remove = (movement: InvestmentTransaction) =>
    confirm({
      title: t("manualMovementDeleteTitle"),
      description: t("manualMovementDeleteHint"),
      onConfirm: async () => {
        try {
          await writes.deleteMovement(movement.id);
          setOpen(false);
          feedback.success(t("deleted"));
        } catch (failure) {
          setSaveError(errorMessage(failure, t("manualMovementDeleteError")));
        }
      },
    });

  return (
    <>
      <InvestmentTimeline
        transactions={transactions}
        isLoading={isLoading}
        error={error}
        onRetry={onRetry}
        onRecord={() => openEditor(null)}
        onEdit={openEditor}
      />
      <InvestmentMovementForm
        open={open}
        investmentId={investmentId}
        movement={editing}
        submitting={writes.isSaving || writes.isDeleting}
        submitError={saveError}
        onSubmit={(write) => void submit(write)}
        onDelete={remove}
        onCancel={() => setOpen(false)}
      />
    </>
  );
}
