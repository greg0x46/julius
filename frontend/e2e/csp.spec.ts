import { credentials, expect, test } from "./fixtures";
import type { Page } from "@playwright/test";

type CspWindow = Window & { __csp?: string[] };

// The e2e server sends the same headers as the binary (httpapi.SecurityHeaders).
async function violations(page: Page) {
  return page.evaluate(() => (window as CspWindow).__csp ?? []);
}

test("pages run under the response CSP without violations", async ({ page }) => {
  await page.addInitScript(() => {
    const seen: string[] = [];
    (window as CspWindow).__csp = seen;
    document.addEventListener("securitypolicyviolation", (event) => {
      seen.push(`${event.violatedDirective} ${event.blockedURI}`);
    });
  });

  const response = await page.goto("/contas-e-cartoes");
  const headers = response!.headers();
  expect(headers["x-content-type-options"]).toBe("nosniff");
  expect(headers["x-frame-options"]).toBe("DENY");
  expect(headers["referrer-policy"]).toBe("same-origin");
  expect(headers["content-security-policy"]).toContain("frame-ancestors 'none'");
  expect(headers["strict-transport-security"]).toBeUndefined();

  await page.getByLabel("E-mail", { exact: true }).fill(credentials.email);
  await page.getByLabel("Senha", { exact: true }).fill(credentials.password);
  await page.getByRole("button", { name: "Entrar", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Contas e cartões", exact: true }),
  ).toBeVisible();
  expect(await violations(page)).toEqual([]);

  for (const [path, title] of [
    ["/", "Início"],
    ["/transacoes", "Transações"],
    ["/configuracoes/geral", "Geral"],
    ["/configuracoes/seguranca", "Segurança"],
    ["/contas-e-cartoes/importar", "Importar extrato"],
  ]) {
    await page.goto(path);
    await expect(
      page.getByRole("heading", { name: title, exact: true, level: 1 }),
    ).toBeVisible();
    expect(await violations(page), path).toEqual([]);
  }

  // The template is saved through a blob: link.
  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: /Baixar modelo/ }).click();
  expect((await download).suggestedFilename()).toBe("modelo-extrato-flash.csv");
  expect(await violations(page)).toEqual([]);

  // A page that would run an inline script must be blocked, not just unflagged.
  const blocked = await page.evaluate(async () => {
    const script = document.createElement("script");
    script.textContent = "window.__inlineRan = true";
    document.head.append(script);
    await new Promise((resolve) => setTimeout(resolve, 100));
    return (window as Window & { __inlineRan?: boolean }).__inlineRan === true;
  });
  expect(blocked).toBe(false);
});
