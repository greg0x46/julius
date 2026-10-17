import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { GeneralSettingsPage } from "./GeneralSettingsPage";
import { queryKeys } from "../api/queryKeys";

const json = (body: unknown) =>
  new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
  });
afterEach(() => vi.unstubAllGlobals());

it("saves the currency with the existing preference and discards old report caches", async () => {
  const fetch = vi.fn<typeof globalThis.fetch>().mockImplementation(async (url, init) => {
    if (String(url).endsWith("/accounts/currencies")) return json(["BRL", "USD", "EUR"]);
    if (init?.method === "PUT") return json(JSON.parse(String(init.body)));
    return json({
      transactions_period_basis: "occurred_at",
      reference_currency: "BRL",
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
        }),
      }),
    ),
  );
  await waitFor(() => expect(client.getQueryData(queryKeys.timeline)).toBeUndefined());
  client.clear();
});
