import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";

import * as authApi from "../api/auth";
import { DemoNotice } from "./DemoNotice";

vi.mock("../api/auth");

function renderNotice() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <DemoNotice />
    </QueryClientProvider>,
  );
}

it("tells visitors the instance is a demo", async () => {
  vi.mocked(authApi.getSession).mockResolvedValue({ authenticated: false, authentication_enabled: false, demo: true });
  renderNotice();
  expect(await screen.findByText("Ambiente de demonstração.")).toBeInTheDocument();
  expect(screen.getByRole("note")).toHaveTextContent("redefinidos todos os dias");
});

it("stays out of the way on a regular instance", async () => {
  vi.mocked(authApi.getSession).mockResolvedValue({ authenticated: true, authentication_enabled: true });
  renderNotice();
  await vi.waitFor(() => expect(authApi.getSession).toHaveBeenCalled());
  expect(screen.queryByRole("note")).not.toBeInTheDocument();
});
