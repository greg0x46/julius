import { apiFetch } from "./transport";
import { ApiError, isAbortError } from "./problems";

export interface ImportCounts {
  total: number;
  new: number;
  duplicate: number;
  ambiguous?: number;
  invalid: number;
}

export interface ImportRow {
  line_number: number;
  occurred_at: string | null;
  description: string;
  amount: string | null;
  currency: string | null;
  payment_method: string | null;
  balance: string | null;
  status: "new" | "duplicate" | "ambiguous" | "invalid";
  errors: string[];
  warnings: string[];
}

export interface ImportPreview {
  format: string;
  format_version: string;
  sha256: string;
  preview_fingerprint: string;
  filename: string;
  period_start: string | null;
  period_end: string | null;
  currency: string;
  counts: ImportCounts;
  rows: ImportRow[];
  warnings: string[];
}

export interface ImportAccount { id: string; name: string; currency_code: string }
export interface ImportResult { run_id: string; account_id: string; counts: ImportCounts }
export interface ImportHistoryItem {
  run_id: string;
  account_id: string;
  account_name: string;
  filename: string;
  format: string;
  created_at: string;
  counts: ImportCounts;
}

type RecordValue = Record<string, unknown>;
const record = (value: unknown): RecordValue => {
  if (value === null || typeof value !== "object" || Array.isArray(value)) throw new Error("Invalid response");
  return value as RecordValue;
};
const str = (value: unknown): string => {
  if (typeof value !== "string") throw new Error("Invalid response");
  return value;
};
const optionalStr = (value: unknown): string | null => value === null || value === undefined ? null : str(value);
const strings = (value: unknown): string[] => {
  if (!Array.isArray(value)) throw new Error("Invalid response");
  return value.map(str);
};
const counts = (value: unknown): ImportCounts => {
  const item = record(value);
  for (const key of ["total", "new", "duplicate", "invalid"] as const) {
    if (!Number.isInteger(item[key]) || (item[key] as number) < 0) throw new Error("Invalid response");
  }
  if (item.ambiguous !== undefined && (!Number.isInteger(item.ambiguous) || (item.ambiguous as number) < 0)) throw new Error("Invalid response");
  return { total: item.total as number, new: item.new as number, duplicate: item.duplicate as number, ambiguous: (item.ambiguous as number | undefined) ?? 0, invalid: item.invalid as number };
};

function parsePreview(value: unknown): ImportPreview {
  const item = record(value);
  if (!Array.isArray(item.rows)) throw new Error("Invalid response");
  return {
    format: str(item.format), format_version: str(item.format_version), sha256: str(item.sha256),
    preview_fingerprint: str(item.preview_fingerprint),
    filename: str(item.filename), period_start: optionalStr(item.period_start),
    period_end: optionalStr(item.period_end), currency: str(item.currency), counts: counts(item.counts),
    warnings: strings(item.warnings),
    rows: item.rows.map((value): ImportRow => {
      const row = record(value);
      if (!Number.isInteger(row.line_number) || !["new", "duplicate", "ambiguous", "invalid"].includes(String(row.status))) throw new Error("Invalid response");
      return {
        line_number: row.line_number as number, occurred_at: optionalStr(row.occurred_at),
        description: row.description === undefined ? "" : str(row.description), amount: optionalStr(row.amount), currency: optionalStr(row.currency),
        payment_method: optionalStr(row.payment_method), balance: optionalStr(row.balance),
        status: row.status as ImportRow["status"], errors: strings(row.errors), warnings: strings(row.warnings),
      };
    }),
  };
}

async function fetchExpecting(
  path: string, init: RequestInit, expectedStatus: number, fallback = "Não foi possível concluir a importação.",
): Promise<Response> {
  let response: Response;
  try { response = await apiFetch(path, { ...init, headers: { Accept: "application/json", ...init.headers } }); }
  catch (error) {
    if (isAbortError(error)) throw error;
    throw new ApiError("transport", "Não foi possível comunicar com o servidor.");
  }
  if (response.status !== expectedStatus) {
    let detail = fallback;
    try {
      const body = record(await response.json());
      if (typeof body.detail === "string") detail = body.detail;
      else if (typeof body.title === "string") detail = body.title;
    } catch { /* Keep the generic message. */ }
    throw new ApiError(response.status === 409 ? "conflict" : response.status === 404 ? "not_found" : "response", detail);
  }
  return response;
}

async function request<T>(path: string, init: RequestInit, expectedStatus: number, parse: (value: unknown) => T): Promise<T> {
  const response = await fetchExpecting(path, init, expectedStatus);
  try { return parse(await response.json()); }
  catch { throw new ApiError("response", "A resposta do servidor não pôde ser confirmada."); }
}

export function previewStatement(file: File, accountId?: string): Promise<ImportPreview> {
  const body = new FormData();
  body.append("file", file);
  if (accountId) body.append("account_id", accountId);
  return request("/api/statement-imports/preview", { method: "POST", body }, 200, parsePreview);
}

export function confirmStatement(file: File, preview: ImportPreview, target: { accountId?: string; newAccountName?: string }, allowPartial: boolean, ambiguousDecisions: Record<number, "import" | "ignore"> = {}): Promise<ImportResult> {
  const body = new FormData();
  body.append("file", file);
  if (target.accountId) body.append("account_id", target.accountId);
  if (target.newAccountName) body.append("new_account_name", target.newAccountName);
  body.append("expected_sha256", preview.sha256);
  body.append("expected_preview_fingerprint", preview.preview_fingerprint);
  body.append("expected_format", preview.format);
  body.append("expected_format_version", preview.format_version);
  body.append("allow_partial", String(allowPartial));
  for (const row of preview.rows) {
    if (row.status === "ambiguous" && ambiguousDecisions[row.line_number]) {
      body.append(`ambiguous_line_${row.line_number}`, ambiguousDecisions[row.line_number]);
    }
  }
  return request("/api/statement-imports", { method: "POST", body }, 201, (value) => {
    const item = record(value);
    return { run_id: str(item.run_id), account_id: str(item.account_id), counts: counts(item.counts) };
  });
}

export function listImportAccounts(signal?: AbortSignal): Promise<ImportAccount[]> {
  return request("/api/statement-imports/accounts", { method: "GET", signal }, 200, (value) => {
    if (!Array.isArray(value)) throw new Error("Invalid response");
    return value.map((entry) => {
      const item = record(entry);
      return { id: str(item.id), name: str(item.name), currency_code: str(item.currency_code) };
    });
  });
}

export function listStatementImports(signal?: AbortSignal): Promise<ImportHistoryItem[]> {
  return request("/api/statement-imports", { method: "GET", signal }, 200, (value) => {
    if (!Array.isArray(value)) throw new Error("Invalid response");
    return value.map((entry) => {
      const item = record(entry);
      return { run_id: str(item.run_id), account_id: str(item.account_id), account_name: str(item.account_name),
        filename: str(item.filename), format: str(item.format), created_at: str(item.created_at), counts: counts(item.counts) };
    });
  });
}

const templateFilename = "modelo-extrato-flash.csv";

/**
 * Saves the CSV template of the supported statement format through the
 * browser's download flow. Errors keep the ApiError kinds of the calls above,
 * so the page can show the server's own message.
 */
export async function downloadStatementTemplate(): Promise<void> {
  const response = await fetchExpecting(
    "/api/statement-imports/template", { method: "GET", headers: { Accept: "text/csv" } }, 200, "Não foi possível baixar o modelo.",
  );
  let blob: Blob;
  try { blob = await response.blob(); }
  catch (error) {
    if (isAbortError(error)) throw error;
    throw new ApiError("response", "O modelo não pôde ser baixado.");
  }
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = templateFilename;
  document.body.append(link);
  link.click();
  link.remove();
  // Revoking in the same tick can cancel the download in some browsers.
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
