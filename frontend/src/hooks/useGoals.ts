import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { createGoal, deleteGoal, listGoals, updateGoal } from "../api/goals";
import type { GoalWrite } from "../api/contracts";
import { invalidateAfterGoalChange, queryKeys } from "../api/queryKeys";

export function useGoals() {
  const queryClient = useQueryClient();
  const goalsQuery = useQuery({
    queryKey: queryKeys.goals,
    queryFn: ({ signal }) => listGoals(signal),
  });

  const refresh = () => invalidateAfterGoalChange(queryClient);

  const createMutation = useMutation({
    mutationFn: (write: GoalWrite) => createGoal(write),
    onSuccess: refresh,
  });
  const updateMutation = useMutation({
    mutationFn: ({ goalId, write }: { goalId: string; write: GoalWrite }) => updateGoal(goalId, write),
    onSuccess: refresh,
  });
  const deleteMutation = useMutation({
    mutationFn: (goalId: string) => deleteGoal(goalId),
    onSuccess: refresh,
  });

  return {
    goals: goalsQuery.data ?? [],
    isLoading: goalsQuery.isLoading,
    error: goalsQuery.error,
    refetch: () => goalsQuery.refetch(),
    createGoal: createMutation.mutateAsync,
    updateGoal: updateMutation.mutateAsync,
    deleteGoal: deleteMutation.mutateAsync,
    isSaving: createMutation.isPending || updateMutation.isPending,
    isDeleting: deleteMutation.isPending,
  };
}
