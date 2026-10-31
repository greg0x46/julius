import { useTranslation } from "../i18n/context";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";

import type { TransactionCategoryResult } from "../api/contracts";
import { setTransactionCategory } from "../api/transactions";
import { markTransactionsStale, queryKeys } from "../api/queryKeys";

export interface CategoryTarget {
  transactionId: string;
  categoryId: string;
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

export interface TransactionWriteCallbacks<Target, Result> {
  /** The write was confirmed by the API (the list refresh may still be pending). */
  onSaved?: (target: Target, result: Result) => void;
  /** The write failed; `writeError` carries the message for the inline Alert. */
  onFailed?: (target: Target, message: string) => void;
}

export function useTransactionCategory({ onSaved, onFailed }: TransactionWriteCallbacks<CategoryTarget, TransactionCategoryResult> = {}) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const locked = useRef(false);
  const [pendingTarget, setPendingTarget] = useState<CategoryTarget | null>(null);
  const [failedWrite, setFailedWrite] = useState<CategoryTarget | null>(null);
  const [writeError, setWriteError] = useState<string | null>(null);
  const [refreshError, setRefreshError] = useState<string | null>(null);
  const [announcement, setAnnouncement] = useState("");

  const refresh = async () => {
    setRefreshError(null);
    try {
      await queryClient.refetchQueries(
        { queryKey: queryKeys.transactions, type: "active" },
        { throwOnError: true },
      );
      setAnnouncement(t("categoryTotalsUpdated"));
    } catch (error) {
      setRefreshError(errorMessage(error, t("categorySaveErrorFull")));
      setAnnouncement(t("categorySavedResultsPending"));
    }
  };

  const mutation = useMutation({
    mutationFn: (target: CategoryTarget) =>
      setTransactionCategory(target.transactionId, target.categoryId),
    onMutate: async (target) => {
      setPendingTarget(target);
      setFailedWrite(null);
      setWriteError(null);
      setRefreshError(null);
      setAnnouncement(t("savingTransactionCategory"));
      await queryClient.cancelQueries({ queryKey: queryKeys.transactions });
    },
    onSuccess: async (result, target) => {
      onSaved?.(target, result);
      await markTransactionsStale(queryClient);
      await refresh();
    },
    onError: (error, target) => {
      setFailedWrite(target);
      setWriteError(errorMessage(error, t("categorySaveErrorFull")));
      onFailed?.(target, errorMessage(error, t("categorySaveErrorFull")));
      setAnnouncement(t("categorySaveFailedRetained"));
    },
    onSettled: () => {
      locked.current = false;
      setPendingTarget(null);
    },
  });

  return {
    setCategory: (target: CategoryTarget) => {
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
