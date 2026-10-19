import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  createRecurringCommitment,
  deleteRecurringCommitment,
  listRecurringCommitments,
  setRecurringCommitmentActive,
  updateRecurringCommitment,
} from "../api/recurringCommitments";
import type { RecurringCommitmentWrite } from "../api/contracts";
import { invalidateAfterRecurringCommitmentChange, queryKeys } from "../api/queryKeys";

export const recurringCommitmentsQueryKey = queryKeys.recurringCommitments;

export function useRecurringCommitments() {
  const queryClient = useQueryClient();
  const commitmentsQuery = useQuery({
    queryKey: recurringCommitmentsQueryKey,
    queryFn: ({ signal }) => listRecurringCommitments(signal),
  });

  const invalidate = () => invalidateAfterRecurringCommitmentChange(queryClient);

  const createMutation = useMutation({
    mutationFn: (write: RecurringCommitmentWrite) => createRecurringCommitment(write),
    onSuccess: invalidate,
  });

  const updateMutation = useMutation({
    mutationFn: ({
      commitmentId,
      write,
    }: {
      commitmentId: string;
      write: RecurringCommitmentWrite;
    }) => updateRecurringCommitment(commitmentId, write),
    onSuccess: invalidate,
  });

  const toggleMutation = useMutation({
    mutationFn: ({ commitmentId, isActive }: { commitmentId: string; isActive: boolean }) =>
      setRecurringCommitmentActive(commitmentId, isActive),
    onSuccess: invalidate,
  });

  const deleteMutation = useMutation({
    mutationFn: (commitmentId: string) => deleteRecurringCommitment(commitmentId),
    onSuccess: invalidate,
  });

  return {
    commitments: commitmentsQuery.data ?? [],
    isLoading: commitmentsQuery.isLoading,
    error: commitmentsQuery.error,
    refetch: commitmentsQuery.refetch,
    createCommitment: createMutation.mutateAsync,
    updateCommitment: updateMutation.mutateAsync,
    toggleCommitment: toggleMutation.mutateAsync,
    deleteCommitment: deleteMutation.mutateAsync,
    isSaving: createMutation.isPending || updateMutation.isPending,
    isDeleting: deleteMutation.isPending,
  };
}
