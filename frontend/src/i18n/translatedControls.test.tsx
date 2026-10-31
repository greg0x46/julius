import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "./I18nProvider";
import { useTranslation } from "./context";
import { Password, Select } from "../components/ui";
import { SettingsPage } from "../pages/SettingsPage";

function LanguageSwitch() {
  const { setLanguage } = useTranslation();
  return <button onClick={() => setLanguage("en")}>English</button>;
}

afterEach(() => {
  localStorage.clear();
  document.documentElement.lang = "pt-BR";
});

it("updates settings and shared controls when the language changes", async () => {
  render(
    <I18nProvider>
      <MemoryRouter>
        <LanguageSwitch />
        <SettingsPage />
        <Password aria-label="Password field" />
        <Select aria-label="Choice" />
      </MemoryRouter>
    </I18nProvider>,
  );
  expect(screen.getByRole("heading", { name: "Configurações" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Mostrar senha" })).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "English" }));
  expect(screen.getByRole("heading", { name: "Settings" })).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "Open Categories" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Show password" })).toBeInTheDocument();
  await userEvent.click(screen.getByRole("combobox", { name: "Choice" }));
  expect(screen.getByText("No options")).toBeInTheDocument();
});
