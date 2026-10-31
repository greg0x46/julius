import { translate, type Language } from "../i18n/messages";
import type { FailureStage, SyncStatus } from "../api/contracts";

const statusMetadata = {
  in_progress: { label: "syncInProgress", symbol: "↻", tone: "progress" },
  completed: { label: "syncCompleted", symbol: "✓", tone: "success" },
  completed_with_failures: {
    label: "syncCompletedFailures",
    symbol: "!",
    tone: "warning",
  },
  failed: { label: "syncFailed", symbol: "×", tone: "danger" },
} as const satisfies Record<SyncStatus, { label: string; symbol: string; tone: string }>;

const stageLabels = {
  auth: "authentication",
  item: "connection",
  accounts: "listAccounts",
  account: "account",
  transactions: "transactions",
  normalize: "normalization",
  interrupted: "interruption",
  worker_unavailable: "processingUnavailable",
} as const satisfies Record<FailureStage, string>;

export const getStatusMetadata = (status: SyncStatus, language: Language = "pt-BR") => ({ ...statusMetadata[status], label: translate(language, statusMetadata[status].label) });
export const getFailureStageLabel = (stage: FailureStage, language: Language = "pt-BR") => translate(language, stageLabels[stage]);
export const isFinalStatus = (status: SyncStatus): boolean => status !== "in_progress";
