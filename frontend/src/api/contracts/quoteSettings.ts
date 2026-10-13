import { isCount, isValidDate, requiredRecord } from "./shared";

export type QuoteRefreshStatus = "success" | "partial" | "deferred" | "failed";

export interface QuoteLastRefresh {
  finished_at: string;
  trigger: "scheduled" | "manual";
  status: QuoteRefreshStatus;
  assets: number;
  fetched: number;
  failures: number;
  deferred: number;
}

export interface QuoteSchedule {
  enabled: boolean;
  time: string;
  timezone: string;
}

export interface QuoteSettings extends QuoteSchedule {
  last_refresh: QuoteLastRefresh | null;
}

const invalid = "Configuração de cotações inválida.";
const statuses: readonly string[] = ["success", "partial", "deferred", "failed"];

export function parseQuoteLastRefresh(value: unknown): QuoteLastRefresh {
  const record = requiredRecord(
    value,
    ["finished_at", "trigger", "status", "assets", "fetched", "failures", "deferred"],
    invalid,
  );
  const { finished_at, trigger, status, assets, fetched, failures, deferred } = record;
  if (
    !isValidDate(finished_at) ||
    (trigger !== "scheduled" && trigger !== "manual") ||
    typeof status !== "string" ||
    !statuses.includes(status) ||
    ![assets, fetched, failures, deferred].every(isCount)
  ) {
    throw new TypeError(invalid);
  }
  return {
    finished_at,
    trigger,
    status: status as QuoteRefreshStatus,
    assets: assets as number,
    fetched: fetched as number,
    failures: failures as number,
    deferred: deferred as number,
  };
}

export function parseQuoteSettings(value: unknown): QuoteSettings {
  const record = requiredRecord(value, ["enabled", "time", "timezone", "last_refresh"], invalid);
  const { enabled, time, timezone, last_refresh } = record;
  if (typeof enabled !== "boolean" || typeof time !== "string" || typeof timezone !== "string") {
    throw new TypeError(invalid);
  }
  return {
    enabled,
    time,
    timezone,
    last_refresh: last_refresh === null ? null : parseQuoteLastRefresh(last_refresh),
  };
}
