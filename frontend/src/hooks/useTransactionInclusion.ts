import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";

import type { TransactionInclusionResult, TransactionInclusionState } from "../api/contracts";
import { setTransactionInclusion } from "../api/transactions";
import type { TransactionWriteCallbacks } from "./useTransactionCategory";
import { markTransactionInclusionViewsStale, queryKeys } from "../api/queryKeys";

export interface InclusionTarget {
  transactionId: string;
  state: TransactionInclusionState;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Não foi possível salvar a decisão.";
}

export function useTransactionInclusion({ onSaved, onFailed }: TransactionWriteCallbacks<InclusionTarget, TransactionInclusionResult> = {}) {
  const queryClient = useQueryClient();
  const locked = useRef(false);
  const [pendingTarget, setPendingTarget] = useState<InclusionTarget | null>(null);
  const [failedWrite, setFailedWrite] = useState<InclusionTarget | null>(null);
  const [writeError, setWriteError] = useState<string | null>(null);
  const [refreshError, setRefreshError] = useState<string | null>(null);
  const [announcement, setAnnouncement] = useState("");

  const refresh = async (state?: TransactionInclusionState) => {
    setRefreshError(null);
    try {
      await Promise.all(
        [queryKeys.transactions, queryKeys.transactionTransfers].map((queryKey) =>
          queryClient.refetchQueries(
            { queryKey, type: "active" },
            { throwOnError: true },
          ),
        ),
      );
      setAnnouncement(
        state === "ignored"
          ? "Transação ignorada. Totais atualizados."
          : state === "considered"
            ? "Transação restaurada. Totais atualizados."
            : "Resultados atualizados.",
      );
    } catch (error) {
      setRefreshError(errorMessage(error));
      setAnnouncement("Alteração salva, atualização dos resultados pendente.");
    }
  };

  const mutation = useMutation({
    mutationFn: (target: InclusionTarget) =>
      setTransactionInclusion(target.transactionId, target.state),
    onMutate: async (target) => {
      setPendingTarget(target);
      setFailedWrite(null);
      setWriteError(null);
      setRefreshError(null);
      setAnnouncement("Salvando decisão da transação…");
      await queryClient.cancelQueries({ queryKey: queryKeys.transactions });
    },
    onSuccess: async (confirmation, target) => {
      onSaved?.(target, confirmation);
      await markTransactionInclusionViewsStale(queryClient);
      await refresh(confirmation.state);
    },
    onError: (error, target) => {
      setFailedWrite(target);
      setWriteError(errorMessage(error));
      onFailed?.(target, errorMessage(error));
      setAnnouncement("Falha ao salvar. O último estado confirmado foi mantido.");
    },
    onSettled: () => {
      locked.current = false;
      setPendingTarget(null);
    },
  });

  return {
    setInclusion: (target: InclusionTarget) => {
      if (locked.current) return;
      locked.current = true;
      mutation.mutate(target);
    },
    retryWrite: () => {
      if (!failedWrite || locked.current) return;
      locked.current = true;
      mutation.mutate(failedWrite);
    },
    retryRefresh: () => refresh(),
    pendingTarget,
    failedTarget: failedWrite,
    isPending: mutation.isPending,
    writeError,
    refreshError,
    announcement,
  };
}
