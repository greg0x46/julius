import { useMutation, useQuery, useQueryClient, type UseQueryResult } from "@tanstack/react-query";
import { useCallback, useRef } from "react";

import { ApiError } from "../api/problems";
import {
  confirmStatement,
  downloadStatementTemplate,
  listImportAccounts,
  listStatementImports,
  previewStatement,
  type ImportAccount,
  type ImportHistoryItem,
  type ImportPreview,
} from "../api/statementImports";
import { invalidateAfterStatementImport, queryKeys } from "../api/queryKeys";

export type ImportListState<T> =
  | { kind: "loading" }
  | { kind: "ready"; items: T[] }
  | { kind: "empty" }
  | { kind: "unavailable" };

interface ConfirmRequest {
  file: File;
  preview: ImportPreview;
  target: { accountId?: string; newAccountName?: string };
  allowPartial: boolean;
  ambiguousDecisions?: Record<number, "import" | "ignore">;
}


function listState<T>(query: UseQueryResult<T[]>): ImportListState<T> {
  if (query.isPending) return { kind: "loading" };
  if (query.isError) return { kind: "unavailable" };
  return query.data.length === 0 ? { kind: "empty" } : { kind: "ready", items: query.data };
}

/**
 * Everything the statement import screen reads from or sends to the server:
 * the file-account list and the import history (each with its own state, so
 * one failing never hides the other), the preview of a file, its
 * confirmation, and the CSV template download.
 */
export function useStatementImports() {
  const queryClient = useQueryClient();
  const accountsQuery = useQuery({
    queryKey: queryKeys.statementImportAccounts,
    queryFn: ({ signal }) => listImportAccounts(signal),
  });
  const historyQuery = useQuery({
    queryKey: queryKeys.statementImportHistory,
    queryFn: ({ signal }) => listStatementImports(signal),
  });

  // A mutation observer follows only its latest call: a newer request, or a
  // reset, detaches it from the older one, so a late answer from a superseded
  // preview never reaches `data`.
  const previewMutation = useMutation({
    mutationFn: ({ file, accountId }: { file: File; accountId?: string }) => previewStatement(file, accountId),
  });

  // `isPending` reaches the screen a tick after the click, so a second click
  // could still slip through; the lock closes that gap and, released by
  // onSettled, cannot stick even when the screen has moved on mid-request.
  const confirmLock = useRef(false);
  const confirmMutation = useMutation({
    mutationFn: ({ file, preview, target, allowPartial, ambiguousDecisions }: ConfirmRequest) =>
      confirmStatement(file, preview, target, allowPartial, ambiguousDecisions),
    onSuccess: () => invalidateAfterStatementImport(queryClient),
    onSettled: () => {
      confirmLock.current = false;
    },
  });

  const templateLock = useRef(false);
  const templateMutation = useMutation({
    mutationFn: downloadStatementTemplate,
    onSettled: () => {
      templateLock.current = false;
    },
  });

  const { mutate: runPreview, reset: resetPreview } = previewMutation;
  const { mutate: runConfirm, reset: resetConfirm } = confirmMutation;
  const { mutate: runTemplate } = templateMutation;

  const requestPreview = useCallback(
    (file: File, accountId?: string) => {
      resetConfirm();
      runPreview({ file, accountId });
    },
    [resetConfirm, runPreview],
  );
  // The file or the account changed: whatever was previewed or confirmed no
  // longer belongs to the screen, including a request still in flight.
  const discard = useCallback(() => {
    resetPreview();
    resetConfirm();
  }, [resetConfirm, resetPreview]);
  const submitConfirm = useCallback(
    (request: ConfirmRequest) => {
      if (confirmLock.current) return;
      confirmLock.current = true;
      runConfirm(request);
    },
    [runConfirm],
  );
  const downloadTemplate = useCallback(() => {
    if (templateLock.current) return;
    templateLock.current = true;
    runTemplate();
  }, [runTemplate]);

  // A conflict means the server moved on since the preview (the file was
  // imported meanwhile, or the account changed), so that preview is void.
  const conflict = confirmMutation.error instanceof ApiError && confirmMutation.error.kind === "conflict";

  return {
    accounts: { state: listState<ImportAccount>(accountsQuery), retry: () => void accountsQuery.refetch() },
    history: { state: listState<ImportHistoryItem>(historyQuery), retry: () => void historyQuery.refetch() },
    preview: {
      data: conflict ? null : (previewMutation.data ?? null),
      isPending: previewMutation.isPending,
      error: previewMutation.error,
      request: requestPreview,
    },
    confirm: {
      result: confirmMutation.data ?? null,
      isPending: confirmMutation.isPending,
      error: confirmMutation.error,
      submit: submitConfirm,
    },
    template: {
      isPending: templateMutation.isPending,
      error: templateMutation.error,
      download: downloadTemplate,
    },
    discard,
  };
}
