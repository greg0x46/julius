import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import { createAppQueryClient } from "./queryClient";
import { App } from "./App";
import { I18nProvider } from "../i18n/I18nProvider";

afterEach(() => vi.restoreAllMocks());

describe("App shell", () => {
  it("scrolls to the top when navigating to another page", async () => {
    const scrollTo = vi.spyOn(window, "scrollTo").mockImplementation(() => {});
    render(
      <QueryClientProvider client={createAppQueryClient()}>
        <MemoryRouter initialEntries={["/"]}>
          <Routes>
            <Route element={<App />}>
              <Route index element={<p>Início</p>} />
              <Route path="metas" element={<p>Metas page</p>} />
            </Route>
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );
    scrollTo.mockClear();
    await userEvent.click(screen.getByRole("link", { name: "Metas" }));
    expect(await screen.findByText("Metas page")).toBeInTheDocument();
    expect(scrollTo).toHaveBeenCalledWith(0, 0);
    expect(screen.getByRole("link", { name: "Metas" })).toHaveAttribute("aria-current", "page");
  });
});

it("switches navigation and accessibility labels from the header", async () => {
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  render(
    <I18nProvider>
      <QueryClientProvider client={createAppQueryClient()}>
        <MemoryRouter><App /></MemoryRouter>
      </QueryClientProvider>
    </I18nProvider>,
  );
  await userEvent.selectOptions(screen.getByRole("combobox", { name: "Idioma" }), "en");
  expect(screen.getByRole("navigation", { name: "Main navigation" })).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "Goals" })).toHaveAttribute("href", "/metas");
  expect(screen.getByRole("link", { name: "Skip to content" })).toBeInTheDocument();
  window.localStorage.removeItem("julius.language");
  document.documentElement.lang = "pt-BR";
});
