import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";

import * as authApi from "../api/auth";
import { AuthenticationSettings } from "./AuthenticationSettings";

vi.mock("../api/auth");

function renderSettings() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <AuthenticationSettings />
    </QueryClientProvider>,
  );
}

it("explains the open access instead of offering to change it in the demo", async () => {
  vi.mocked(authApi.getSession).mockResolvedValue({ authenticated: false, authentication_enabled: false, demo: true });
  renderSettings();
  expect(await screen.findByText("Ambiente de demonstração")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Ativar autenticação" })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Desativar autenticação" })).not.toBeInTheDocument();
});

it("still offers to enable authentication when it is off on a regular instance", async () => {
  vi.mocked(authApi.getSession).mockResolvedValue({ authenticated: false, authentication_enabled: false });
  renderSettings();
  expect(await screen.findByRole("button", { name: "Ativar autenticação" })).toBeInTheDocument();
});
