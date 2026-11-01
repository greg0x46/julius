import { useTranslation } from "../i18n/context";
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

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

export function useTransactionInclusion({ onSaved, onFailed }: TransactionWriteCallbacks<InclusionTarget, TransactionInclusionResult> = {}) {
  const { t } = useTranslation();
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
          ? t("ignoredTotalsUpdated")
          : state === "considered"
            ? t("restoredTotalsUpdated")
            : t("resultsUpdated"),
      );
    } catch (error) {
      setRefreshError(errorMessage(error, t("decisionSaveErrorFull")));
      setAnnouncement(t("changeSavedRefreshPending"));
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
      setAnnouncement(t("savingTransactionDecision"));
      await queryClient.cancelQueries({ queryKey: queryKeys.transactions });
    },
    onSuccess: async (confirmation, target) => {
      onSaved?.(target, confirmation);
      await markTransactionInclusionViewsStale(queryClient);
      await refresh(confirmation.state);
    },
    onError: (error, target) => {
      setFailedWrite(target);
      setWriteError(errorMessage(error, t("decisionSaveErrorFull")));
      onFailed?.(target, errorMessage(error, t("decisionSaveErrorFull")));
      setAnnouncement(t("decisionSaveFailedRetained"));
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
