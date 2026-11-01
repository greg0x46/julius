import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "../i18n/I18nProvider";
import { GeneralSettingsPage } from "./GeneralSettingsPage";
import { queryKeys } from "../api/queryKeys";

const json = (body: unknown) =>
  new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
  });
afterEach(() => vi.unstubAllGlobals());

it("saves the currency with the existing preference and discards old report caches", async () => {
  const fetch = vi.fn<typeof globalThis.fetch>().mockImplementation(async (url, init) => {
    if (String(url).endsWith("/accounts/currencies")) return json({ currencies: [{ code: "BRL" }, { code: "USD" }, { code: "EUR" }], status: "complete", source: "frankfurter" });
    if (init?.method === "PUT") return json(JSON.parse(String(init.body)));
    return json({
      transactions_period_basis: "occurred_at",
      reference_currency: "BRL",
      timezone: "America/Sao_Paulo",
    });
  });
  vi.stubGlobal("fetch", fetch);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(queryKeys.timeline, {
    currency_code: "BRL",
    amount: "500",
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <GeneralSettingsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  const select = await screen.findByRole("combobox", {
    name: "Moeda de referência",
  });
  await userEvent.click(select);
  await userEvent.click(await screen.findByText(/USD —/));
  await userEvent.click(screen.getByRole("button", { name: "Salvar" }));
  await waitFor(() =>
    expect(fetch).toHaveBeenCalledWith(
      "/api/preferences",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({
          transactions_period_basis: "occurred_at",
          reference_currency: "USD",
          timezone: "America/Sao_Paulo",
        }),
      }),
    ),
  );
  await waitFor(() => expect(client.getQueryData(queryKeys.timeline)).toBeUndefined());
  client.clear();
});

it("saves a new timezone and drops every cache that depends on the zone", async () => {
  const fetch = vi.fn<typeof globalThis.fetch>().mockImplementation(async (url, init) => {
    if (String(url).endsWith("/accounts/currencies")) return json({ currencies: [{ code: "BRL" }], status: "complete", source: "frankfurter" });
    if (init?.method === "PUT") return json(JSON.parse(String(init.body)));
    return json({ transactions_period_basis: "occurred_at", reference_currency: "BRL", timezone: "America/Sao_Paulo" });
  });
  vi.stubGlobal("fetch", fetch);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(queryKeys.spendingByCategory("America/Sao_Paulo"), { items: [] });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <GeneralSettingsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  const select = await screen.findByRole("combobox", { name: "Fuso horário" });
  expect(screen.getByText("America/Sao_Paulo")).toBeInTheDocument();
  await userEvent.click(select);
  await userEvent.click(await screen.findByText("Asia/Tokyo"));
  await userEvent.click(screen.getByRole("button", { name: "Salvar" }));
  await waitFor(() =>
    expect(fetch).toHaveBeenCalledWith(
      "/api/preferences",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({
          transactions_period_basis: "occurred_at",
          reference_currency: "BRL",
          timezone: "Asia/Tokyo",
        }),
      }),
    ),
  );
  await waitFor(() => expect(client.getQueryData(queryKeys.spendingByCategory("America/Sao_Paulo"))).toBeUndefined());
  // The preferences themselves stay, now with the saved zone.
  expect(client.getQueryData(queryKeys.preferences)).toMatchObject({ timezone: "Asia/Tokyo" });
  client.clear();
});

it("switches the interface language from the settings screen", async () => {
  vi.stubGlobal("fetch", vi.fn<typeof globalThis.fetch>().mockImplementation(async (url) => {
    if (String(url).endsWith("/accounts/currencies")) return json({ currencies: [{ code: "BRL" }], status: "complete", source: "frankfurter" });
    return json({ transactions_period_basis: "occurred_at", reference_currency: "BRL", timezone: "America/Sao_Paulo" });
  }));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <I18nProvider>
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <GeneralSettingsPage />
        </MemoryRouter>
      </QueryClientProvider>
    </I18nProvider>,
  );
  await userEvent.click(await screen.findByRole("combobox", { name: "Idioma" }));
  await userEvent.click(await screen.findByText("English"));
  expect(await screen.findByRole("combobox", { name: "Language" })).toBeInTheDocument();
  window.localStorage.removeItem("julius.language");
  document.documentElement.lang = "pt-BR";
  client.clear();
});
