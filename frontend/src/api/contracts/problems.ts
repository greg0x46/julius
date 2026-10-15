import { parseReportingSummary, type ReportingSummary } from "./reporting";
import { isRecord, isUuid, parseCurrencyTotals, type CurrencyTotals } from "./shared";

export interface Problem {
  reporting?: ReportingSummary;
  totals?: CurrencyTotals[];
  type: string;
  title: string;
  status: number;
  detail?: string;
  active_sync_run_id?: string | null;
}

export function parseProblem(value: unknown): Problem {
  if (
    !isRecord(value) ||
    (value.totals !== undefined && !Array.isArray(value.totals)) ||
    typeof value.type !== "string" ||
    typeof value.title !== "string" ||
    !Number.isInteger(value.status) ||
    (value.detail !== undefined && typeof value.detail !== "string") ||
    (value.active_sync_run_id !== undefined &&
      value.active_sync_run_id !== null &&
      (typeof value.active_sync_run_id !== "string" || !isUuid(value.active_sync_run_id)))
  ) {
    throw new TypeError("Resposta de problema inválida.");
  }
  return {
    ...(value.reporting === undefined ? {} : { reporting: parseReportingSummary(value.reporting) }),
    ...(Array.isArray(value.totals) ? { totals: value.totals.map(parseCurrencyTotals) } : {}),
    type: value.type,
    title: value.title,
    status: value.status as number,
    ...(value.detail === undefined ? {} : { detail: value.detail }),
    ...(value.active_sync_run_id === undefined
      ? {}
      : { active_sync_run_id: value.active_sync_run_id }),
  };
}
