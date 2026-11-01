import type { Page, Response } from "@playwright/test";
import {
  parseAccountList, parseCategoryList, parseInvestmentOperationList,
  parseInvestmentSummary, parseTransactionQueryResult,
} from "../src/api/contracts";
import { sumDecimals } from "../src/presentation/decimal";
import { credentials, expect, test } from "./fixtures";

async function login(page: Page) {
  await page.goto("/contas-e-cartoes");
  await page.getByLabel("E-mail", { exact: true }).fill(credentials.email);
  await page.getByLabel("Senha", { exact: true }).fill(credentials.password);
  await page.getByRole("button", { name: "Entrar", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Contas e cartões", exact: true })).toBeVisible();
}

function writeResponse(page: Page, path: string, method = "POST") {
  return page.waitForResponse((response) => new URL(response.url()).pathname === path && response.request().method() === method);
}

async function successful(response: Promise<Response>) {
  const result = await response;
  expect(result.ok(), await result.text()).toBeTruthy();
}

async function read<T>(page: Page, path: string, parse: (value: unknown) => T): Promise<T> {
  const result = await page.request.get(path);
  expect(result.ok(), await result.text()).toBeTruthy();
  return parse(await result.json());
}

async function ledger(page: Page) {
  const result = await page.request.post("/api/transactions/query", {
    headers: { Origin: new URL(page.url()).origin, "X-Julius-Request": "1" },
    data: {
      timezone: "America/Sao_Paulo", group_by: "none", page: 1, page_size: 100,
      filters: { date_from: null, date_to: null, description: null, account_ids: [], category_ids: [], classification: null, amount_min: null, amount_max: null, uncategorized: null },
    },
  });
  expect(result.ok(), await result.text()).toBeTruthy();
  return parseTransactionQueryResult(await result.json());
}

function ledgerState(result: ReturnType<typeof parseTransactionQueryResult>) {
  // Query confirmation and identity FX fetch timestamps are generated on each read.
  return {
    ...result,
    confirmed_at: undefined,
    ...(result.fx?.source === "identity" ? { fx: { ...result.fx, fetched_at: undefined } } : {}),
  };
}

async function createAccount(page: Page, name: string, opening: string) {
  await page.goto("/contas-e-cartoes");
  await page.getByRole("button", { name: "Nova conta", exact: true }).first().click();
  const form = page.getByRole("dialog", { name: "Nova conta bancária", exact: true });
  await form.getByLabel("Nome", { exact: true }).fill(name);
  await form.getByLabel("Saldo inicial", { exact: true }).fill(opening);
  const response = writeResponse(page, "/api/accounts");
  await form.getByRole("button", { name: "Criar conta", exact: true }).click();
  await successful(response);
  await expect(form).not.toBeVisible();
}

async function select(page: Page, label: string, option: string) {
  const control = page.getByRole("combobox", { name: label, exact: true });
  await control.press("ArrowDown");
  if (await control.isEditable()) await control.fill(option);
  await page.getByRole("option", { name: option, exact: true }).click();
}

async function transaction(page: Page, account: string, description: string, amount: string, inflow = false, category?: string, date = "2026-09-15") {
  await page.goto("/transacoes?period=all");
  await page.getByRole("button", { name: "Nova transação", exact: true }).click();
  const form = page.getByRole("dialog", { name: "Nova transação", exact: true });
  await select(page, "Conta", account);
  await form.getByLabel("Descrição", { exact: true }).fill(description);
  if (inflow) await form.getByText("Entrada", { exact: true }).click();
  await form.getByLabel("Valor", { exact: true }).fill(amount);
  await form.getByLabel("Data", { exact: true }).fill(date);
  await form.getByLabel("Data", { exact: true }).press("Tab");
  if (category) await select(page, "Categoria (opcional)", category);
  const response = writeResponse(page, "/api/transactions");
  await form.getByRole("button", { name: "Salvar", exact: true }).click();
  await successful(response);
  await expect(form).not.toBeVisible();
}

async function categorize(page: Page, description: string, category: string) {
  await page.goto("/transacoes?period=all");
  await page.getByRole("button", { name: `Ver detalhes de ${description}`, exact: true }).click();
  await page.getByRole("button", { name: "Categoria", exact: true }).click();
  await page.getByRole("textbox", { name: "Buscar categoria", exact: true }).fill(category);
  const response = page.waitForResponse((result) => result.url().endsWith("/category") && result.request().method() === "PUT");
  await page.getByRole("option", { name: category, exact: true }).last().click();
  await successful(response);
  await expect(page.getByRole("button", { name: "Categoria", exact: true })).toContainText(category);
}

async function expectTotals(page: Page, inflow: string, outflow: string, balance: string) {
  const result = await ledger(page);
  const totals = { currency_code: "BRL", inflow, outflow, balance };
  expect(result.reporting?.status).toBe("complete");
  const reported = result.reporting!.totals!;
  expect({ ...reported, inflow: sumDecimals([reported.inflow]), outflow: sumDecimals([reported.outflow]), balance: sumDecimals([reported.balance]) }).toEqual(totals);
  expect(result.totals).toEqual(inflow === "0.00" && outflow === "0.00" ? [] : [totals]);
}

async function expectSummary(page: Page, result: string) {
  await page.goto("/transacoes?period=all");
  await page.reload();
  await expect(page.getByRole("region", { name: "Resumo financeiro" })).toContainText(result);
}

test("authentication rejects invalid credentials, retains session and revokes it on logout", async ({ page }) => {
  await page.goto("/contas-e-cartoes");
  expect((await page.request.get("/api/accounts")).status()).toBe(401);
  await page.getByLabel("E-mail", { exact: true }).fill(credentials.email);
  await page.getByLabel("Senha", { exact: true }).fill("wrong-password");
  const rejected = writeResponse(page, "/api/auth/login");
  await page.getByRole("button", { name: "Entrar", exact: true }).click();
  expect((await rejected).status()).toBe(401);
  await expect(page.getByRole("alert")).toBeVisible();
  await login(page);
  await createAccount(page, "Carteira", "1000.25");
  await page.reload();
  expect(await read(page, "/api/accounts", parseAccountList)).toEqual([expect.objectContaining({ name: "Carteira", balance: "1000.25" })]);
  const cookies = await page.context().cookies();
  await page.goto("/configuracoes/seguranca");
  const loggedOut = writeResponse(page, "/api/auth/logout");
  await page.getByRole("button", { name: "Sair", exact: true }).click();
  await successful(loggedOut);
  await expect(page.getByRole("button", { name: "Entrar", exact: true })).toBeVisible();
  // Replaying the old cookie proves server-side revocation, not just UI logout.
  await page.context().addCookies(cookies);
  expect((await page.request.get("/api/accounts")).status()).toBe(401);
  await page.reload();
  await expect(page.getByRole("button", { name: "Entrar", exact: true })).toBeVisible();
});

test("manual entries and categorization persist; own-account transfers move cash without income or expense", async ({ page }) => {
  await login(page);
  await createAccount(page, "Conta principal", "1000.25");
  await createAccount(page, "Reserva", "200");
  const categories = await read(page, "/api/categories", parseCategoryList);
  const expense = categories.find((category) => category.kind === "expense")!;
  const transfer = categories.find((category) => category.name === "Transferência entre Contas Próprias")!;
  await transaction(page, "Conta principal", "Mercado sintético", "125,50");
  await categorize(page, "Mercado sintético", expense.name);
  await transaction(page, "Conta principal", "Salário sintético", "500", true);
  await transaction(page, "Conta principal", "Transferência enviada", "300", false, `Transferência: ${transfer.name}`);
  await transaction(page, "Reserva", "Transferência recebida", "300", true, `Transferência: ${transfer.name}`);
  await expectSummary(page, "374,50");
  await expectTotals(page, "500.00", "125.50", "374.50");
  const saved = await ledger(page);
  expect(saved.items).toHaveLength(4);
  expect(saved.items.find((item) => item.description === "Mercado sintético")?.internal_category).toMatchObject({ id: expense.id, origin: "manual" });
  for (const item of saved.items.filter((item) => item.description?.startsWith("Transferência"))) {
    expect(item.totals_eligibility).toMatchObject({ included: false, reason: "transfer_category" });
  }
  expect(await read(page, "/api/accounts", parseAccountList)).toEqual(expect.arrayContaining([
    expect.objectContaining({ name: "Conta principal", balance: "1074.75" }),
    expect.objectContaining({ name: "Reserva", balance: "500.00" }),
  ]));
});

const csv = {
  name: "synthetic-flash.csv", mimeType: "text/csv",
  buffer: Buffer.from('Data,Hora,Movimentação,Valor,Meio de Pagamento,Saldo\n08/09/2026,09:00,Depósito sintético,"R$ 500,00",Depósito,"R$ 500,00"\n15/09/2026,12:00,Almoço sintético,"-R$ 49,90",Cartão,"R$ 450,10"\n'),
};

test("file-provider statement reimport keeps identities, decisions, balance and totals", async ({ page }) => {
  await login(page);
  await page.goto("/contas-e-cartoes/importar");
  await page.getByLabel("Nome da nova conta").fill("Flash sintético");
  await page.getByLabel("Arquivo CSV").setInputFiles(csv);
  const imported = page.waitForResponse((response) => new URL(response.url()).pathname === "/api/statement-imports" && response.request().method() === "POST");
  await page.getByRole("button", { name: "Importar 2 transações", exact: true }).click();
  await successful(imported);
  await expect(page.getByText("Extrato importado", { exact: true })).toBeVisible();
  const expense = (await read(page, "/api/categories", parseCategoryList)).find((category) => category.kind === "expense")!;
  await categorize(page, "Almoço sintético", expense.name);
  const before = await ledger(page);
  expect(before.items).toHaveLength(2);
  await expectTotals(page, "500.00", "49.90", "450.10");
  await page.goto("/contas-e-cartoes/importar");
  await page.getByLabel("Usar conta existente").check();
  await select(page, "Conta de arquivo", "Flash sintético (BRL)");
  await page.getByLabel("Arquivo CSV").setInputFiles(csv);
  const repeated = page.waitForResponse((response) => new URL(response.url()).pathname === "/api/statement-imports" && response.request().method() === "POST");
  await page.getByRole("button", { name: "Registrar reenvio sem novas transações", exact: true }).click();
  await successful(repeated);
  await expect(page.getByText("Extrato conferido: nenhuma transação nova", { exact: true })).toBeVisible();
  await expectSummary(page, "450,10");
  const after = await ledger(page);
  expect(after.items).toEqual(before.items);
  expect(after.totals).toEqual(before.totals);
  expect(await read(page, "/api/accounts", parseAccountList)).toEqual([expect.objectContaining({ name: "Flash sintético", balance: "450.10" })]);
});

async function investmentOperation(page: Page, kind: string, amount: string) {
  await page.goto("/investimentos");
  await page.getByRole("button", { name: "Adicionar", exact: true }).click();
  await page.getByRole("menuitem", { name: /^Registrar movimentação/ }).click();
  const form = page.getByRole("dialog");
  await select(page, "Tipo", kind);
  await form.getByLabel("Valor", { exact: true }).fill(amount);
  await form.getByLabel("Data", { exact: true }).fill("2026-09-15");
  await form.getByLabel("Data", { exact: true }).press("Tab");
  const response = writeResponse(page, "/api/investment-operations");
  await form.getByRole("button", { name: "Salvar", exact: true }).click();
  await successful(response);
  await expect(form).not.toBeVisible();
}

async function linkInvestment(page: Page, description: string, kind: string) {
  await page.goto("/transacoes?period=all");
  await page.getByRole("button", { name: `Ver detalhes de ${description}`, exact: true }).click();
  await page.getByRole("button", { name: /Vincular a investimento/ }).click();
  await page.getByRole("combobox", { name: "Movimentação de destino", exact: true }).press("ArrowDown");
  await page.getByRole("listbox").getByText(kind, { exact: true }).click();
  const response = writeResponse(page, "/api/investment-reconciliations");
  await page.getByRole("button", { name: "Vincular", exact: true }).click();
  await successful(response);
}

test("investment contributions and withdrawals preserve bank cash and exclude reconciled principal from totals", async ({ page }) => {
  await login(page);
  await createAccount(page, "Banco sintético", "1000");
  await page.goto("/investimentos");
  await page.getByRole("button", { name: "Adicionar", exact: true }).click();
  await page.getByRole("menuitem", { name: /^Nova conta de investimento/ }).click();
  const form = page.getByRole("dialog", { name: "Nova conta de investimento", exact: true });
  await form.getByLabel("Nome", { exact: true }).fill("Corretora sintética");
  const created = writeResponse(page, "/api/investment-accounts");
  await form.getByRole("button", { name: "Criar conta", exact: true }).click();
  await successful(created);
  await expect(form).not.toBeVisible();
  await investmentOperation(page, "Aporte em caixa", "300");
  await investmentOperation(page, "Resgate da caixa", "100");
  await transaction(page, "Banco sintético", "Aporte sintético", "300");
  await transaction(page, "Banco sintético", "Resgate sintético", "100", true);
  await expectTotals(page, "100.00", "300.00", "-200.00");
  await linkInvestment(page, "Aporte sintético", "Aporte em caixa");
  await linkInvestment(page, "Resgate sintético", "Resgate da caixa");
  await expectSummary(page, "0,00");
  await expectTotals(page, "0.00", "0.00", "0.00");
  const saved = await ledger(page);
  expect(saved.items).toHaveLength(2);
  expect(saved.items.find((item) => item.description === "Aporte sintético")?.investment_transfer_amount).toBe("300.00");
  expect(saved.items.find((item) => item.description === "Resgate sintético")?.investment_transfer_amount).toBe("100.00");
  expect(await read(page, "/api/accounts", parseAccountList)).toEqual([expect.objectContaining({ name: "Banco sintético", balance: "800.00" })]);
  await page.goto("/investimentos");
  await page.reload();
  await expect(page.getByRole("region", { name: /^Carteira em/ })).toContainText("R$ 0,00");
  await page.getByRole("button", { name: "Contas", exact: true }).click();
  const drawer = page.getByRole("dialog", { name: "Contas de investimento", exact: true });
  await expect(drawer.getByRole("listitem").filter({ hasText: "Corretora sintética" })).toContainText("200,00");
  expect(await read(page, "/api/investment-summary", parseInvestmentSummary)).toMatchObject({ total_value: "200.00", cash_balance: "200.00" });
  const operations = await read(page, "/api/investment-operations", parseInvestmentOperationList);
  expect(operations).toHaveLength(2);
  expect(operations).toEqual(expect.arrayContaining([
    expect.objectContaining({ kind: "deposit", amount: "300.00", occurred_on: "2026-09-15" }),
    expect.objectContaining({ kind: "withdrawal", amount: "100.00", occurred_on: "2026-09-15" }),
  ]));
});

for (const mobile of [false, true]) {
  test(`discover and approve a recurring commitment (${mobile ? "mobile" : "desktop"})`, async ({ page }) => {
    if (mobile) await page.setViewportSize({ width: 390, height: 844 });
    await login(page);
    await createAccount(page, "Conta recorrente", "1000");
    const expense = (await read(page, "/api/categories", parseCategoryList)).find((c) => c.is_active && c.kind === "expense")!;
    const today = new Date();
    for (let offset = -6; offset < 0; offset++) {
      const date = new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth() + offset, 5)).toISOString().slice(0, 10);
      await transaction(page, "Conta recorrente", "Internet sintética", "100.12", false, `Despesa: ${expense.name}`, date);
    }
    const before = await ledger(page);
    await page.goto("/recorrencias");
    await page.getByRole("button", { name: mobile ? "Descobrir" : "Descobrir recorrências", exact: true }).click();
    await expect(page.getByRole("heading", { name: "Internet sintética", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Revisar", exact: true }).click();
    const drawer = page.getByRole("dialog");
    await drawer.getByRole("button", { name: "Atualizar prévia", exact: true }).click();
    await expect(drawer.getByRole("button", { name: "Criar recorrência e regra", exact: true })).toBeEnabled();
    await expect(drawer.getByText("Correspondência encontrada", { exact: false }).first()).toBeVisible();
    const response = writeResponse(page, "/api/recurrence-suggestions/accept");
    await drawer.getByRole("button", { name: "Criar recorrência e regra", exact: true }).click();
    await successful(response);
    await expect(drawer).not.toBeVisible();
    await expect(page.getByText("Nenhuma sugestão encontrada", { exact: true })).toBeVisible();
    const saved = await page.request.get("/api/recurring-scenarios");
    expect(await saved.json()).toEqual([expect.objectContaining({ name: "Internet sintética", amount: "100.12", is_active: true })]);
    expect(ledgerState(await ledger(page))).toEqual(ledgerState(before));
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy();
  });
}
