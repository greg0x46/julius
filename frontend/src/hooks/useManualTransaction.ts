import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRef } from "react";

import { createManualTransaction, deleteManualTransaction, updateManualTransaction } from "../api/transactions";
import type { ManualTransactionWrite } from "../api/contracts";
import { invalidateAfterTransactionChange } from "../api/queryKeys";

/**
 * Wraps the three lançamento-manual write endpoints, invalidating every
 * ["transactions", ...] query on success so the ledger, totals, and the
 * detail drawer all pick up the change — the same query key
 * useTransactionInclusion/useTransactionCategory already invalidate.
 */
export function useManualTransaction() {
  const queryClient = useQueryClient();
  const invalidateAll = () => invalidateAfterTransactionChange(queryClient);
  const pendingCreate = useRef<{ payload: string; key: string } | null>(null);

  const createMutation = useMutation({
    mutationFn: (write: ManualTransactionWrite) => {
      const payload = JSON.stringify(write);
      if (pendingCreate.current?.payload !== payload) {
        pendingCreate.current = { payload, key: crypto.randomUUID() };
      }
      return createManualTransaction(write, pendingCreate.current.key);
    },
    onSuccess: () => {
      pendingCreate.current = null;
      invalidateAll();
    },
  });

  const updateMutation = useMutation({
    mutationFn: ({ transactionId, write }: { transactionId: string; write: ManualTransactionWrite }) =>
      updateManualTransaction(transactionId, write),
    onSuccess: invalidateAll,
  });

  const deleteMutation = useMutation({
    mutationFn: (transactionId: string) => deleteManualTransaction(transactionId),
    onSuccess: invalidateAll,
  });

  return {
    create: createMutation.mutateAsync,
    isCreating: createMutation.isPending,
    update: updateMutation.mutateAsync,
    isUpdating: updateMutation.isPending,
    remove: deleteMutation.mutateAsync,
    isRemoving: deleteMutation.isPending,
  };
}
