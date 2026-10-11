import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { QuotesSettings } from "./QuotesSettings";

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": status === 200 ? "application/json" : "application/problem+json" },
  });

const lastRefresh = (overrides: Record<string, unknown> = {}) => ({
  finished_at: "2026-10-10T22:00:00Z",
  trigger: "scheduled",
  status: "success",
  assets: 3,
  fetched: 3,
  failures: 0,
  deferred: 0,
  ...overrides,
});

const settings = (overrides: Record<string, unknown> = {}) => ({
  enabled: true,
  time: "19:00",
  timezone: "America/Sao_Paulo",
  last_refresh: lastRefresh(),
  ...overrides,
});

function mockFetch(...responses: Response[]) {
  const fetch = vi.fn<typeof globalThis.fetch>();
  responses.forEach((response) => fetch.mockResolvedValueOnce(response));
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

afterEach(() => vi.unstubAllGlobals());

describe("QuotesSettings", () => {
  it("shows the persisted schedule and last refresh", async () => {
    mockFetch(json(settings({ enabled: false, time: "08:15", timezone: "Asia/Tokyo" })));
    render(<QuotesSettings />);
    expect(await screen.findByText("3 de 3 ativos cotados.")).toBeInTheDocument();
    expect(screen.getByText(/Agendada/)).toBeInTheDocument();
    expect(screen.getByRole("switch", { name: "Atualizar automaticamente" })).not.toBeChecked();
    expect(screen.getByLabelText("Horário")).toHaveValue("08:15");
    expect(screen.getByLabelText("Horário")).toBeDisabled();
    expect(screen.getByText("Asia/Tokyo")).toBeInTheDocument();
    expect(screen.getByText("Provedor de cotações (avançado)")).toBeInTheDocument();
  });

  it("offers a retry when loading fails", async () => {
    const fetch = mockFetch(json({ type: "x", title: "x", status: 503 }, 503), json(settings({ last_refresh: null })));
    render(<QuotesSettings />);
    await userEvent.click(await screen.findByRole("button", { name: "Tentar novamente" }));
    expect(await screen.findByText("Nenhuma atualização ainda.")).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it.each([
    { assets: 0, fetched: 0 },
    { assets: 3, fetched: 2 },
  ])("shows a failed refresh with $assets assets and $fetched fetched as an error", async (counts) => {
    mockFetch(json(settings({ last_refresh: lastRefresh({ status: "failed", ...counts }) })));
    render(<QuotesSettings />);
    expect(
      await screen.findByText("Não foi possível concluir a atualização. Tente novamente mais tarde."),
    ).toHaveClass("ant-typography-danger");
    expect(screen.queryByText("Nenhum ativo com cotação automática para atualizar.")).not.toBeInTheDocument();
    expect(screen.queryByText("Nenhum ativo cotado. Tente novamente mais tarde.")).not.toBeInTheDocument();
  });

  it("saves the schedule", async () => {
    const fetch = mockFetch(json(settings({ enabled: false })), json({ saved: true }));
    render(<QuotesSettings />);
    await userEvent.click(await screen.findByRole("switch", { name: "Atualizar automaticamente" }));
    const time = screen.getByLabelText("Horário");
    await userEvent.clear(time);
    await userEvent.type(time, "07:30");
    await userEvent.click(screen.getByRole("button", { name: "Salvar agendamento" }));
    expect(await screen.findByText("Agendamento salvo.")).toBeInTheDocument();
    const [url, init] = fetch.mock.calls[1];
    expect(url).toBe("/api/settings/quotes");
    expect(init?.method).toBe("PUT");
    expect(JSON.parse(String(init?.body))).toEqual({ enabled: true, time: "07:30", timezone: "America/Sao_Paulo" });
  });

  it("reports a rejected schedule", async () => {
    mockFetch(json(settings()), json({ type: "x", title: "Dados inválidos", status: 422, detail: "invalid" }, 422));
    render(<QuotesSettings />);
    await userEvent.click(await screen.findByRole("button", { name: "Salvar agendamento" }));
    expect(await screen.findByText("Confira o horário e o fuso horário.")).toBeInTheDocument();
  });

  it("updates now and shows each outcome", async () => {
    const fetch = mockFetch(
      json(settings({ enabled: false, last_refresh: null })),
      json(lastRefresh({ trigger: "manual", status: "deferred", fetched: 2, deferred: 1 })),
      json(lastRefresh({ trigger: "manual", status: "failed", fetched: 0, failures: 3 })),
      json({ type: "x", title: "Atualização em andamento", status: 409 }, 409),
      json({ type: "x", title: "x", status: 503 }, 503),
    );
    render(<QuotesSettings />);
    const button = await screen.findByRole("button", { name: "Atualizar agora" });
    expect(screen.getByRole("switch", { name: "Atualizar automaticamente" })).not.toBeChecked();

    await userEvent.click(button);
    expect(
      await screen.findByText(
        "2 de 3 ativos cotados; 1 adiados porque o provedor limitou as consultas. Tente novamente mais tarde.",
      ),
    ).toBeInTheDocument();
    expect(screen.getByText(/Manual/)).toBeInTheDocument();
    expect(fetch.mock.calls[1][0]).toBe("/api/settings/quotes/refresh");
    expect(fetch.mock.calls[1][1]?.method).toBe("POST");

    await userEvent.click(button);
    expect(await screen.findByText("Não foi possível concluir a atualização. Tente novamente mais tarde.")).toBeInTheDocument();

    await userEvent.click(button);
    expect(await screen.findByText(/Já existe uma atualização em andamento/)).toBeInTheDocument();
    expect(screen.getByText("Não foi possível concluir a atualização. Tente novamente mais tarde.")).toBeInTheDocument();

    await userEvent.click(button);
    expect(await screen.findByText("Não foi possível atualizar as cotações agora.")).toBeInTheDocument();
  });

  it("shows the refresh as running", async () => {
    let finish: (response: Response) => void = () => undefined;
    const fetch = mockFetch(json(settings()));
    fetch.mockReturnValueOnce(new Promise((resolve) => (finish = resolve)));
    render(<QuotesSettings />);
    await userEvent.click(await screen.findByRole("button", { name: "Atualizar agora" }));
    expect(screen.getByText("Isso pode levar alguns segundos.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Atualizando/ })).toBeInTheDocument();
    finish(json(lastRefresh({ trigger: "manual", assets: 0, fetched: 0 })));
    expect(await screen.findByText("Nenhum ativo com cotação automática para atualizar.")).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText("Isso pode levar alguns segundos.")).not.toBeInTheDocument());
  });
});
