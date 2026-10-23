import { credentials, expect, test } from "./fixtures";
import type { Page } from "@playwright/test";

async function login(page: Page) {
  await page.goto("/contas-e-cartoes");
  await page.getByLabel("E-mail", { exact: true }).fill(credentials.email);
  await page.getByLabel("Senha", { exact: true }).fill(credentials.password);
  await page.getByRole("button", { name: "Entrar", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Contas e cartões", exact: true }),
  ).toBeVisible();
}

test("local components render every primary workspace and settings route", async ({
  page,
}, testInfo) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await login(page);
  const routes = [
    ["/", "Início"],
    ["/transacoes", "Transações"],
    ["/pendencias", "Pendências"],
    ["/recorrencias", "Recorrências"],
    ["/investimentos", "Investimentos"],
    ["/metas", "Metas"],
    ["/patrimonio", "Patrimônio"],
    ["/configuracoes", "Configurações"],
    ["/configuracoes/geral", "Geral"],
    ["/configuracoes/seguranca", "Segurança"],
    ["/configuracoes/ativos-de-investimento", "Ativos de investimento"],
    ["/configuracoes/open-banking", "Open Banking"],
    ["/configuracoes/automacoes", "Automações"],
    ["/configuracoes/categorias", "Categorias"],
    ["/configuracoes/cenarios", "Cenários"],
    ["/contas-e-cartoes/importar", "Importar extrato"],
  ];
  for (const [path, title] of routes) {
    await page.goto(path);
    await expect(
      page.getByRole("heading", { name: title, exact: true, level: 1 }),
    ).toBeVisible();
    await expect(page.getByRole("heading", { level: 1 })).toHaveCount(1);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
  }
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "Início", exact: true }),
  ).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("desktop.png"),
    fullPage: true,
  });
  expect(errors).toEqual([]);
});

test("mobile navigation, selector Escape, dialog focus and dismissal use browser behavior", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await login(page);
  const menu = page.getByRole("button", { name: "Abrir navegação" });
  await menu.click();
  const navigation = page.getByRole("dialog", { name: "Julius", exact: true });
  await navigation.getByRole("link", { name: "Metas", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Metas", exact: true, level: 1 }),
  ).toBeVisible();
  await expect(navigation).not.toBeVisible();
  await menu.click();
  await page.keyboard.press("Escape");
  await expect(navigation).not.toBeVisible();
  await expect(menu).toBeFocused();
  await menu.click();
  await navigation
    .getByRole("link", { name: "Contas e cartões", exact: true })
    .click();
  const create = page
    .getByRole("button", { name: "Nova conta", exact: true })
    .first();
  await create.click();
  const form = page.getByRole("dialog", {
    name: "Nova conta bancária",
    exact: true,
  });
  const currency = form.getByRole("combobox", { name: "Moeda", exact: true });
  await currency.click();
  await expect(page.getByRole("listbox")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("listbox")).not.toBeVisible();
  await expect(form).toBeVisible();
  await currency.fill("BRL");
  const option = page.getByRole("option", { name: "BRL — Real brasileiro", exact: true });
  await expect(option).toBeVisible();
  const bounds = await option.boundingBox();
  expect(bounds!.x).toBeGreaterThanOrEqual(0);
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(390);
  await option.click();
  expect(await form.evaluate((element) => element.matches(":modal"))).toBe(true);
  for (let index = 0; index < 12; index++) {
    await page.keyboard.press("Tab");
    expect(
      await form.evaluate((element) =>
        document.activeElement === document.body || element.contains(document.activeElement),
      ),
    ).toBe(true);
  }
  // Native dialogs may send Tab to browser chrome, but keep page controls inert.
  await create.evaluate((element) => element.focus());
  await expect(create).not.toBeFocused();
  await form.getByLabel("Nome", { exact: true }).focus();
  const prefix = await form.getByText("R$", { exact: true }).boundingBox();
  expect(prefix!.height).toBeLessThanOrEqual(28);
  expect(prefix!.width).toBeGreaterThan(14);
  await page.screenshot({
    path: testInfo.outputPath("mobile-form.png"),
    fullPage: true,
  });
  await page.keyboard.press("Escape");
  await expect(form).not.toBeVisible();
  await expect(create).toBeFocused();
  expect(await page.evaluate(() => document.body.style.overflow)).not.toBe(
    "hidden",
  );
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
});
