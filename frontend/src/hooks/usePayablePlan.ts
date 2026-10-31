import { apiMessage } from "../i18n/apiMessage";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  createPayableScenario,
  createRealization,
  deleteRealization,
  deleteScenarioTransaction,
  generateInstallments,
  getScenario,
  listPayableScenarios,
  readjustInstallments,
} from "../api/scenarios";
import type {
  GenerateInstallmentsWrite,
  ReadjustWrite,
  RealizationWrite,
  ScenarioDetail,
} from "../api/contracts";
import { invalidateAfterPayablePlanChange, queryKeys } from "../api/queryKeys";

export const payableScenariosQueryKey = queryKeys.payableScenarios;

// usePayablePlan surfaces the accounting Scenario for a payable. Simulation
// scenarios may share the payable, so position in the API array is not a
// valid selector; the canonical row is marked is_accounting_source.
export function usePayablePlan(payableId: string) {
  const queryClient = useQueryClient();

  const listQuery = useQuery({
    queryKey: payableScenariosQueryKey(payableId),
    queryFn: ({ signal }) => listPayableScenarios(payableId, signal),
  });
  const planSummary =
    listQuery.data?.find((scenario) => scenario.is_accounting_source === true) ?? null;

  const detailQuery = useQuery({
    queryKey: [...payableScenariosQueryKey(payableId), planSummary?.id ?? null, "detail"],
    queryFn: ({ signal }) => getScenario(planSummary!.id, signal),
    enabled: planSummary !== null,
  });

  const invalidate = () => {
    // Installments generated/allocated/readjusted here are exactly what the
    // unified projector reads, so any cached balance curve or period total
    // is now stale.
    void invalidateAfterPayablePlanChange(queryClient, payableId);
  };

  const createMutation = useMutation({
    mutationFn: (name: string) => createPayableScenario(payableId, { name }),
    onSuccess: invalidate,
  });

  const generateMutation = useMutation({
    mutationFn: (write: GenerateInstallmentsWrite) => {
      if (planSummary === null) throw new Error(apiMessage("apiNoPlanForInstallments"));
      return generateInstallments(planSummary.id, write);
    },
    onSuccess: invalidate,
  });

  const deleteTransactionMutation = useMutation({
    mutationFn: (transactionId: string) => {
      if (planSummary === null) throw new Error(apiMessage("apiNoActivePlan"));
      return deleteScenarioTransaction(planSummary.id, transactionId);
    },
    onSuccess: invalidate,
  });

  const allocateMutation = useMutation({
    mutationFn: ({ transactionId, write }: { transactionId: string; write: RealizationWrite }) => {
      if (planSummary === null) throw new Error(apiMessage("apiNoActivePlan"));
      return createRealization(planSummary.id, transactionId, write);
    },
    onSuccess: invalidate,
  });

  const deallocateMutation = useMutation({
    mutationFn: ({ transactionId, realizationId }: { transactionId: string; realizationId: string }) => {
      if (planSummary === null) throw new Error(apiMessage("apiNoActivePlan"));
      return deleteRealization(planSummary.id, transactionId, realizationId);
    },
    onSuccess: invalidate,
  });

  const readjustMutation = useMutation({
    mutationFn: (write: ReadjustWrite) => {
      if (planSummary === null) throw new Error(apiMessage("apiNoActivePlan"));
      return readjustInstallments(planSummary.id, write);
    },
    onSuccess: invalidate,
  });

  const plan: ScenarioDetail | null = detailQuery.data ?? null;

  return {
    plan,
    isLoading: listQuery.isLoading || (planSummary !== null && detailQuery.isLoading),
    error: listQuery.error ?? detailQuery.error,
    createPlan: createMutation.mutateAsync,
    isCreating: createMutation.isPending,
    generateInstallments: generateMutation.mutateAsync,
    isGenerating: generateMutation.isPending,
    deleteInstallment: deleteTransactionMutation.mutateAsync,
    isDeletingInstallment: deleteTransactionMutation.isPending,
    allocateRealization: allocateMutation.mutateAsync,
    isAllocating: allocateMutation.isPending,
    deallocateRealization: deallocateMutation.mutateAsync,
    isDeallocating: deallocateMutation.isPending,
    readjust: readjustMutation.mutateAsync,
    isReadjusting: readjustMutation.isPending,
  };
}
