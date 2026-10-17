import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  createInvestmentAccount,
  createInvestmentOperation,
  createInvestmentOperations,
  createInvestmentTransfer,
  type InvestmentTransferWrite,
  type InvestmentCompoundWrite,
  createInvestmentPosition,
  createInvestmentReconciliation,
  deleteInvestmentAccount,
  deleteInvestmentOperation,
  deleteInvestmentPosition,
  deleteInvestmentReconciliation,
  deleteInvestmentTransfer,
  getInvestmentSummary,
  listInvestmentAccounts,
  listInvestmentOperations,
  listInvestmentPositions,
  listInvestmentReconciliations,
  updateInvestmentAccount,
  updateInvestmentOperation,
  updateInvestmentPosition,
} from "../api/investmentPortfolio";
import type {
  InvestmentAccountUpdate,
  InvestmentAccountWrite,
  InvestmentOperationWrite,
  InvestmentPositionUpdate,
  InvestmentPositionWrite,
  InvestmentReconciliationWrite,
} from "../api/contracts";
import { invalidateAfterLedgerChange, queryKeys } from "../api/queryKeys";

export const investmentWorkspaceQueryKey = queryKeys.investmentWorkspace;

/**
 * The investment ledger has a deliberately small, connected set of views.
 * One workspace cache keeps a new operation, its cash balance and the
 * summary moving together instead of leaving stale figures in another
 * tab of the same page.
 */
export function useInvestmentWorkspace({ enabled = true }: { enabled?: boolean } = {}) {
  const queryClient = useQueryClient();
  // `enabled` lets a component that only sometimes needs the ledger (the
  // transaction drawer's reconciliation panel) mount without firing six
  // requests; mutations stay available regardless.
  const accountsQuery = useQuery({
    queryKey: queryKeys.investmentWorkspacePart("accounts"),
    queryFn: ({ signal }) => listInvestmentAccounts(signal),
    enabled,
  });
  const positionsQuery = useQuery({
    queryKey: queryKeys.investmentWorkspacePart("positions"),
    queryFn: ({ signal }) => listInvestmentPositions({ includeClosed: true }, signal),
    enabled,
  });
  const operationsQuery = useQuery({
    queryKey: queryKeys.investmentWorkspacePart("operations"),
    queryFn: ({ signal }) => listInvestmentOperations({}, signal),
    enabled,
  });
  const reconciliationsQuery = useQuery({
    queryKey: queryKeys.investmentWorkspacePart("reconciliations"),
    queryFn: ({ signal }) => listInvestmentReconciliations({}, signal),
    enabled,
  });
  const summaryQuery = useQuery({
    queryKey: queryKeys.investmentWorkspacePart("summary"),
    queryFn: ({ signal }) => getInvestmentSummary(signal),
    enabled,
  });

  const refreshLedger = async () => {
    // A reconciliation changes what is reported as ordinary cash flow.
    await invalidateAfterLedgerChange(queryClient);
  };

  const createAccountMutation = useMutation({
    mutationFn: (write: InvestmentAccountWrite) => createInvestmentAccount(write),
    onSuccess: refreshLedger,
  });
  const updateAccountMutation = useMutation({
    mutationFn: ({ accountId, write }: { accountId: string; write: InvestmentAccountUpdate }) =>
      updateInvestmentAccount(accountId, write),
    onSuccess: refreshLedger,
  });
  const deleteAccountMutation = useMutation({
    mutationFn: (accountId: string) => deleteInvestmentAccount(accountId),
    onSuccess: refreshLedger,
  });

  const createPositionMutation = useMutation({
    mutationFn: (write: InvestmentPositionWrite) => createInvestmentPosition(write),
    onSuccess: refreshLedger,
  });
  const updatePositionMutation = useMutation({
    mutationFn: ({ positionId, write }: { positionId: string; write: InvestmentPositionUpdate }) =>
      updateInvestmentPosition(positionId, write),
    onSuccess: refreshLedger,
  });
  const deletePositionMutation = useMutation({
    mutationFn: (positionId: string) => deleteInvestmentPosition(positionId),
    onSuccess: refreshLedger,
  });

  const createOperationMutation = useMutation({
    mutationFn: (write: InvestmentOperationWrite) => createInvestmentOperation(write),
    onSuccess: refreshLedger,
  });
  const createOperationsMutation = useMutation({
    mutationFn: (write: InvestmentCompoundWrite) => createInvestmentOperations(write),
    onSuccess: refreshLedger,
  });
  const updateOperationMutation = useMutation({
    mutationFn: ({ operationId, write }: { operationId: string; write: InvestmentOperationWrite }) =>
      updateInvestmentOperation(operationId, write),
    onSuccess: refreshLedger,
  });
  const deleteOperationMutation = useMutation({
    mutationFn: (operationId: string) => deleteInvestmentOperation(operationId),
    onSuccess: refreshLedger,
  });
  const createTransferMutation = useMutation({
    mutationFn: (write: InvestmentTransferWrite) => createInvestmentTransfer(write),
    onSuccess: refreshLedger,
  });
  const deleteTransferMutation = useMutation({
    mutationFn: (transferId: string) => deleteInvestmentTransfer(transferId),
    onSuccess: refreshLedger,
  });

  const createReconciliationMutation = useMutation({
    mutationFn: (write: InvestmentReconciliationWrite) => createInvestmentReconciliation(write),
    onSuccess: refreshLedger,
  });
  const deleteReconciliationMutation = useMutation({
    mutationFn: (reconciliationId: string) => deleteInvestmentReconciliation(reconciliationId),
    onSuccess: refreshLedger,
  });

  const refetch = async () => {
    await Promise.all([
      accountsQuery.refetch(),
      positionsQuery.refetch(),
      operationsQuery.refetch(),
      reconciliationsQuery.refetch(),
      summaryQuery.refetch(),
    ]);
  };

  return {
    accounts: accountsQuery.data ?? [],
    positions: positionsQuery.data ?? [],
    operations: operationsQuery.data ?? [],
    reconciliations: reconciliationsQuery.data ?? [],
    summary: summaryQuery.data ?? null,
    isLoading:
      accountsQuery.isLoading ||
      positionsQuery.isLoading ||
      operationsQuery.isLoading ||
      reconciliationsQuery.isLoading ||
      summaryQuery.isLoading,
    isFetching:
      accountsQuery.isFetching ||
      positionsQuery.isFetching ||
      operationsQuery.isFetching ||
      reconciliationsQuery.isFetching ||
      summaryQuery.isFetching,
    error:
      accountsQuery.error ??
      positionsQuery.error ??
      operationsQuery.error ??
      reconciliationsQuery.error ??
      summaryQuery.error,
    refetch,
    createAccount: createAccountMutation.mutateAsync,
    updateAccount: updateAccountMutation.mutateAsync,
    deleteAccount: deleteAccountMutation.mutateAsync,
    createPosition: createPositionMutation.mutateAsync,
    updatePosition: updatePositionMutation.mutateAsync,
    deletePosition: deletePositionMutation.mutateAsync,
    createOperation: createOperationMutation.mutateAsync,
    createTransfer: createTransferMutation.mutateAsync,
    deleteTransfer: deleteTransferMutation.mutateAsync,
    createOperations: createOperationsMutation.mutateAsync,
    updateOperation: updateOperationMutation.mutateAsync,
    deleteOperation: deleteOperationMutation.mutateAsync,
    createReconciliation: createReconciliationMutation.mutateAsync,
    deleteReconciliation: deleteReconciliationMutation.mutateAsync,
    isSaving:
      createAccountMutation.isPending ||
      updateAccountMutation.isPending ||
      createPositionMutation.isPending ||
      updatePositionMutation.isPending ||
      createOperationMutation.isPending ||
      createTransferMutation.isPending ||
      createOperationsMutation.isPending ||
      updateOperationMutation.isPending ||
      createReconciliationMutation.isPending,
    isDeleting:
      deleteAccountMutation.isPending ||
      deletePositionMutation.isPending ||
      deleteOperationMutation.isPending ||
      deleteTransferMutation.isPending ||
      deleteReconciliationMutation.isPending,
  };
}
