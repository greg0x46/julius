import { afterEach, expect, it, vi } from "vitest";
import { ApiError } from "./problems";
import { confirmStatement, downloadStatementTemplate, previewStatement } from "./statementImports";
import { expireSession } from "./transport";

afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers(); expireSession(); });

it("accepts omitted fields on an invalid row and sends the preview identity on confirmation", async () => {
  const preview = {
    format: "flash_csv", format_version: "1", sha256: "abc", preview_fingerprint: "fingerprint", filename: "sample.csv",
    currency: "BRL", counts: { total: 1, new: 0, duplicate: 0, invalid: 1 }, warnings: [],
    rows: [{ line_number: 2, status: "invalid", errors: ["valor inválido"], warnings: [] }],
  };
  const fetchMock = vi.fn()
    .mockResolvedValueOnce(new Response(JSON.stringify(preview), { status: 200, headers: { "content-type": "application/json" } }))
    .mockResolvedValueOnce(new Response(JSON.stringify({ run_id: "run", account_id: "acc", counts: preview.counts }),
      { status: 201, headers: { "content-type": "application/json" } }));
  vi.stubGlobal("fetch", fetchMock);
  const file = new File(["Data,Hora\n"], "sample.csv", { type: "text/csv" });

  const parsed = await previewStatement(file);
  expect(parsed.rows[0]).toMatchObject({ occurred_at: null, amount: null, description: "", errors: ["valor inválido"] });
  await confirmStatement(file, parsed, { newAccountName: "Flash" }, true);
  const init = fetchMock.mock.calls[1][1] as RequestInit;
  const body = init.body as FormData;
  expect(body.get("expected_sha256")).toBe("abc");
  expect(body.get("expected_preview_fingerprint")).toBe("fingerprint");
  expect(body.get("expected_format_version")).toBe("1");
  expect(body.get("new_account_name")).toBe("Flash");
  expect(body.get("allow_partial")).toBe("true");
  expect(new Headers(init.headers).get("X-Julius-Request")).toBe("1");
  expect(new Headers(init.headers).has("content-type")).toBe(false);
});

it("sends the selected decision for an ambiguous statement line", async () => {
  const preview = {
    format: "flash_csv", format_version: "1", sha256: "other-file", preview_fingerprint: "reviewed", filename: "other.csv",
    currency: "BRL", counts: { total: 1, new: 0, duplicate: 0, ambiguous: 1, invalid: 0 }, warnings: [],
    rows: [{ line_number: 7, status: "ambiguous", errors: [], warnings: ["movimentação idêntica"] }],
  };
  const fetchMock = vi.fn()
    .mockResolvedValueOnce(new Response(JSON.stringify(preview), { status: 200 }))
    .mockResolvedValueOnce(new Response(JSON.stringify({ run_id: "run", account_id: "acc", counts: preview.counts }), { status: 201 }));
  vi.stubGlobal("fetch", fetchMock);
  const file = new File(["synthetic"], "other.csv", { type: "text/csv" });
  const parsed = await previewStatement(file, "acc");
  expect(parsed.rows[0].status).toBe("ambiguous");
  await confirmStatement(file, parsed, { accountId: "acc" }, false, { 7: "import" });
  const body = fetchMock.mock.calls[1][1].body as FormData;
  expect(body.get("ambiguous_line_7")).toBe("import");
  expect(body.get("expected_preview_fingerprint")).toBe("reviewed");
});

// jsdom has no object URLs and would try to navigate on an anchor click, so the
// download flow is observed through stand-ins for both.
function stubDownload() {
  const created: Blob[] = [];
  const clicked: { download: string; href: string; attached: boolean }[] = [];
  const revoked: string[] = [];
  Object.defineProperty(URL, "createObjectURL", { configurable: true, writable: true, value: vi.fn((blob: Blob) => { created.push(blob); return "blob:template"; }) });
  Object.defineProperty(URL, "revokeObjectURL", { configurable: true, writable: true, value: vi.fn((url: string) => { revoked.push(url); }) });
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) {
    clicked.push({ download: this.download, href: this.href, attached: this.isConnected });
  });
  return { created, clicked, revoked };
}

it("downloads the CSV template through an anchor and revokes the object URL afterwards", async () => {
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  const csv = "Data,Hora,Movimentação\n";
  const fetchMock = vi.fn().mockResolvedValue(new Response(csv,
    { status: 200, headers: { "content-type": "text/csv; charset=utf-8", "content-disposition": 'attachment; filename="modelo-extrato-flash.csv"' } }));
  vi.stubGlobal("fetch", fetchMock);
  const { created, clicked, revoked } = stubDownload();

  await downloadStatementTemplate();

  expect(fetchMock.mock.calls[0][0]).toBe("/api/statement-imports/template");
  const init = fetchMock.mock.calls[0][1] as RequestInit;
  expect(init.method).toBe("GET");
  expect(new Headers(init.headers).get("Accept")).toBe("text/csv");
  expect(created).toHaveLength(1);
  expect(created[0].type).toMatch(/^text\/csv/);
  expect(created[0].size).toBe(new Blob([csv]).size);
  expect(clicked).toEqual([{ download: "modelo-extrato-flash.csv", href: "blob:template", attached: true }]);
  expect(document.querySelector("a[download]")).toBeNull();
  expect(revoked).toEqual([]);
  await vi.advanceTimersByTimeAsync(1000);
  expect(revoked).toEqual(["blob:template"]);
});

it("surfaces a non-200 template response as an error without downloading anything", async () => {
  const problem = { title: "Falha", detail: "Modelo indisponível no momento." };
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify(problem),
    { status: 500, headers: { "content-type": "application/problem+json" } })));
  const { created, clicked } = stubDownload();

  await expect(downloadStatementTemplate()).rejects.toMatchObject({ kind: "response", message: "Modelo indisponível no momento." });
  expect(created).toEqual([]);
  expect(clicked).toEqual([]);
});

it("uses a template-specific message when the failure carries no problem body", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("<html>bad gateway</html>", { status: 502 })));
  stubDownload();

  await expect(downloadStatementTemplate()).rejects.toMatchObject({ kind: "response", message: "Não foi possível baixar o modelo." });
});

it("reports a transport failure while downloading the template", async () => {
  vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("offline")));
  const { created } = stubDownload();

  const failure = await downloadStatementTemplate().catch((error: unknown) => error);
  expect(failure).toBeInstanceOf(ApiError);
  expect(failure).toMatchObject({ kind: "transport" });
  expect(created).toEqual([]);
});
