import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { I18nProvider } from "./I18nProvider";
import { defaultLanguage, languageStorageKey, rememberLanguage, storedLanguage, useTranslation } from "./context";

function Consumer() {
  const { t, setLanguage } = useTranslation();
  return <button onClick={() => setLanguage("en")}>{t("home")}</button>;
}

afterEach(() => {
  localStorage.clear();
  rememberLanguage(defaultLanguage);
  document.documentElement.lang = "pt-BR";
  vi.restoreAllMocks();
});

it("updates consumers, document language and saved preference", async () => {
  render(<I18nProvider><Consumer /><Consumer /></I18nProvider>);
  await userEvent.click(screen.getAllByRole("button", { name: "Início" })[0]);
  expect(screen.getAllByRole("button", { name: "Home" })).toHaveLength(2);
  expect(document.documentElement.lang).toBe("en");
  expect(localStorage.getItem(languageStorageKey)).toBe("en");
});

it("restores language and follows other tabs", () => {
  localStorage.setItem(languageStorageKey, "en");
  render(<I18nProvider><Consumer /></I18nProvider>);
  expect(screen.getByRole("button", { name: "Home" })).toBeInTheDocument();
  localStorage.clear();
  fireEvent(window, new StorageEvent("storage", { key: null }));
  expect(screen.getByRole("button", { name: "Início" })).toBeInTheDocument();
});

it("defaults to Portuguese for unsupported preferences", () => {
  localStorage.setItem(languageStorageKey, "fr");
  render(<I18nProvider><Consumer /></I18nProvider>);
  expect(screen.getByRole("button", { name: "Início" })).toBeInTheDocument();
});

it("switches with blocked storage", async () => {
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => { throw new Error("blocked"); });
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("blocked"); });
  render(<I18nProvider><Consumer /></I18nProvider>);
  await userEvent.click(screen.getByRole("button", { name: "Início" }));
  expect(screen.getByRole("button", { name: "Home" })).toBeInTheDocument();
});

it("keeps API messages in the chosen language when storage is blocked", async () => {
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("blocked"); });
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => { throw new Error("blocked"); });
  render(<I18nProvider><Consumer /></I18nProvider>);
  await userEvent.click(screen.getByRole("button", { name: "Início" }));
  expect(storedLanguage()).toBe("en");
});
