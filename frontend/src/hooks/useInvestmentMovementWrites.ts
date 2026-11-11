import { useMutation, useQueryClient } from "@tanstack/react-query";

import { createInvestmentMovement, deleteInvestmentMovement, updateInvestmentMovement } from "../api/investmentPortfolio";
import type { InvestmentMovementWrite } from "../api/contracts";
import { invalidateAfterProviderMovementChange } from "../api/queryKeys";

/** Records, corrects and removes the movements of one provider holding. */
export function useInvestmentMovementWrites() {
  const queryClient = useQueryClient();
  const refresh = () => invalidateAfterProviderMovementChange(queryClient);

  const create = useMutation({
    mutationFn: (write: InvestmentMovementWrite) => createInvestmentMovement(write),
    onSuccess: refresh,
  });
  const update = useMutation({
    mutationFn: ({ movementId, write }: { movementId: string; write: InvestmentMovementWrite }) =>
      updateInvestmentMovement(movementId, write),
    onSuccess: refresh,
  });
  const remove = useMutation({
    mutationFn: (movementId: string) => deleteInvestmentMovement(movementId),
    onSuccess: refresh,
  });

  return {
    createMovement: create.mutateAsync,
    updateMovement: update.mutateAsync,
    deleteMovement: remove.mutateAsync,
    isSaving: create.isPending || update.isPending,
    isDeleting: remove.isPending,
  };
}
