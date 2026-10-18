import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { expect, it, vi } from "vitest";

import * as authApi from "../api/auth";
import { queryKeys } from "../api/queryKeys";
import { AuthGate } from "./AuthGate";

vi.mock("../api/auth");
vi.mock("../pages/LoginPage", () => ({ LoginPage: () => <div>LOGIN</div> }));

function renderGate(client: QueryClient) {
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <AuthGate>
          <div>APP</div>
        </AuthGate>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

it("keeps the app mounted when a background session refetch fails", async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  vi.mocked(authApi.getSession)
    .mockResolvedValueOnce({ authenticated: true, authentication_enabled: true })
    .mockRejectedValue(new TypeError("offline"));
  renderGate(client);
  await screen.findByText("APP");
  await client.refetchQueries({ queryKey: queryKeys.authSession });
  await waitFor(() => expect(authApi.getSession).toHaveBeenCalledTimes(2));
  await waitFor(() => expect(client.getQueryState(queryKeys.authSession)?.status).toBe("error"));
  expect(screen.getByText("APP")).toBeInTheDocument();
});

it("shows the unavailable state when the first session check fails", async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  vi.mocked(authApi.getSession).mockRejectedValue(new TypeError("offline"));
  renderGate(client);
  expect(await screen.findByText("Não foi possível verificar sua sessão.")).toBeInTheDocument();
  expect(screen.queryByText("APP")).not.toBeInTheDocument();
});
